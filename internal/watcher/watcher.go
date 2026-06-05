package watcher

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"lylatlink/internal/slp"
)

type EventType string

const (
	EventMatchStart EventType = "match_start"
	EventMatchEnd   EventType = "match_end"
)

type MatchEvent struct {
	Type  EventType
	Path  string
	Match slp.Match
}

type Watcher struct {
	ReplayDir       string
	Debounce        time.Duration
	StableThreshold time.Duration
	StartupScanAge  time.Duration
}

type fileState struct {
	matchID  string
	started  bool
	ended    bool
	lastSize int64
}

func New(replayDir string) *Watcher {
	return &Watcher{
		ReplayDir:       replayDir,
		Debounce:        time.Second,
		StableThreshold: 10 * time.Minute,
		StartupScanAge:  30 * time.Second,
	}
}

func (w *Watcher) Run(ctx context.Context, out chan<- MatchEvent) error {
	if w.ReplayDir == "" {
		return errors.New("replay directory is required")
	}

	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer fsWatcher.Close()

	if err := addRecursive(fsWatcher, w.ReplayDir); err != nil {
		return err
	}

	states := map[string]*fileState{}
	timers := map[string]*time.Timer{}
	tracked := map[string]struct{}{}
	var inspectMu sync.Mutex
	inspect := func(path string) {
		inspectMu.Lock()
		defer inspectMu.Unlock()
		w.inspect(ctx, path, states, out)
	}
	track := func(path string) {
		if !isSLP(path) {
			return
		}
		tracked[path] = struct{}{}
		schedule(ctx, timers, path, w.Debounce, inspect)
	}
	w.scheduleSLPScan(ctx, w.ReplayDir, track)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-fsWatcher.Events:
			if !ok {
				return nil
			}
			if event.Op&fsnotify.Create != 0 {
				if addDirectoryIfNeeded(fsWatcher, event.Name) {
					w.scheduleSLPScan(ctx, event.Name, track)
				}
			}
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) == 0 || !isSLP(event.Name) {
				continue
			}
			track(event.Name)
		case err, ok := <-fsWatcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("watcher error: %v", err)
		case <-ticker.C:
			for path := range tracked {
				inspect(path)
				inspectMu.Lock()
				state := states[path]
				ended := state != nil && state.ended
				inspectMu.Unlock()
				if ended {
					delete(tracked, path)
				}
			}
		}
	}
}

func (w *Watcher) scheduleSLPScan(ctx context.Context, root string, track func(string)) {
	paths, err := collectRecentSLPFiles(root, w.StartupScanAge)
	if err != nil {
		log.Printf("scan replay files in %s: %v", root, err)
		return
	}
	for _, path := range paths {
		select {
		case <-ctx.Done():
			return
		default:
			track(path)
		}
	}
}

func (w *Watcher) inspect(ctx context.Context, path string, states map[string]*fileState, out chan<- MatchEvent) {
	if !isSLP(path) {
		return
	}

	match, err := slp.ParseFile(path)
	if err != nil {
		if !errors.Is(err, slp.ErrNoGameStart) && !errors.Is(err, slp.ErrNoEventPayloads) {
			log.Printf("parse %s: %v", filepath.Base(path), err)
		}
		return
	}
	if match.MatchID == "" || !hasPairablePlayerCodes(match.PlayerCodes) {
		return
	}

	state := states[path]
	if state == nil {
		state = &fileState{}
		states[path] = state
	}
	if state.matchID == "" {
		state.matchID = match.MatchID
	}

	if !state.started {
		state.started = true
		emit(ctx, out, MatchEvent{Type: EventMatchStart, Path: path, Match: *match})
	}

	if match.GameEnded && !state.ended {
		state.ended = true
		emit(ctx, out, MatchEvent{Type: EventMatchEnd, Path: path, Match: *match})
		return
	}

	w.detectStableEnd(ctx, path, state, match, out)
}

func hasPairablePlayerCodes(codes []string) bool {
	return len(codes) >= 1 && len(codes) <= 2
}

func (w *Watcher) detectStableEnd(ctx context.Context, path string, state *fileState, match *slp.Match, out chan<- MatchEvent) {
	if !state.started || state.ended {
		return
	}

	// Normal game-end events close calls immediately. This fallback only catches
	// abnormal exits where Slippi stops writing without recording Game End; keep
	// it conservative so in-game pauses do not look like ended matches.
	info, err := os.Stat(path)
	if err != nil {
		return
	}

	size := info.Size()
	now := time.Now()
	if size != state.lastSize {
		state.lastSize = size
		return
	}
	if now.Sub(info.ModTime()) >= w.StableThreshold {
		state.ended = true
		ended := *match
		ended.GameEnded = true
		emit(ctx, out, MatchEvent{Type: EventMatchEnd, Path: path, Match: ended})
	}
}

func schedule(ctx context.Context, timers map[string]*time.Timer, path string, delay time.Duration, fn func(string)) {
	if timer := timers[path]; timer != nil {
		timer.Stop()
	}
	timers[path] = time.AfterFunc(delay, func() {
		select {
		case <-ctx.Done():
			return
		default:
			fn(path)
		}
	})
}

func emit(ctx context.Context, out chan<- MatchEvent, event MatchEvent) {
	select {
	case <-ctx.Done():
	case out <- event:
	}
}

func isSLP(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".slp")
}

func addRecursive(fsWatcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return fsWatcher.Add(path)
		}
		return nil
	})
}

func collectRecentSLPFiles(root string, maxAge time.Duration) ([]string, error) {
	paths := []string{}
	cutoff := time.Time{}
	if maxAge > 0 {
		cutoff = time.Now().Add(-maxAge)
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isSLP(path) {
			return nil
		}
		if !cutoff.IsZero() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.ModTime().Before(cutoff) {
				return nil
			}
		}
		paths = append(paths, path)
		return nil
	})
	sort.Strings(paths)
	return paths, err
}

func addDirectoryIfNeeded(fsWatcher *fsnotify.Watcher, path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	if err := addRecursive(fsWatcher, path); err != nil {
		log.Printf("watch new directory %s: %v", path, err)
		return false
	}
	return true
}
