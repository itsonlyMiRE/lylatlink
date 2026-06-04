package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

	dolphinCmd, err := startDetached(dolphinPath)
	if err != nil {
		return fmt.Errorf("start Slippi Dolphin: %w", err)
	}

	if !lylatlinkAlreadyRunning() {
		if _, err := startLylatLink(lylatPath, dolphinCmd.Process.Pid); err != nil {
			return fmt.Errorf("start LylatLink: %w", err)
		}
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
	appBundleRoot := filepath.Clean(filepath.Join(launcherDir, "..", "..", ".."))
	candidates := []string{
		filepath.Join(launcherDir, "lylatlink.exe"),
		filepath.Join(launcherDir, "LylatLink.app"),
		filepath.Join(appBundleRoot, "LylatLink.app"),
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
		if dolphinPath, ok := dolphinExecutablePath(path, entry.IsDir()); ok {
			return dolphinPath, true
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
		if dolphinPath, ok := dolphinExecutablePath(path, d.IsDir()); ok {
			found = dolphinPath
			return filepath.SkipAll
		}
		return nil
	})
	return found, found != ""
}

func dolphinExecutablePath(path string, isDir bool) (string, bool) {
	name := filepath.Base(path)
	switch runtime.GOOS {
	case "windows":
		if !isDir && strings.HasSuffix(strings.ToLower(name), "dolphin.exe") {
			return path, true
		}
	case "darwin":
		if isDir && strings.HasSuffix(name, "Dolphin.app") {
			return dolphinBinaryInApp(path)
		}
	}
	return "", false
}

func dolphinBinaryInApp(appPath string) (string, bool) {
	candidates := []string{
		filepath.Join(appPath, "Contents", "MacOS", "Slippi_Dolphin"),
		filepath.Join(appPath, "Contents", "MacOS", "Slippi Dolphin"),
	}
	for _, candidate := range candidates {
		if exists(candidate) {
			return candidate, true
		}
	}

	macosDir := filepath.Join(appPath, "Contents", "MacOS")
	entries, err := os.ReadDir(macosDir)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			return filepath.Join(macosDir, entry.Name()), true
		}
	}
	return "", false
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func lylatlinkAlreadyRunning() bool {
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq lylatlink.exe").Output()
		return err == nil && strings.Contains(strings.ToLower(string(out)), "lylatlink.exe")
	case "darwin":
		err := exec.Command("pgrep", "-x", "LylatLink").Run()
		return err == nil
	default:
		return false
	}
}

func startLylatLink(path string, dolphinPID int) (*exec.Cmd, error) {
	if runtime.GOOS == "darwin" && strings.HasSuffix(path, ".app") {
		cmd := exec.Command("open", path, "--args", "-exit-when-pid", fmt.Sprintf("%d", dolphinPID))
		return cmd, cmd.Start()
	}
	cmd := exec.Command(path, "-exit-when-pid", fmt.Sprintf("%d", dolphinPID))
	cmd.Dir = filepath.Dir(path)
	return cmd, cmd.Start()
}

func startDetached(path string) (*exec.Cmd, error) {
	cmd := exec.Command(path)
	cmd.Dir = filepath.Dir(path)
	return cmd, cmd.Start()
}
