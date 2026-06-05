package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"lylatlink/internal/config"
	"lylatlink/internal/hotkey"
	"lylatlink/internal/signaling"
	"lylatlink/internal/slp"
	"lylatlink/internal/voice"
	"lylatlink/internal/watcher"
)

type Options struct {
	SyntheticAudio bool
	NoPlayback     bool
	IgnoreMatchEnd bool
	Verbose        bool
}

type State string

const (
	StateWatching     State = "watching"
	StateWaiting      State = "waiting"
	StateInVoice      State = "in_voice"
	StateNotReady     State = "not_ready"
	StateError        State = "error"
	StateShuttingDown State = "shutting_down"
)

type Status struct {
	State          State
	ReplayDir      string
	AutoJoin       bool
	PlayChimes     bool
	InputDeviceID  string
	OutputDeviceID string
	AudioCodec     string
	InputGainDB    float64
	OutputGainDB   float64
	NoiseGateDB    float64
	NoPlayback     bool
	EndCallHotkey  string
	MatchID        string
	MatchLabel     string
	Message        string
}

type Controller struct {
	mu sync.Mutex

	cfg     config.Config
	cfgPath string
	opts    Options

	endCallHotkeyChanged func(string)

	commands chan command
	statusCh chan Status
	last     Status
}

type command struct {
	kind           commandKind
	autoJoin       bool
	playChimes     bool
	inputDeviceID  string
	outputDeviceID string
	outputGainDB   float64
	replayDir      string
	endCallHotkey  string
}

type signalResult struct {
	match   slp.Match
	resp    *signaling.StartResponse
	err     error
	elapsed time.Duration
}

type commandKind int

const (
	commandSetAutoJoin commandKind = iota + 1
	commandSetPlayChimes
	commandSetInputDevice
	commandSetOutputDevice
	commandSetOutputGain
	commandSetReplayDir
	commandSetEndCallHotkey
	commandEndCall
)

func ParseOnce(out io.Writer, path string) error {
	match, err := slp.ParseFile(path)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(match)
}

func Run(ctx context.Context, cfg config.Config, opts Options) error {
	controller := NewController(cfg, "", opts)
	return controller.Run(ctx)
}

func NewController(cfg config.Config, cfgPath string, opts Options) *Controller {
	controller := &Controller{
		cfg:      cfg,
		cfgPath:  cfgPath,
		opts:     opts,
		commands: make(chan command, 16),
		statusCh: make(chan Status, 16),
	}
	controller.last = controller.status(StateWatching, "", "", "")
	if cfg.ReplayDir == "" {
		controller.last = controller.status(StateNotReady, "", "", "Set replay_dir in config before watching.")
	}
	return controller
}

func (c *Controller) Run(ctx context.Context) error {
	nonce, err := clientNonce()
	if err != nil {
		return err
	}

	c.mu.Lock()
	cfg := c.cfg
	c.mu.Unlock()

	if !cfg.AutoJoin {
		log.Printf("auto_join=false; matches will be detected but not submitted")
	}

	signalClient := signaling.NewClient(cfg.SignalBaseURL)
	voiceController := c.newVoiceController(signalClient)
	active := map[string]slp.Match{}
	waiting := map[string]slp.Match{}
	signalResults := make(chan signalResult, 16)

	var events chan watcher.MatchEvent
	var errs <-chan error
	var stopWatcher context.CancelFunc

	if cfg.ReplayDir == "" {
		c.publish(c.status(StateNotReady, "", "", "Set replay_dir in config or choose a Replay Folder."))
	} else {
		events, errs, stopWatcher = startWatcher(ctx, cfg.ReplayDir)
		c.publish(c.status(StateWatching, "", "", ""))
	}

	for {
		select {
		case <-ctx.Done():
			if stopWatcher != nil {
				stopWatcher()
			}
			c.publish(c.status(StateShuttingDown, "", "", ""))
			return nil
		case err := <-errs:
			if err != nil {
				c.publish(c.status(StateError, "", "", err.Error()))
			}
			return err
		case cmd := <-c.commands:
			switch cmd.kind {
			case commandSetAutoJoin:
				c.setAutoJoin(cmd.autoJoin)
				c.publish(c.statusForCurrent(active, waiting, ""))
			case commandSetPlayChimes:
				c.setPlayChimes(cmd.playChimes)
				voiceController.Options.PlayChimes = cmd.playChimes
				c.publish(c.statusForCurrent(active, waiting, ""))
			case commandSetInputDevice:
				c.setInputDeviceID(cmd.inputDeviceID)
				voiceController.Options.InputDeviceID = cmd.inputDeviceID
				c.publish(c.statusForCurrent(active, waiting, "Input device applies to the next voice session."))
			case commandSetOutputDevice:
				c.setOutputDeviceID(cmd.outputDeviceID)
				voiceController.Options.OutputDeviceID = cmd.outputDeviceID
				c.publish(c.statusForCurrent(active, waiting, "Output device applies to the next voice session."))
			case commandSetOutputGain:
				c.setOutputGainDB(cmd.outputGainDB)
				voiceController.Options.OutputGainDB = cmd.outputGainDB
				c.publish(c.statusForCurrent(active, waiting, ""))
			case commandSetReplayDir:
				if err := c.setReplayDir(cmd.replayDir); err != nil {
					c.publish(c.status(StateError, "", "", err.Error()))
					continue
				}
				c.endCalls(ctx, signalClient, voiceController, nonce, active, waiting)
				if stopWatcher != nil {
					stopWatcher()
				}
				events, errs, stopWatcher = startWatcher(ctx, cmd.replayDir)
				c.publish(c.status(StateWatching, "", "", "Replay folder updated."))
			case commandSetEndCallHotkey:
				c.setEndCallHotkey(cmd.endCallHotkey)
				c.publish(c.statusForCurrent(active, waiting, ""))
			case commandEndCall:
				c.endCalls(ctx, signalClient, voiceController, nonce, active, waiting)
				c.publish(c.status(StateWatching, "", "", ""))
			}
		case result := <-signalResults:
			matchID := result.match.MatchID
			if result.err != nil {
				log.Printf("match start signaling failed: %v", result.err)
				if _, ok := waiting[matchID]; ok {
					delete(waiting, matchID)
					c.publish(c.status(StateError, matchID, matchLabel(result.match), result.err.Error()))
				}
				continue
			}
			if result.resp == nil {
				log.Printf("match start signaling failed: empty response")
				if _, ok := waiting[matchID]; ok {
					delete(waiting, matchID)
					c.publish(c.status(StateError, matchID, matchLabel(result.match), "empty signaling response"))
				}
				continue
			}
			log.Printf("match start signaling response: %s status=%s elapsed=%s", matchID, result.resp.Status, result.elapsed.Round(time.Millisecond))
			if _, ok := waiting[matchID]; !ok {
				continue
			}
			switch result.resp.Status {
			case "ready":
				delete(waiting, matchID)
				active[matchID] = result.match
				c.publish(c.status(StateInVoice, matchID, matchLabel(result.match), ""))
				if err := voiceController.Start(ctx, matchID, *result.resp); err != nil {
					log.Printf("voice start failed: %v", err)
					c.publish(c.status(StateError, matchID, matchLabel(result.match), err.Error()))
				}
			case "ended":
				delete(waiting, matchID)
				c.publish(c.statusForCurrent(active, waiting, ""))
			case "waiting":
				c.publish(c.status(StateWaiting, matchID, matchLabel(result.match), "Waiting for other player."))
				log.Printf("signaling match start retry: %s", matchID)
				go signalMatchStart(ctx, signalClient, result.match, nonce, signalResults)
			default:
				log.Printf("unexpected match start signaling status: %s status=%s", matchID, result.resp.Status)
				c.publish(c.status(StateWaiting, matchID, matchLabel(result.match), "Waiting for other player."))
			}
		case event := <-events:
			switch event.Type {
			case watcher.EventMatchStart:
				log.Printf("match detected: %s (%s)", event.Match.MatchID, event.Match.PlayerCodes)
				c.mu.Lock()
				cfg = c.cfg
				c.mu.Unlock()

				if !cfg.AutoJoin {
					c.publish(c.status(StateWatching, event.Match.MatchID, matchLabel(event.Match), "Match detected; auto-join is off."))
					continue
				}
				waiting[event.Match.MatchID] = event.Match
				c.publish(c.status(StateWaiting, event.Match.MatchID, matchLabel(event.Match), "Waiting for other player."))
				log.Printf("signaling match start: %s", event.Match.MatchID)
				go signalMatchStart(ctx, signalClient, event.Match, nonce, signalResults)
			case watcher.EventMatchEnd:
				log.Printf("match ended: %s", event.Match.MatchID)
				if c.opts.IgnoreMatchEnd {
					log.Printf("ignoring match end for test mode: %s", event.Match.MatchID)
					continue
				}
				if _, ok := waiting[event.Match.MatchID]; ok {
					go signalMatchEnd(ctx, signalClient, event.Match.MatchID, nonce)
					delete(waiting, event.Match.MatchID)
				}
				if _, ok := active[event.Match.MatchID]; ok {
					go signalMatchEnd(ctx, signalClient, event.Match.MatchID, nonce)
					if err := voiceController.Stop(ctx, event.Match.MatchID); err != nil {
						log.Printf("voice stop failed: %v", err)
					}
					delete(active, event.Match.MatchID)
				}
				c.publish(c.statusForCurrent(active, waiting, ""))
			}
		}
	}
}

func signalMatchEnd(ctx context.Context, signalClient *signaling.Client, matchID string, nonce string) {
	if err := signalClient.EndMatch(ctx, matchID, nonce); err != nil {
		log.Printf("match end signaling failed: %v", err)
	}
}

func signalMatchStart(ctx context.Context, signalClient *signaling.Client, match slp.Match, nonce string, out chan<- signalResult) {
	startedAt := time.Now()
	resp, err := signalClient.StartMatch(ctx, signaling.NewStartRequest(&match, nonce))
	result := signalResult{
		match:   match,
		resp:    resp,
		err:     err,
		elapsed: time.Since(startedAt),
	}
	select {
	case <-ctx.Done():
	case out <- result:
	}
}

func startWatcher(ctx context.Context, replayDir string) (chan watcher.MatchEvent, <-chan error, context.CancelFunc) {
	events := make(chan watcher.MatchEvent, 16)
	errs := make(chan error, 1)
	watcherCtx, stopWatcher := context.WithCancel(ctx)
	w := watcher.New(replayDir)
	go func() {
		errs <- w.Run(watcherCtx, events)
	}()
	log.Printf("LylatLink watching %s", replayDir)
	return events, errs, stopWatcher
}

func (c *Controller) Status() <-chan Status {
	return c.statusCh
}

func (c *Controller) Snapshot() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last
}

func (c *Controller) ConfigPath() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfgPath
}

func (c *Controller) SetEndCallHotkeyChanged(fn func(string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.endCallHotkeyChanged = fn
}

func (c *Controller) SetAutoJoin(enabled bool) {
	c.enqueue(command{kind: commandSetAutoJoin, autoJoin: enabled})
}

func (c *Controller) SetPlayChimes(enabled bool) {
	c.enqueue(command{kind: commandSetPlayChimes, playChimes: enabled})
}

func (c *Controller) SetInputDeviceID(id string) {
	c.enqueue(command{kind: commandSetInputDevice, inputDeviceID: id})
}

func (c *Controller) SetOutputDeviceID(id string) {
	c.enqueue(command{kind: commandSetOutputDevice, outputDeviceID: id})
}

func (c *Controller) SetOutputGainDB(gainDB float64) {
	c.enqueue(command{kind: commandSetOutputGain, outputGainDB: gainDB})
}

func (c *Controller) SetReplayDir(path string) {
	c.enqueue(command{kind: commandSetReplayDir, replayDir: path})
}

func (c *Controller) SetEndCallHotkey(key string) {
	c.enqueue(command{kind: commandSetEndCallHotkey, endCallHotkey: key})
}

func (c *Controller) EndCall() {
	c.enqueue(command{kind: commandEndCall})
}

func (c *Controller) enqueue(cmd command) {
	select {
	case c.commands <- cmd:
	default:
		log.Printf("app command dropped: %v", cmd.kind)
	}
}

func (c *Controller) newVoiceController(signalClient *signaling.Client) *voice.WebRTCController {
	c.mu.Lock()
	cfg := c.cfg
	c.mu.Unlock()
	return voice.NewWebRTCController(signalClient, voice.Options{
		InputDeviceID:     cfg.InputDeviceID,
		OutputDeviceID:    cfg.OutputDeviceID,
		AudioCodec:        cfg.AudioCodec,
		InputGainDB:       cfg.InputGainDB,
		OutputGainDB:      cfg.OutputGainDB,
		NoiseGateDB:       cfg.NoiseGateDB,
		PlayChimes:        cfg.PlayChimes,
		UseSyntheticAudio: c.opts.SyntheticAudio,
		DisablePlayback:   c.opts.NoPlayback,
		Verbose:           c.opts.Verbose,
	})
}

func (c *Controller) setAutoJoin(enabled bool) {
	c.mu.Lock()
	c.cfg.AutoJoin = enabled
	cfg := c.cfg
	c.mu.Unlock()
	c.saveConfig(cfg)
}

func (c *Controller) setPlayChimes(enabled bool) {
	c.mu.Lock()
	c.cfg.PlayChimes = enabled
	cfg := c.cfg
	c.mu.Unlock()
	c.saveConfig(cfg)
}

func (c *Controller) setInputDeviceID(id string) {
	c.mu.Lock()
	c.cfg.InputDeviceID = id
	cfg := c.cfg
	c.mu.Unlock()
	c.saveConfig(cfg)
}

func (c *Controller) setOutputDeviceID(id string) {
	c.mu.Lock()
	c.cfg.OutputDeviceID = id
	cfg := c.cfg
	c.mu.Unlock()
	c.saveConfig(cfg)
}

func (c *Controller) setOutputGainDB(gainDB float64) {
	c.mu.Lock()
	c.cfg.OutputGainDB = gainDB
	cfg := c.cfg
	c.mu.Unlock()
	c.saveConfig(cfg)
}

func (c *Controller) setReplayDir(path string) error {
	if path == "" {
		return fmt.Errorf("replay folder cannot be empty")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat replay folder: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("replay folder is not a directory: %s", path)
	}

	c.mu.Lock()
	c.cfg.ReplayDir = path
	c.cfg.ReplayDirAutoDetected = false
	cfg := c.cfg
	c.mu.Unlock()
	c.saveConfig(cfg)
	return nil
}

func (c *Controller) setEndCallHotkey(key string) {
	key = hotkey.NormalizeKey(key)
	c.mu.Lock()
	c.cfg.EndCallHotkey = key
	cfg := c.cfg
	changed := c.endCallHotkeyChanged
	c.mu.Unlock()
	c.saveConfig(cfg)
	if changed != nil {
		changed(key)
	}
}

func (c *Controller) saveConfig(cfg config.Config) {
	if c.cfgPath == "" {
		return
	}
	if cfg.ReplayDirAutoDetected {
		cfg.ReplayDir = ""
	}
	if err := config.Save(c.cfgPath, cfg); err != nil {
		log.Printf("save config failed: %v", err)
		c.publish(c.status(StateError, "", "", err.Error()))
	}
}

func (c *Controller) endCalls(ctx context.Context, signalClient *signaling.Client, voiceController voice.Controller, nonce string, active map[string]slp.Match, waiting map[string]slp.Match) {
	for matchID := range waiting {
		if err := signalClient.EndMatch(ctx, matchID, nonce); err != nil {
			log.Printf("match end signaling failed: %v", err)
		}
		delete(waiting, matchID)
	}
	for matchID := range active {
		if err := signalClient.EndMatch(ctx, matchID, nonce); err != nil {
			log.Printf("match end signaling failed: %v", err)
		}
		if err := voiceController.Stop(ctx, matchID); err != nil {
			log.Printf("voice stop failed: %v", err)
		}
		delete(active, matchID)
	}
}

func (c *Controller) statusForCurrent(active map[string]slp.Match, waiting map[string]slp.Match, message string) Status {
	c.mu.Lock()
	replayDir := c.cfg.ReplayDir
	c.mu.Unlock()
	if replayDir == "" {
		if message == "" {
			message = "Set replay_dir in config or choose a Replay Folder."
		}
		return c.status(StateNotReady, "", "", message)
	}
	for matchID, match := range active {
		return c.status(StateInVoice, matchID, matchLabel(match), message)
	}
	for matchID, match := range waiting {
		if message == "" {
			message = "Waiting for other player."
		}
		return c.status(StateWaiting, matchID, matchLabel(match), message)
	}
	return c.status(StateWatching, "", "", message)
}

func (c *Controller) status(state State, matchID string, label string, message string) Status {
	c.mu.Lock()
	cfg := c.cfg
	c.mu.Unlock()
	return Status{
		State:          state,
		ReplayDir:      cfg.ReplayDir,
		AutoJoin:       cfg.AutoJoin,
		PlayChimes:     cfg.PlayChimes,
		InputDeviceID:  cfg.InputDeviceID,
		OutputDeviceID: cfg.OutputDeviceID,
		AudioCodec:     cfg.AudioCodec,
		InputGainDB:    cfg.InputGainDB,
		OutputGainDB:   cfg.OutputGainDB,
		NoiseGateDB:    cfg.NoiseGateDB,
		NoPlayback:     c.opts.NoPlayback,
		EndCallHotkey:  cfg.EndCallHotkey,
		MatchID:        matchID,
		MatchLabel:     label,
		Message:        message,
	}
}

func (c *Controller) publish(status Status) {
	c.mu.Lock()
	c.last = status
	c.mu.Unlock()
	select {
	case c.statusCh <- status:
	default:
	}
}

func matchLabel(match slp.Match) string {
	if len(match.PlayerCodes) == 2 {
		return fmt.Sprintf("%s vs %s", match.PlayerCodes[0], match.PlayerCodes[1])
	}
	if len(match.PlayerCodes) == 1 {
		return fmt.Sprintf("%s self-match", match.PlayerCodes[0])
	}
	return match.MatchID
}

func clientNonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate client nonce: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
