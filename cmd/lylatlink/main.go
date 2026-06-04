package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"lylatlink/internal/app"
	"lylatlink/internal/audio"
	"lylatlink/internal/config"
	"lylatlink/internal/console"
	"lylatlink/internal/hotkey"
	"lylatlink/internal/tray"
)

func main() {
	var (
		configPath        = flag.String("config", "", "path to config.toml")
		replayDir         = flag.String("replay-dir", "", "override replay directory")
		signalURL         = flag.String("signal-url", "", "override signaling server URL")
		autoJoin          = flag.Bool("auto-join", false, "join voice automatically when a match is detected")
		once              = flag.String("parse-once", "", "parse one .slp file and print match metadata")
		listAudioDevices  = flag.Bool("list-audio-devices", false, "list input devices and exit")
		listOutputDevices = flag.Bool("list-output-devices", false, "list output devices and exit")
		audioInputDevice  = flag.String("audio-input-device", "", "input device ID or exact name for mic capture")
		audioOutputDevice = flag.String("audio-output-device", "", "output device ID or exact name for remote playback")
		audioCodec        = flag.String("audio-codec", "", "voice codec: opus or pcmu")
		inputGainDB       = flag.Float64("input-gain-db", 0, "microphone gain in dB; config value is used when omitted")
		outputGainDB      = flag.Float64("output-gain-db", 0, "remote playback gain in dB; config value is used when omitted")
		noiseGateDB       = flag.Float64("noise-gate-db", 0, "noise gate threshold in dBFS; config value is used when omitted")
		audioTest         = flag.Bool("audio-device-test", false, "capture microphone and print level diagnostics")
		audioDur          = flag.Duration("audio-test-duration", 10*time.Second, "duration for -audio-device-test")
		audioDebug        = flag.Bool("audio-test-verbose", false, "include low-level audio backend logs during -audio-device-test")
		verbose           = flag.Bool("verbose", false, "enable verbose WebRTC/audio diagnostics")
		syntheticAudio    = flag.Bool("synthetic-audio", false, "send synthetic PCMU audio instead of live microphone audio")
		noPlayback        = flag.Bool("no-playback", false, "disable remote speaker playback")
		ignoreMatchEnd    = flag.Bool("ignore-match-end", false, "test mode: keep voice open when copied replays contain Game End")
		exitWhenPID       = flag.Int("exit-when-pid", 0, "exit when the given process ID is no longer running")
		trayMode          = flag.Bool("tray", true, "run with system tray menu")
		consoleMode       = flag.Bool("console", false, "run in the foreground without the system tray")
	)
	flag.Parse()

	needsConsole := *consoleMode || *verbose || *once != "" || *listAudioDevices || *listOutputDevices || *audioTest
	if needsConsole {
		if err := console.Enable(); err != nil {
			log.Printf("enable console failed: %v", err)
		}
	}

	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	ctx, cancel := context.WithCancel(signalCtx)
	defer cancel()
	if *exitWhenPID > 0 {
		watchExitProcess(ctx, cancel, *exitWhenPID)
	}

	if *once != "" {
		if err := app.ParseOnce(os.Stdout, *once); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *listAudioDevices {
		devices, err := audio.ListInputDevices()
		if err != nil {
			log.Fatal(err)
		}
		if len(devices) == 0 {
			fmt.Println("no input devices found")
			return
		}
		for _, device := range devices {
			defaultMark := ""
			if device.IsDefault {
				defaultMark = " (default)"
			}
			fmt.Printf("%s\t%s%s\n", device.ID, device.Name, defaultMark)
		}
		return
	}
	if *listOutputDevices {
		devices, err := audio.ListOutputDevices()
		if err != nil {
			log.Fatal(err)
		}
		if len(devices) == 0 {
			fmt.Println("no output devices found")
			return
		}
		for _, device := range devices {
			defaultMark := ""
			if device.IsDefault {
				defaultMark = " (default)"
			}
			fmt.Printf("%s\t%s%s\n", device.ID, device.Name, defaultMark)
		}
		return
	}
	if *audioTest {
		if err := audio.RunDeviceTest(ctx, audio.DeviceTestOptions{
			Duration:      *audioDur,
			Verbose:       *audioDebug,
			InputDeviceID: *audioInputDevice,
		}); err != nil {
			log.Fatal(err)
		}
		return
	}

	resolvedConfigPath := *configPath
	if resolvedConfigPath == "" {
		var err error
		resolvedConfigPath, err = config.DefaultPath()
		if err != nil {
			log.Fatal(err)
		}
	}

	cfg, err := config.LoadOrCreate(resolvedConfigPath)
	if err != nil {
		log.Fatal(err)
	}

	if *replayDir != "" {
		cfg.ReplayDir = *replayDir
	} else if cfg.ReplayDir == "" {
		resolution, err := config.ResolveReplayDir()
		if err != nil {
			log.Printf("auto-detect replay folder failed: %v", err)
		} else {
			cfg.ReplayDir = resolution.Path
			cfg.ReplayDirAutoDetected = true
			log.Printf("auto-detected replay folder from %s: %s", resolution.Source, resolution.Path)
			if resolution.EnableNetplayReplays != nil && !*resolution.EnableNetplayReplays {
				log.Printf("Slippi netplay replay saving appears disabled; LylatLink may not detect live matches")
			}
		}
	}
	if *signalURL != "" {
		cfg.SignalBaseURL = *signalURL
	}
	if *autoJoin {
		cfg.AutoJoin = true
	}
	if *audioInputDevice != "" {
		cfg.InputDeviceID = *audioInputDevice
	}
	if *audioOutputDevice != "" {
		cfg.OutputDeviceID = *audioOutputDevice
	}
	if *audioCodec != "" {
		cfg.AudioCodec = *audioCodec
	}
	if flagWasSet("input-gain-db") {
		cfg.InputGainDB = *inputGainDB
	}
	if flagWasSet("output-gain-db") {
		cfg.OutputGainDB = *outputGainDB
	}
	if flagWasSet("noise-gate-db") {
		cfg.NoiseGateDB = *noiseGateDB
	}
	useTray := *trayMode && !*consoleMode
	if cfg.ReplayDir == "" && !useTray {
		path, _ := config.DefaultPath()
		fmt.Fprintf(os.Stderr, "missing replay_dir; set it in %s or pass -replay-dir\n", path)
		os.Exit(2)
	}

	opts := app.Options{SyntheticAudio: *syntheticAudio, NoPlayback: *noPlayback, IgnoreMatchEnd: *ignoreMatchEnd, Verbose: *verbose}
	if useTray {
		trayCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		controller := app.NewController(cfg, resolvedConfigPath, opts)
		hotkeys := newEndCallHotkeys(trayCtx, controller)
		controller.SetEndCallHotkeyChanged(hotkeys.Set)
		hotkeys.Set(cfg.EndCallHotkey)
		errs := make(chan error, 1)
		go func() {
			errs <- controller.Run(trayCtx)
		}()
		tray.Run(trayCtx, controller)
		cancel()
		if err := <-errs; err != nil {
			log.Fatal(err)
		}
		return
	}

	controller := app.NewController(cfg, resolvedConfigPath, opts)
	hotkeys := newEndCallHotkeys(ctx, controller)
	controller.SetEndCallHotkeyChanged(hotkeys.Set)
	hotkeys.Set(cfg.EndCallHotkey)
	if err := controller.Run(ctx); err != nil {
		log.Fatal(err)
	}
}

type endCallHotkeys struct {
	ctx        context.Context
	controller *app.Controller

	mu     sync.Mutex
	cancel context.CancelFunc
	key    string
}

func newEndCallHotkeys(ctx context.Context, controller *app.Controller) *endCallHotkeys {
	return &endCallHotkeys{ctx: ctx, controller: controller}
}

func (h *endCallHotkeys) Set(key string) {
	key = hotkey.NormalizeKey(key)

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.key == key {
		return
	}
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
		time.Sleep(100 * time.Millisecond)
	}
	h.key = key
	if key == "" {
		log.Printf("global end-call hotkey disabled")
		return
	}

	hotkeyCtx, cancel := context.WithCancel(h.ctx)
	if err := hotkey.Start(hotkeyCtx, key, func() {
		log.Printf("global end-call hotkey detected: %s", hotkey.Label(key))
		h.controller.EndCall()
	}); err != nil {
		cancel()
		h.key = ""
		log.Printf("global end-call hotkey unavailable (%s): %v", hotkey.Label(key), err)
		return
	}
	h.cancel = cancel
	log.Printf("global end-call hotkey registered: %s", hotkey.Label(key))
}

func flagWasSet(name string) bool {
	wasSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			wasSet = true
		}
	})
	return wasSet
}
