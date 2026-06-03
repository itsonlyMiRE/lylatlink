package watcher

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInspectEmitsEndWhenFileStabilizesWithoutGameEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Game.slp")
	if err := os.WriteFile(path, syntheticSLPWithoutGameEnd(), 0o644); err != nil {
		t.Fatal(err)
	}

	w := &Watcher{StableThreshold: time.Millisecond}
	states := map[string]*fileState{}
	events := make(chan MatchEvent, 4)

	w.inspect(context.Background(), path, states, events)
	start := nextEvent(t, events)
	if start.Type != EventMatchStart {
		t.Fatalf("first event = %s, want %s", start.Type, EventMatchStart)
	}

	time.Sleep(2 * time.Millisecond)
	w.inspect(context.Background(), path, states, events)
	end := nextEvent(t, events)
	if end.Type != EventMatchEnd {
		t.Fatalf("second event = %s, want %s", end.Type, EventMatchEnd)
	}
	if !end.Match.GameEnded {
		t.Fatal("stable-file fallback should mark emitted end event as game-ended")
	}

	w.inspect(context.Background(), path, states, events)
	select {
	case event := <-events:
		t.Fatalf("unexpected duplicate event: %s", event.Type)
	default:
	}
}

func TestRunEmitsStartForExistingReplayInMonthFolder(t *testing.T) {
	root := t.TempDir()
	monthDir := filepath.Join(root, "2026-06")
	if err := os.Mkdir(monthDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(monthDir, "Game.slp")
	if err := os.WriteFile(path, syntheticSLPWithoutGameEnd(), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := &Watcher{
		ReplayDir:       root,
		Debounce:        time.Millisecond,
		StableThreshold: time.Hour,
	}
	events := make(chan MatchEvent, 4)
	errs := make(chan error, 1)
	go func() {
		errs <- w.Run(ctx, events)
	}()

	event := nextEvent(t, events)
	if event.Type != EventMatchStart {
		t.Fatalf("event = %s, want %s", event.Type, EventMatchStart)
	}
	if event.Path != path {
		t.Fatalf("path = %s, want %s", event.Path, path)
	}

	cancel()
	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("watcher returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for watcher shutdown")
	}
}

func TestInspectEmitsStartForOneCodeSelfMatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Game.slp")
	if err := os.WriteFile(path, syntheticSelfMatchSLPWithoutGameEnd(), 0o644); err != nil {
		t.Fatal(err)
	}

	w := &Watcher{StableThreshold: time.Hour}
	states := map[string]*fileState{}
	events := make(chan MatchEvent, 4)

	w.inspect(context.Background(), path, states, events)
	event := nextEvent(t, events)
	if event.Type != EventMatchStart {
		t.Fatalf("event = %s, want %s", event.Type, EventMatchStart)
	}
	if len(event.Match.PlayerCodes) != 1 || event.Match.PlayerCodes[0] != "TAFO#001" {
		t.Fatalf("player codes = %#v, want one deduped code", event.Match.PlayerCodes)
	}
}

func TestRunSkipsStaleReplayOnStartup(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Game.slp")
	if err := os.WriteFile(path, syntheticSLPWithoutGameEnd(), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := New(root)
	w.Debounce = time.Millisecond
	events := make(chan MatchEvent, 4)
	errs := make(chan error, 1)
	go func() {
		errs <- w.Run(ctx, events)
	}()

	select {
	case event := <-events:
		t.Fatalf("unexpected event for stale startup replay: %s", event.Type)
	case <-time.After(50 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("watcher returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for watcher shutdown")
	}
}

func TestRunEmitsStartForNewReplayAfterStartup(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Game.slp")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := New(root)
	w.Debounce = time.Millisecond
	w.StableThreshold = time.Hour
	events := make(chan MatchEvent, 4)
	errs := make(chan error, 1)
	go func() {
		errs <- w.Run(ctx, events)
	}()

	select {
	case event := <-events:
		t.Fatalf("unexpected startup event before replay exists: %s", event.Type)
	case <-time.After(50 * time.Millisecond):
	}

	if err := os.WriteFile(path, syntheticSLPWithoutGameEnd(), 0o644); err != nil {
		t.Fatal(err)
	}

	event := nextEvent(t, events)
	if event.Type != EventMatchStart {
		t.Fatalf("event = %s, want %s", event.Type, EventMatchStart)
	}
	if event.Path != path {
		t.Fatalf("path = %s, want %s", event.Path, path)
	}

	cancel()
	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("watcher returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for watcher shutdown")
	}
}

func nextEvent(t *testing.T, events <-chan MatchEvent) MatchEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for watcher event")
		return MatchEvent{}
	}
}

func syntheticSLPWithoutGameEnd() []byte {
	raw := syntheticRawWithoutGameEnd("MANG", "000")
	header := []byte{'{', 'U', 0x03, 'r', 'a', 'w', '[', '$', 'U', '#', 'l', 0, 0, 0, 0}
	binary.BigEndian.PutUint32(header[len(header)-4:], uint32(len(raw)))
	return append(header, raw...)
}

func syntheticSelfMatchSLPWithoutGameEnd() []byte {
	raw := syntheticRawWithoutGameEnd("TAFO", "001")
	header := []byte{'{', 'U', 0x03, 'r', 'a', 'w', '[', '$', 'U', '#', 'l', 0, 0, 0, 0}
	binary.BigEndian.PutUint32(header[len(header)-4:], uint32(len(raw)))
	return append(header, raw...)
}

func syntheticRawWithoutGameEnd(secondCodeName, secondCodeDigits string) []byte {
	const (
		eventPayloads = 0x35
		gameStart     = 0x36
		gameEnd       = 0x39

		displayNameOffset = 0x1A5
		displayNameStride = 0x1F
		displayNameLen    = 0x1F
		connectCodeOffset = 0x221
		connectCodeStride = 0x0A
		connectCodeLen    = 0x0A
		sessionIDOffset   = 0x2BE
		sessionIDLen      = 0x33
		gameNumberOffset  = 0x2F1
		gameStartLen      = 0x2F8
	)

	payload := []byte{
		eventPayloads,
		0x07,
		gameStart,
		byte((gameStartLen - 1) >> 8),
		byte((gameStartLen - 1) & 0xff),
		gameEnd,
		0x00,
		0x00,
	}

	start := make([]byte, gameStartLen)
	start[0] = gameStart
	writeTestASCII(start, displayNameOffset, displayNameLen, "Tafo")
	writeTestASCII(start, displayNameOffset+displayNameStride, displayNameLen, "Mang")
	writeTestConnectCode(start, connectCodeOffset, "TAFO", "001")
	writeTestConnectCode(start, connectCodeOffset+connectCodeStride, secondCodeName, secondCodeDigits)
	writeTestASCII(start, sessionIDOffset, sessionIDLen, "mode.unranked-2022-12-20T06:52:39.18-0")
	binary.BigEndian.PutUint32(start[gameNumberOffset:gameNumberOffset+4], 1)

	return append(payload, start...)
}

func writeTestASCII(buf []byte, offset, maxLen int, value string) {
	copy(buf[offset:offset+maxLen], []byte(value))
}

func writeTestConnectCode(buf []byte, offset int, name, digits string) {
	i := offset
	copy(buf[i:], []byte(name))
	i += len(name)
	buf[i] = 0x81
	buf[i+1] = 0x94
	i += 2
	copy(buf[i:], []byte(digits))
}
