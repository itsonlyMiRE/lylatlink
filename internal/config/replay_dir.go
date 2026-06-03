package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const slippiLauncherDirName = "Slippi Launcher"

type ReplayDirResolution struct {
	Path                 string
	Source               string
	EnableNetplayReplays *bool
	UseMonthlySubfolders *bool
}

type replayDirResolver struct {
	home      string
	appData   string
	configDir string
	documents string
	goos      string
}

type slippiLauncherSettings struct {
	Settings struct {
		RootSLPPath          *string `json:"rootSlpPath"`
		EnableNetplayReplays *bool   `json:"enableNetplayReplays"`
		UseMonthlySubfolders *bool   `json:"useMonthlySubfolders"`
	} `json:"settings"`
}

func ResolveReplayDir() (ReplayDirResolution, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return ReplayDirResolution{}, err
	}
	resolver := replayDirResolver{
		home:      home,
		appData:   os.Getenv("APPDATA"),
		configDir: os.Getenv("XDG_CONFIG_HOME"),
		documents: filepath.Join(home, "Documents"),
		goos:      runtime.GOOS,
	}
	return resolver.resolve()
}

func (r replayDirResolver) resolve() (ReplayDirResolution, error) {
	if res, ok := r.fromLauncherSettings(); ok {
		return res, nil
	}
	if res, ok := r.fromDolphinINI(); ok {
		return res, nil
	}
	if pathExistsDir(r.defaultRootSLPPath()) {
		return ReplayDirResolution{Path: r.defaultRootSLPPath(), Source: "default Slippi folder"}, nil
	}
	return ReplayDirResolution{}, errors.New("could not auto-detect Slippi replay folder")
}

func (r replayDirResolver) fromLauncherSettings() (ReplayDirResolution, bool) {
	settingsPath := r.launcherSettingsPath()
	if settingsPath == "" {
		return ReplayDirResolution{}, false
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return ReplayDirResolution{}, false
	}
	var settings slippiLauncherSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return ReplayDirResolution{}, false
	}
	if settings.Settings.RootSLPPath == nil || strings.TrimSpace(*settings.Settings.RootSLPPath) == "" {
		return ReplayDirResolution{}, false
	}
	path := filepath.Clean(*settings.Settings.RootSLPPath)
	if !pathExistsDir(path) {
		return ReplayDirResolution{}, false
	}
	return ReplayDirResolution{
		Path:                 path,
		Source:               "Slippi Launcher settings",
		EnableNetplayReplays: settings.Settings.EnableNetplayReplays,
		UseMonthlySubfolders: settings.Settings.UseMonthlySubfolders,
	}, true
}

func (r replayDirResolver) fromDolphinINI() (ReplayDirResolution, bool) {
	for _, iniPath := range r.dolphinINIPaths() {
		ini, err := readINI(iniPath)
		if err != nil {
			continue
		}
		if res, ok := replayDirFromMainlineINI(ini); ok && pathExistsDir(res.Path) {
			res.Source = "Slippi Dolphin.ini"
			return res, true
		}
		if res, ok := replayDirFromIshiiINI(ini); ok && pathExistsDir(res.Path) {
			res.Source = "Slippi Dolphin.ini"
			return res, true
		}
	}
	return ReplayDirResolution{}, false
}

func replayDirFromMainlineINI(ini map[string]map[string]string) (ReplayDirResolution, bool) {
	section := ini["Slippi"]
	if section == nil {
		return ReplayDirResolution{}, false
	}
	path := strings.TrimSpace(section["ReplayDir"])
	if path == "" {
		return ReplayDirResolution{}, false
	}
	return ReplayDirResolution{
		Path:                 filepath.Clean(path),
		EnableNetplayReplays: iniBoolPtr(section["SaveReplays"]),
		UseMonthlySubfolders: iniBoolPtr(section["ReplayMonthlyFolders"]),
	}, true
}

func replayDirFromIshiiINI(ini map[string]map[string]string) (ReplayDirResolution, bool) {
	section := ini["Core"]
	if section == nil {
		return ReplayDirResolution{}, false
	}
	path := strings.TrimSpace(section["SlippiReplayDir"])
	if path == "" {
		return ReplayDirResolution{}, false
	}
	return ReplayDirResolution{
		Path:                 filepath.Clean(path),
		EnableNetplayReplays: iniBoolPtr(section["SlippiSaveReplays"]),
		UseMonthlySubfolders: iniBoolPtr(section["SlippiReplayMonthFolders"]),
	}, true
}

func (r replayDirResolver) launcherSettingsPath() string {
	switch r.goos {
	case "darwin":
		return filepath.Join(r.home, "Library", "Application Support", slippiLauncherDirName, "Settings")
	case "windows":
		if r.appData == "" {
			return ""
		}
		return filepath.Join(r.appData, slippiLauncherDirName, "Settings")
	case "linux":
		configDir := r.configDir
		if configDir == "" {
			configDir = filepath.Join(r.home, ".config")
		}
		return filepath.Join(configDir, slippiLauncherDirName, "Settings")
	default:
		return ""
	}
}

func (r replayDirResolver) dolphinINIPaths() []string {
	switch r.goos {
	case "darwin":
		root := filepath.Join(r.home, "Library", "Application Support", "com.project-slippi.dolphin")
		return []string{
			filepath.Join(root, "netplay", "User", "Config", "Dolphin.ini"),
			filepath.Join(root, "netplay-beta", "User", "Config", "Dolphin.ini"),
			filepath.Join(root, "netplay-legacy", "User", "Config", "Dolphin.ini"),
		}
	case "windows":
		if r.appData == "" {
			return nil
		}
		root := filepath.Join(r.appData, slippiLauncherDirName)
		return []string{
			filepath.Join(root, "netplay", "User", "Config", "Dolphin.ini"),
			filepath.Join(root, "netplay-beta", "User", "Config", "Dolphin.ini"),
			filepath.Join(root, "netplay-legacy", "User", "Config", "Dolphin.ini"),
		}
	case "linux":
		configDir := r.configDir
		if configDir == "" {
			configDir = filepath.Join(r.home, ".config")
		}
		return []string{
			filepath.Join(configDir, "slippi-dolphin", "netplay", "User", "Config", "Dolphin.ini"),
			filepath.Join(configDir, "slippi-dolphin", "netplay-beta", "User", "Config", "Dolphin.ini"),
			filepath.Join(configDir, "slippi-dolphin", "netplay-legacy", "User", "Config", "Dolphin.ini"),
			filepath.Join(configDir, "SlippiOnline", "User", "Config", "Dolphin.ini"),
		}
	default:
		return nil
	}
}

func (r replayDirResolver) defaultRootSLPPath() string {
	if r.goos == "windows" && r.documents != "" {
		return filepath.Join(r.documents, "Slippi")
	}
	return filepath.Join(r.home, "Slippi")
}

func readINI(path string) (map[string]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ini := map[string]map[string]string{}
	sectionName := ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.Contains(line, "]") {
			end := strings.Index(line, "]")
			sectionName = strings.TrimSpace(line[1:end])
			if sectionName != "" && ini[sectionName] == nil {
				ini[sectionName] = map[string]string{}
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || sectionName == "" {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		ini[sectionName][key] = strings.TrimSpace(value)
	}
	return ini, scanner.Err()
}

func iniBoolPtr(value string) *bool {
	if value == "" {
		return nil
	}
	enabled := strings.EqualFold(strings.TrimSpace(value), "true")
	return &enabled
}

func pathExistsDir(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
