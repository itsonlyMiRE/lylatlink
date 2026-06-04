package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/adrg/xdg"

	"lylatlink/internal/hotkey"
)

const appConfigPath = "lylatlink/config.toml"

const configHeader = `# replay_dir is a fallback.
# Leave it empty unless LylatLink cannot auto-detect your Slippi replay folder.
`

type Config struct {
	ReplayDir             string  `toml:"replay_dir"`
	AutoJoin              bool    `toml:"auto_join"`
	PlayChimes            bool    `toml:"play_chimes"`
	EndCallHotkey         string  `toml:"end_call_hotkey"`
	InputDeviceID         string  `toml:"input_device_id,omitempty"`
	OutputDeviceID        string  `toml:"output_device_id,omitempty"`
	AudioCodec            string  `toml:"audio_codec,omitempty"`
	InputGainDB           float64 `toml:"input_gain_db,omitempty"`
	OutputGainDB          float64 `toml:"output_gain_db,omitempty"`
	NoiseGateDB           float64 `toml:"noise_gate_threshold_db,omitempty"`
	SignalBaseURL         string  `toml:"-"`
	ReplayDirAutoDetected bool    `toml:"-"`
}

func Default() Config {
	return Config{
		AutoJoin:      true,
		PlayChimes:    true,
		EndCallHotkey: "f8",
		AudioCodec:    "opus",
		OutputGainDB:  -1.5,
		NoiseGateDB:   -45,
		SignalBaseURL: "http://lylatlink.signal.mire.systems:8787",
	}
}

func DefaultPath() (string, error) {
	return xdg.ConfigFile(appConfigPath)
}

func LoadOrCreate(path string) (Config, error) {
	if path == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return Config{}, err
		}
	}

	cfg := Default()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := Save(path, cfg); err != nil {
			return Config{}, err
		}
		return cfg, nil
	}
	if err := decodeConfigFile(path, &cfg); err != nil {
		return Config{}, err
	}
	cfg.ReplayDir = normalizePathForConfig(cfg.ReplayDir)
	cfg.EndCallHotkey = hotkey.NormalizeKey(cfg.EndCallHotkey)
	if cfg.SignalBaseURL == "" {
		cfg.SignalBaseURL = Default().SignalBaseURL
	}
	if cfg.AudioCodec == "" {
		cfg.AudioCodec = Default().AudioCodec
	}
	if cfg.NoiseGateDB == 0 {
		cfg.NoiseGateDB = Default().NoiseGateDB
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	cfg.ReplayDir = normalizePathForConfig(cfg.ReplayDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(configHeader); err != nil {
		return err
	}
	return toml.NewEncoder(f).Encode(cfg)
}

func decodeConfigFile(path string, cfg *Config) error {
	if _, err := toml.DecodeFile(path, cfg); err == nil {
		return nil
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return readErr
	}
	repaired, ok := repairUnescapedWindowsReplayDir(string(data))
	if !ok {
		_, err := toml.Decode(string(data), cfg)
		return err
	}
	_, err := toml.Decode(repaired, cfg)
	return err
}

func repairUnescapedWindowsReplayDir(data string) (string, bool) {
	lines := strings.SplitAfter(data, "\n")
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "replay_dir") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		firstQuote := strings.Index(value, "\"")
		lastQuote := strings.LastIndex(value, "\"")
		if firstQuote < 0 || lastQuote <= firstQuote {
			continue
		}
		raw := value[firstQuote+1 : lastQuote]
		if !strings.Contains(raw, `\`) {
			continue
		}
		value = value[:firstQuote+1] + normalizePathForConfig(raw) + value[lastQuote:]
		lines[i] = key + "=" + value
		changed = true
	}
	return strings.Join(lines, ""), changed
}

func normalizePathForConfig(path string) string {
	return strings.ReplaceAll(path, `\`, `/`)
}
