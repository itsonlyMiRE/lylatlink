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
