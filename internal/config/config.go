package config

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/adrg/xdg"
)

const appConfigPath = "lylatlink/config.toml"

type Config struct {
	ReplayDir      string  `toml:"replay_dir"`
	AutoJoin       bool    `toml:"auto_join"`
	EndCallHotkey  string  `toml:"end_call_hotkey,omitempty"`
	InputDeviceID  string  `toml:"input_device_id,omitempty"`
	OutputDeviceID string  `toml:"output_device_id,omitempty"`
	AudioCodec     string  `toml:"audio_codec,omitempty"`
	InputGainDB    float64 `toml:"input_gain_db,omitempty"`
	OutputGainDB   float64 `toml:"output_gain_db,omitempty"`
	NoiseGateDB    float64 `toml:"noise_gate_threshold_db,omitempty"`
	SignalBaseURL  string  `toml:"-"`
}

func Default() Config {
	return Config{
		AutoJoin:      true,
		EndCallHotkey: "f8",
		AudioCodec:    "opus",
		OutputGainDB:  -1.5,
		NoiseGateDB:   -55,
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
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Config{}, err
	}
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}
