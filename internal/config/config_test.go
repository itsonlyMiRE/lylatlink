package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRepairsUnescapedWindowsReplayDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("replay_dir = \"C:\\Users\\mire\\Slippi\"\nauto_join = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ReplayDir != "C:/Users/mire/Slippi" {
		t.Fatalf("replay dir = %q", cfg.ReplayDir)
	}
}

func TestSaveNormalizesWindowsReplayDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	err := Save(path, Config{
		ReplayDir:     `C:\Users\mire\Slippi`,
		AutoJoin:      true,
		EndCallHotkey: "f8",
		AudioCodec:    "opus",
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, `C:\Users`) {
		t.Fatalf("config contains unescaped Windows path:\n%s", text)
	}
	if !strings.Contains(text, `replay_dir = "C:/Users/mire/Slippi"`) {
		t.Fatalf("config did not use normalized path:\n%s", text)
	}

	cfg, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ReplayDir != "C:/Users/mire/Slippi" {
		t.Fatalf("replay dir = %q", cfg.ReplayDir)
	}
}
