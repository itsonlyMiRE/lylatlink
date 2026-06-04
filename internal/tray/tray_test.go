package tray

import (
	"strings"
	"testing"

	"lylatlink/internal/app"
)

func TestStatusTitleHidesRawHTTPError(t *testing.T) {
	status := app.Status{
		State:   app.StateError,
		Message: "Post \"http://127.0.0.1:8787/match/start\": dial tcp 127.0.0.1:8787: connect: connection refused",
	}

	title := statusTitle(status)
	if title != "Connection issue" {
		t.Fatalf("title = %q, want Connection issue", title)
	}
	if strings.Contains(strings.ToLower(title), "http") {
		t.Fatalf("title leaks raw transport detail: %q", title)
	}
}

func TestStatusTitleClipsLongWatchingMessage(t *testing.T) {
	status := app.Status{
		State:   app.StateWatching,
		Message: "This is an unexpectedly verbose status message that should never stretch the tray menu.",
	}

	title := statusTitle(status)
	if len(title) > 48 {
		t.Fatalf("title length = %d, want <= 48: %q", len(title), title)
	}
	if !strings.HasSuffix(title, "...") {
		t.Fatalf("title = %q, want clipped ellipsis", title)
	}
}

func TestReplayTitleShowsFullShortPath(t *testing.T) {
	got := replayTitle(`/Users/mire/Slippi`)
	want := `Replay Folder: /Users/mire/Slippi`
	if got != want {
		t.Fatalf("replayTitle() = %q, want %q", got, want)
	}
}

func TestEndCallTitleShowsHotkey(t *testing.T) {
	got := endCallTitle("f8")
	want := "End Call (F8)"
	if got != want {
		t.Fatalf("endCallTitle() = %q, want %q", got, want)
	}
}

func TestEndCallTitleOmitsEmptyHotkey(t *testing.T) {
	got := endCallTitle("")
	want := "End Call"
	if got != want {
		t.Fatalf("endCallTitle() = %q, want %q", got, want)
	}
}

func TestReplayTitleCompactsLongMacPath(t *testing.T) {
	got := replayTitle(`/Users/mire/some/really/really/really/really/long/path/to/Slippi`)
	if len(strings.TrimPrefix(got, "Replay Folder: ")) > 58 {
		t.Fatalf("compacted path is too long: %q", got)
	}
	if !strings.Contains(got, ".../Slippi") {
		t.Fatalf("compacted path should preserve final folder: %q", got)
	}
	if !strings.HasPrefix(got, "Replay Folder: /Users/mire/") {
		t.Fatalf("compacted path should preserve leading context: %q", got)
	}
}

func TestReplayTitleCompactsLongWindowsPath(t *testing.T) {
	got := replayTitle(`C:\Users\mire\some\really\really\really\really\long\path\to\Slippi`)
	if len(strings.TrimPrefix(got, "Replay Folder: ")) > 58 {
		t.Fatalf("compacted path is too long: %q", got)
	}
	if !strings.Contains(got, `...\Slippi`) {
		t.Fatalf("compacted path should preserve final folder: %q", got)
	}
	if !strings.HasPrefix(got, `Replay Folder: C:\Users\mire\`) {
		t.Fatalf("compacted path should preserve leading context: %q", got)
	}
}
