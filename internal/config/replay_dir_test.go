package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveReplayDirUsesLauncherSettings(t *testing.T) {
	root := t.TempDir()
	replayDir := mkdir(t, root, "custom-replays")
	settingsDir := mkdir(t, root, "Library", "Application Support", slippiLauncherDirName)
	writeFile(t, filepath.Join(settingsDir, "Settings"), `{
  "settings": {
    "rootSlpPath": "`+slashPath(replayDir)+`",
    "enableNetplayReplays": true,
    "useMonthlySubfolders": false
  }
}`)

	res, err := replayDirResolver{home: root, goos: "darwin"}.resolve()
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != replayDir {
		t.Fatalf("path = %q, want %q", res.Path, replayDir)
	}
	if res.Source != "Slippi Launcher settings" {
		t.Fatalf("source = %q", res.Source)
	}
	if res.EnableNetplayReplays == nil || !*res.EnableNetplayReplays {
		t.Fatalf("expected enabled netplay replays")
	}
	if res.UseMonthlySubfolders == nil || *res.UseMonthlySubfolders {
		t.Fatalf("expected disabled monthly subfolders")
	}
}

func TestResolveReplayDirFallsBackToDolphinINI(t *testing.T) {
	root := t.TempDir()
	replayDir := mkdir(t, root, "dolphin-replays")
	settingsDir := mkdir(t, root, "Library", "Application Support", slippiLauncherDirName)
	writeFile(t, filepath.Join(settingsDir, "Settings"), `{"settings":{"isoPath":"/tmp/melee.iso"}}`)
	configDir := mkdir(t, root, "Library", "Application Support", "com.project-slippi.dolphin", "netplay", "User", "Config")
	writeFile(t, filepath.Join(configDir, "Dolphin.ini"), `[Core]
SlippiReplayDir = `+replayDir+`
SlippiSaveReplays = True
SlippiReplayMonthFolders = True
`)

	res, err := replayDirResolver{home: root, goos: "darwin"}.resolve()
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != replayDir {
		t.Fatalf("path = %q, want %q", res.Path, replayDir)
	}
	if res.Source != "Slippi Dolphin.ini" {
		t.Fatalf("source = %q", res.Source)
	}
	if res.UseMonthlySubfolders == nil || !*res.UseMonthlySubfolders {
		t.Fatalf("expected monthly subfolders from Dolphin.ini")
	}
}

func TestResolveReplayDirFallsBackToDefaultFolder(t *testing.T) {
	root := t.TempDir()
	defaultDir := mkdir(t, root, "Slippi")

	res, err := replayDirResolver{home: root, goos: "darwin"}.resolve()
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != defaultDir {
		t.Fatalf("path = %q, want %q", res.Path, defaultDir)
	}
	if res.Source != "default Slippi folder" {
		t.Fatalf("source = %q", res.Source)
	}
}

func TestResolveReplayDirIgnoresMissingConfiguredPaths(t *testing.T) {
	root := t.TempDir()
	defaultDir := mkdir(t, root, "Slippi")
	settingsDir := mkdir(t, root, "Library", "Application Support", slippiLauncherDirName)
	writeFile(t, filepath.Join(settingsDir, "Settings"), `{"settings":{"rootSlpPath":"`+slashPath(filepath.Join(root, "missing"))+`"}}`)

	res, err := replayDirResolver{home: root, goos: "darwin"}.resolve()
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != defaultDir {
		t.Fatalf("path = %q, want fallback %q", res.Path, defaultDir)
	}
}

func mkdir(t *testing.T, root string, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{root}, parts...)...)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func slashPath(path string) string {
	return filepath.ToSlash(path)
}
