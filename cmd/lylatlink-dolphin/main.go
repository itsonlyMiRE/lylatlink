package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		notify("LylatLink", err.Error())
		os.Exit(1)
	}
}

func run() error {
	launcherDir, err := executableDir()
	if err != nil {
		return err
	}

	lylatPath, err := lylatlinkPath(launcherDir)
	if err != nil {
		return err
	}

	dolphinPath, err := findNetplayDolphin()
	if err != nil {
		return err
	}

	if !lylatlinkAlreadyRunning() {
		if err := startDetached(lylatPath); err != nil {
			return fmt.Errorf("start LylatLink: %w", err)
		}
	}

	time.Sleep(500 * time.Millisecond)
	if err := startDetached(dolphinPath); err != nil {
		return fmt.Errorf("start Slippi Dolphin: %w", err)
	}
	return nil
}

func executableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve launcher path: %w", err)
	}
	return filepath.Dir(exe), nil
}

func lylatlinkPath(launcherDir string) (string, error) {
	candidates := []string{
		filepath.Join(launcherDir, "lylatlink.exe"),
		filepath.Join(launcherDir, "LylatLink.app"),
	}
	for _, candidate := range candidates {
		if exists(candidate) {
			return candidate, nil
		}
	}
	return "", errors.New("could not find LylatLink next to this launcher")
}

func findNetplayDolphin() (string, error) {
	roots, err := slippiNetplayRoots()
	if err != nil {
		return "", err
	}

	for _, root := range roots {
		path, ok := findDolphinInRoot(root)
		if ok {
			return path, nil
		}
	}

	return "", fmt.Errorf("could not find Slippi netplay Dolphin under:\n\n%s\n\nOpen Slippi Launcher once and use Netplay > Configure Dolphin, then try this launcher again.", strings.Join(roots, "\n"))
}

func slippiNetplayRoots() ([]string, error) {
	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return nil, errors.New("APPDATA is not set")
		}
		return slippiNetplayRootsFrom(filepath.Join(appData, "Slippi Launcher")), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		return slippiNetplayRootsFrom(filepath.Join(home, "Library", "Application Support", "Slippi Launcher")), nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func slippiNetplayRootsFrom(base string) []string {
	return []string{
		filepath.Join(base, "netplay"),
		filepath.Join(base, "netplay-beta"),
		filepath.Join(base, "netplay-legacy"),
	}
}

func findDolphinInRoot(root string) (string, bool) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}

	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if isDolphinExecutable(path, entry.IsDir()) {
			return path, true
		}
	}

	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || path == root {
			return nil
		}
		if rel, relErr := filepath.Rel(root, path); relErr == nil && strings.Count(rel, string(filepath.Separator)) > 2 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if isDolphinExecutable(path, d.IsDir()) {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found, found != ""
}

func isDolphinExecutable(path string, isDir bool) bool {
	name := filepath.Base(path)
	switch runtime.GOOS {
	case "windows":
		return !isDir && strings.HasSuffix(strings.ToLower(name), "dolphin.exe")
	case "darwin":
		return isDir && strings.HasSuffix(name, "Dolphin.app")
	default:
		return false
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func lylatlinkAlreadyRunning() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq lylatlink.exe").Output()
	return err == nil && strings.Contains(strings.ToLower(string(out)), "lylatlink.exe")
}

func startDetached(path string) error {
	if runtime.GOOS == "darwin" && strings.HasSuffix(path, ".app") {
		return exec.Command("open", path).Start()
	}
	cmd := exec.Command(path)
	cmd.Dir = filepath.Dir(path)
	return cmd.Start()
}
