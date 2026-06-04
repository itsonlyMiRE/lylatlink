package tray

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/getlantern/systray"
	"github.com/sqweek/dialog"

	"lylatlink/assets"
	"lylatlink/internal/app"
	"lylatlink/internal/audio"
	"lylatlink/internal/hotkey"
)

const projectURL = "https://github.com/itsonlyMiRE/lylatlink"

type Runner struct {
	Controller *app.Controller
}

type menu struct {
	controller *app.Controller

	title         *systray.MenuItem
	status        *systray.MenuItem
	match         *systray.MenuItem
	editConfig    *systray.MenuItem
	replayFolder  *systray.MenuItem
	codec         *systray.MenuItem
	inputGain     *systray.MenuItem
	outputGain    *systray.MenuItem
	noiseGate     *systray.MenuItem
	playback      *systray.MenuItem
	autoJoin      *systray.MenuItem
	endCall       *systray.MenuItem
	hotkeyRoot    *systray.MenuItem
	inputRoot     *systray.MenuItem
	refreshInput  *systray.MenuItem
	outputRoot    *systray.MenuItem
	refreshOutput *systray.MenuItem
	quit          *systray.MenuItem

	inputItems  map[string]*systray.MenuItem
	outputItems map[string]*systray.MenuItem
	hotkeyItems map[string]*systray.MenuItem
	last        app.Status
}

func Run(ctx context.Context, controller *app.Controller) {
	runner := &Runner{Controller: controller}
	systray.Run(
		func() { runner.ready(ctx) },
		func() {},
	)
}

func (r *Runner) ready(ctx context.Context) {
	setTrayIcon()
	systray.SetTooltip("LylatLink")

	m := &menu{
		controller:  r.Controller,
		inputItems:  map[string]*systray.MenuItem{},
		outputItems: map[string]*systray.MenuItem{},
		hotkeyItems: map[string]*systray.MenuItem{},
	}

	m.title = systray.AddMenuItem("LylatLink", "Open LylatLink project")
	systray.AddSeparator()
	m.status = systray.AddMenuItem("Status: Starting", "Current LylatLink status")
	m.status.Disable()
	m.match = systray.AddMenuItem("Match: none", "Current match")
	m.match.Disable()
	systray.AddSeparator()
	m.autoJoin = systray.AddMenuItemCheckbox("Auto-join voice on match", "Automatically join voice when a match pairs", false)
	m.endCall = systray.AddMenuItem("End Call", "End the current voice session")
	m.endCall.Disable()
	m.hotkeyRoot = systray.AddMenuItem("End Call Hotkey", "Choose the global end-call hotkey")
	m.addHotkeyItems()
	systray.AddSeparator()
	m.inputRoot = systray.AddMenuItem("Input Device", "Choose microphone input")
	m.refreshInput = m.inputRoot.AddSubMenuItem("Refresh Devices", "Refresh input devices")
	m.outputRoot = systray.AddMenuItem("Output Device", "Choose remote audio playback output")
	m.refreshOutput = m.outputRoot.AddSubMenuItem("Refresh Devices", "Refresh output devices")
	systray.AddSeparator()
	m.replayFolder = systray.AddMenuItem("Replay Folder: unset", "Configured replay folder")
	m.editConfig = systray.AddMenuItem("Edit Config File", "Open LylatLink config file")
	m.codec = systray.AddMenuItem("Codec: opus", "Configured voice codec")
	m.codec.Disable()
	m.inputGain = systray.AddMenuItem("Input Gain: 0.0 dB", "Configured microphone gain")
	m.inputGain.Disable()
	m.outputGain = systray.AddMenuItem("Output Gain: -1.5 dB", "Configured remote playback gain")
	m.outputGain.Disable()
	m.noiseGate = systray.AddMenuItem("Noise Gate: -45.0 dBFS", "Configured microphone noise gate threshold")
	m.noiseGate.Disable()
	m.playback = systray.AddMenuItem("Playback: on", "Remote speaker playback state")
	m.playback.Disable()
	systray.AddSeparator()
	m.quit = systray.AddMenuItem("Quit", "Quit LylatLink")

	m.update(r.Controller.Snapshot())
	m.refreshInputs()
	m.refreshOutputs()

	go m.handleTitle()
	go m.listenStatus(ctx)
	go m.handleAutoJoin()
	go m.handleEndCall()
	go m.handleHotkeys()
	go m.handleRefreshInputs()
	go m.handleRefreshOutputs()
	go m.handleReplayFolder()
	go m.handleEditConfig()
	go m.handleQuit()
	go func() {
		<-ctx.Done()
		systray.Quit()
	}()
}

func setTrayIcon() {
	switch runtime.GOOS {
	case "darwin":
		systray.SetTemplateIcon(assets.MacOSTrayTemplate32PNG, assets.IconPNG)
	case "windows":
		systray.SetIcon(assets.IconICO)
	default:
		systray.SetIcon(assets.IconPNG)
	}
}

func (m *menu) listenStatus(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case status := <-m.controller.Status():
			m.update(status)
		}
	}
}

func (m *menu) handleTitle() {
	for range m.title.ClickedCh {
		if err := openExternal(projectURL); err != nil {
			log.Printf("open project URL failed: %v", err)
		}
	}
}

func (m *menu) handleAutoJoin() {
	for range m.autoJoin.ClickedCh {
		next := !m.last.AutoJoin
		m.controller.SetAutoJoin(next)
		m.last.AutoJoin = next
		m.applyAutoJoin(next)
	}
}

func (m *menu) handleEndCall() {
	for range m.endCall.ClickedCh {
		m.controller.EndCall()
	}
}

func (m *menu) handleHotkeys() {
	for key, item := range m.hotkeyItems {
		key := key
		item := item
		go func() {
			for range item.ClickedCh {
				m.controller.SetEndCallHotkey(key)
				m.last.EndCallHotkey = key
				m.applyEndCallHotkey(key)
				m.endCall.SetTitle(endCallTitle(key))
			}
		}()
	}
}

func (m *menu) handleRefreshInputs() {
	for range m.refreshInput.ClickedCh {
		m.refreshInputs()
	}
}

func (m *menu) handleRefreshOutputs() {
	for range m.refreshOutput.ClickedCh {
		m.refreshOutputs()
	}
}

func (m *menu) handleReplayFolder() {
	for range m.replayFolder.ClickedCh {
		builder := dialog.Directory().Title("Choose Slippi Replay Folder")
		if m.last.ReplayDir != "" {
			builder = builder.SetStartDir(m.last.ReplayDir)
		}
		path, err := builder.Browse()
		if err != nil {
			if err != dialog.Cancelled {
				log.Printf("choose replay folder failed: %v", err)
			}
			continue
		}
		if path != "" {
			m.controller.SetReplayDir(path)
			m.last.ReplayDir = path
			m.replayFolder.SetTitle(replayTitle(path))
		}
	}
}

func (m *menu) handleEditConfig() {
	for range m.editConfig.ClickedCh {
		path := m.controller.ConfigPath()
		if path == "" {
			log.Printf("config path is not available")
			continue
		}
		if err := openExternal(path); err != nil {
			log.Printf("open config file failed: %v", err)
		}
	}
}

func (m *menu) handleQuit() {
	for range m.quit.ClickedCh {
		systray.Quit()
		return
	}
}

func (m *menu) update(status app.Status) {
	m.last = status
	systray.SetTooltip(fmt.Sprintf("LylatLink - %s", statusTitle(status)))
	m.status.SetTitle(fmt.Sprintf("Status: %s", statusTitle(status)))
	m.match.SetTitle(matchTitle(status))
	m.replayFolder.SetTitle(replayTitle(status.ReplayDir))
	m.codec.SetTitle(fmt.Sprintf("Codec: %s", status.AudioCodec))
	m.inputGain.SetTitle(fmt.Sprintf("Input Gain: %.1f dB", status.InputGainDB))
	m.outputGain.SetTitle(fmt.Sprintf("Output Gain: %.1f dB", status.OutputGainDB))
	m.noiseGate.SetTitle(fmt.Sprintf("Noise Gate: %.1f dBFS", status.NoiseGateDB))
	if status.NoPlayback {
		m.playback.SetTitle("Playback: off")
	} else {
		m.playback.SetTitle("Playback: on")
	}
	m.applyAutoJoin(status.AutoJoin)
	m.applyEndCall(status)
	m.applyEndCallHotkey(status.EndCallHotkey)
	m.applyInputChecks()
	m.applyOutputChecks()
}

func (m *menu) applyAutoJoin(enabled bool) {
	if enabled {
		m.autoJoin.Check()
	} else {
		m.autoJoin.Uncheck()
	}
}

func (m *menu) applyEndCall(status app.Status) {
	m.endCall.SetTitle(endCallTitle(status.EndCallHotkey))
	switch status.State {
	case app.StateInVoice, app.StateWaiting:
		m.endCall.Enable()
	default:
		m.endCall.Disable()
	}
}

func (m *menu) addHotkeyItems() {
	for _, choice := range hotkey.Choices() {
		tooltip := "Use " + choice.Label + " to end calls"
		if choice.Key == "" {
			tooltip = "Disable the global end-call hotkey"
		}
		item := m.hotkeyRoot.AddSubMenuItemCheckbox(choice.Label, tooltip, false)
		m.hotkeyItems[choice.Key] = item
	}
}

func (m *menu) applyEndCallHotkey(key string) {
	key = hotkey.NormalizeKey(key)
	m.hotkeyRoot.SetTitle("End Call Hotkey: " + hotkey.Label(key))
	for itemKey, item := range m.hotkeyItems {
		if itemKey == key {
			item.Check()
		} else {
			item.Uncheck()
		}
	}
}

func (m *menu) refreshInputs() {
	for _, item := range m.inputItems {
		item.Hide()
	}
	m.inputItems = map[string]*systray.MenuItem{}

	m.addInputItem("", "System Default", m.last.InputDeviceID == "")

	devices, err := audio.ListInputDevices()
	if err != nil {
		log.Printf("list input devices failed: %v", err)
		item := m.inputRoot.AddSubMenuItem("Could not list devices", err.Error())
		item.Disable()
		return
	}

	sort.Slice(devices, func(i, j int) bool {
		if devices[i].IsDefault != devices[j].IsDefault {
			return devices[i].IsDefault
		}
		return devices[i].Name < devices[j].Name
	})
	for _, device := range devices {
		title := device.Name
		if device.IsDefault {
			title += " (default)"
		}
		m.addInputItem(device.ID, title, m.last.InputDeviceID == device.ID)
	}
}

func (m *menu) refreshOutputs() {
	for _, item := range m.outputItems {
		item.Hide()
	}
	m.outputItems = map[string]*systray.MenuItem{}

	m.addOutputItem("", "System Default", m.last.OutputDeviceID == "")

	devices, err := audio.ListOutputDevices()
	if err != nil {
		log.Printf("list output devices failed: %v", err)
		item := m.outputRoot.AddSubMenuItem("Could not list devices", err.Error())
		item.Disable()
		return
	}

	sort.Slice(devices, func(i, j int) bool {
		if devices[i].IsDefault != devices[j].IsDefault {
			return devices[i].IsDefault
		}
		return devices[i].Name < devices[j].Name
	})
	for _, device := range devices {
		title := device.Name
		if device.IsDefault {
			title += " (default)"
		}
		m.addOutputItem(device.ID, title, m.last.OutputDeviceID == device.ID)
	}
}

func (m *menu) addInputItem(id string, title string, checked bool) {
	item := m.inputRoot.AddSubMenuItemCheckbox(title, title, checked)
	m.inputItems[id] = item
	if checked {
		item.Check()
	}
	go func() {
		for range item.ClickedCh {
			m.controller.SetInputDeviceID(id)
			m.last.InputDeviceID = id
			m.applyInputChecks()
		}
	}()
}

func (m *menu) addOutputItem(id string, title string, checked bool) {
	item := m.outputRoot.AddSubMenuItemCheckbox(title, title, checked)
	m.outputItems[id] = item
	if checked {
		item.Check()
	}
	go func() {
		for range item.ClickedCh {
			m.controller.SetOutputDeviceID(id)
			m.last.OutputDeviceID = id
			m.applyOutputChecks()
		}
	}()
}

func (m *menu) applyInputChecks() {
	for id, item := range m.inputItems {
		if id == m.last.InputDeviceID {
			item.Check()
		} else {
			item.Uncheck()
		}
	}
}

func (m *menu) applyOutputChecks() {
	for id, item := range m.outputItems {
		if id == m.last.OutputDeviceID {
			item.Check()
		} else {
			item.Uncheck()
		}
	}
}

func openExternal(target string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", target).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", "", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}

func statusTitle(status app.Status) string {
	switch status.State {
	case app.StateWatching:
		if status.Message != "" {
			return menuStatusText(status.Message)
		}
		return "Watching"
	case app.StateWaiting:
		if status.Message != "" {
			return menuStatusText(status.Message)
		}
		return "Waiting for other player"
	case app.StateInVoice:
		return "In Voice"
	case app.StateNotReady:
		return "Not Ready"
	case app.StateError:
		return friendlyErrorTitle(status.Message)
	case app.StateShuttingDown:
		return "Shutting Down"
	default:
		return string(status.State)
	}
}

func friendlyErrorTitle(message string) string {
	if message == "" {
		return "Issue detected"
	}
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "http"),
		strings.Contains(lower, "post "),
		strings.Contains(lower, "websocket"),
		strings.Contains(lower, "connection refused"),
		strings.Contains(lower, "connection reset"),
		strings.Contains(lower, "no such host"),
		strings.Contains(lower, "deadline exceeded"),
		strings.Contains(lower, "timeout"):
		return "Connection issue"
	case strings.Contains(lower, "replay folder"),
		strings.Contains(lower, "replay directory"):
		return "Replay folder issue"
	case strings.Contains(lower, "input device"),
		strings.Contains(lower, "microphone"),
		strings.Contains(lower, "audio"):
		return "Audio issue"
	default:
		return "Issue detected"
	}
}

func menuStatusText(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	const maxLen = 48
	if len(text) <= maxLen {
		return text
	}
	return strings.TrimSpace(text[:maxLen-3]) + "..."
}

func matchTitle(status app.Status) string {
	if status.MatchLabel != "" {
		return "Match: " + status.MatchLabel
	}
	return "Match: none"
}

func replayTitle(path string) string {
	if path == "" {
		return "Replay Folder: unset"
	}
	return "Replay Folder: " + compactPath(path, 58)
}

func endCallTitle(key string) string {
	key = hotkey.NormalizeKey(key)
	if key == "" {
		return "End Call"
	}
	return "End Call (" + hotkey.Label(key) + ")"
}

func compactPath(path string, maxLen int) string {
	path = strings.TrimSpace(path)
	if len(path) <= maxLen {
		return path
	}

	base, separator := pathBase(path)

	const ellipsis = "..."
	if base == "" || len(base)+len(ellipsis)+1 >= maxLen {
		return strings.TrimSpace(path[:maxLen-len(ellipsis)]) + ellipsis
	}

	prefixLen := maxLen - len(base) - len(ellipsis) - 1
	if prefixLen < 1 {
		prefixLen = 1
	}
	return strings.TrimRight(path[:prefixLen], `/\`) + ellipsis + separator + base
}

func pathBase(path string) (string, string) {
	trimmed := strings.TrimRight(path, `/\`)
	lastSlash := strings.LastIndex(trimmed, "/")
	lastBackslash := strings.LastIndex(trimmed, `\`)
	lastSeparator := lastSlash
	separator := "/"
	if lastBackslash > lastSlash {
		lastSeparator = lastBackslash
		separator = `\`
	}
	if lastSeparator == -1 {
		return filepath.Base(trimmed), separator
	}
	return trimmed[lastSeparator+1:], separator
}
