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

type launchTarget struct {
	Path        string
	AppBundle   bool
	ProcessName string
}

type launchWatch struct {
	PID         int
	ProcessName string
}

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

	dolphin, err := findNetplayDolphin()
	if err != nil {
		return err
	}

	watch, err := startDetached(dolphin)
	if err != nil {
		return fmt.Errorf("start Slippi Dolphin: %w", err)
	}

	if !lylatlinkAlreadyRunning() {
		if _, err := startLylatLink(lylatPath, watch); err != nil {
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
		filepath.Join(launcherDir, "..", "Resources", "LylatLink.app"),
		filepath.Join(launcherDir, "lylatlink.exe"),
		filepath.Join(launcherDir, "LylatLink.app"),
		filepath.Join(appBundleRoot, "..", "LylatLink.app"),
		filepath.Join(appBundleRoot, "LylatLink.app"),
	}
	for _, candidate := range candidates {
		if exists(candidate) {
			return candidate, nil
		}
	}
	return "", errors.New("could not find LylatLink next to this launcher")
}

func findNetplayDolphin() (launchTarget, error) {
	roots, err := slippiNetplayRoots()
	if err != nil {
		return launchTarget{}, err
	}

	for _, root := range roots {
		target, ok := findDolphinInRoot(root)
		if ok {
			return target, nil
		}
	}

	return launchTarget{}, fmt.Errorf("could not find Slippi netplay Dolphin under:\n\n%s\n\nOpen Slippi Launcher once and use Netplay > Configure Dolphin, then try this launcher again.", strings.Join(roots, "\n"))
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

func findDolphinInRoot(root string) (launchTarget, bool) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return launchTarget{}, false
	}

	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if target, ok := dolphinLaunchTarget(path, entry.IsDir()); ok {
			return target, true
		}
	}

	var found launchTarget
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
		if target, ok := dolphinLaunchTarget(path, d.IsDir()); ok {
			found = target
			return filepath.SkipAll
		}
		return nil
	})
	return found, found.Path != ""
}

func dolphinLaunchTarget(path string, isDir bool) (launchTarget, bool) {
	name := filepath.Base(path)
	switch runtime.GOOS {
	case "windows":
		if !isDir && strings.HasSuffix(strings.ToLower(name), "dolphin.exe") {
			return launchTarget{Path: path}, true
		}
	case "darwin":
		if isDir && strings.HasSuffix(name, "Dolphin.app") {
			if processName, ok := dolphinProcessNameInApp(path); ok {
				return launchTarget{Path: path, AppBundle: true, ProcessName: processName}, true
			}
		}
	}
	return launchTarget{}, false
}

func dolphinProcessNameInApp(appPath string) (string, bool) {
	candidates := []string{
		"Slippi_Dolphin",
		"Slippi Dolphin",
	}
	macosDir := filepath.Join(appPath, "Contents", "MacOS")
	for _, candidate := range candidates {
		if exists(filepath.Join(macosDir, candidate)) {
			return candidate, true
		}
	}

	entries, err := os.ReadDir(macosDir)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			return entry.Name(), true
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

func startLylatLink(path string, watch launchWatch) (*exec.Cmd, error) {
	args := watchArgs(watch)
	if runtime.GOOS == "darwin" && strings.HasSuffix(path, ".app") {
		openArgs := append([]string{path, "--args"}, args...)
		cmd := exec.Command("open", openArgs...)
		return cmd, cmd.Start()
	}
	cmd := exec.Command(path, args...)
	cmd.Dir = filepath.Dir(path)
	return cmd, cmd.Start()
}

func watchArgs(watch launchWatch) []string {
	if watch.ProcessName != "" {
		return []string{"-exit-when-process-name", watch.ProcessName}
	}
	return []string{"-exit-when-pid", fmt.Sprintf("%d", watch.PID)}
}

func startDetached(target launchTarget) (launchWatch, error) {
	if runtime.GOOS == "darwin" && target.AppBundle {
		cmd := exec.Command("open", "-W", target.Path)
		if err := cmd.Start(); err != nil {
			return launchWatch{}, err
		}
		return launchWatch{PID: cmd.Process.Pid}, nil
	}

	cmd := exec.Command(target.Path)
	cmd.Dir = filepath.Dir(target.Path)
	if err := cmd.Start(); err != nil {
		return launchWatch{}, err
	}
	return launchWatch{PID: cmd.Process.Pid}, nil
}
