package slp

import (
	"encoding/binary"
	"testing"
)

func TestParseFinalizedReplay(t *testing.T) {
	data := syntheticSLP(true, true, 0x2F9)
	match, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if match.MatchID != "mode.unranked-2022-12-20T06:52:39.18-0:1:0" {
		t.Fatalf("unexpected match id: %s", match.MatchID)
	}
	if !match.GameEnded {
		t.Fatal("expected game end to be detected")
	}
	if len(match.PlayerCodes) != 2 || match.PlayerCodes[0] != "MANG#000" || match.PlayerCodes[1] != "TAFO#001" {
		t.Fatalf("unexpected player codes: %#v", match.PlayerCodes)
	}
	if len(match.Players) != 2 || match.Players[0].DisplayName != "Tafo" || match.Players[1].DisplayName != "Mang" {
		t.Fatalf("unexpected players: %#v", match.Players)
	}
}

func TestParseLiveReplayWithZeroRawLength(t *testing.T) {
	data := syntheticSLP(false, false, 0x2F9)
	match, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if match.GameEnded {
		t.Fatal("live replay should not report game ended")
	}
	if match.MatchID == "" {
		t.Fatal("expected live replay match id")
	}
}

func TestParseGameStartWithShortOptionalTail(t *testing.T) {
	data := syntheticSLP(true, true, 0x2F8)
	match, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if match.TiebreakerNumber != 0 {
		t.Fatalf("unexpected tiebreaker number: %d", match.TiebreakerNumber)
	}
	if match.MatchID != "mode.unranked-2022-12-20T06:52:39.18-0:1:0" {
		t.Fatalf("unexpected match id: %s", match.MatchID)
	}
}

func syntheticSLP(ended bool, finalized bool, gameStartLen int) []byte {
	raw := syntheticRaw(ended, gameStartLen)
	header := []byte{'{', 'U', 0x03, 'r', 'a', 'w', '[', '$', 'U', '#', 'l', 0, 0, 0, 0}
	if finalized {
		binary.BigEndian.PutUint32(header[len(header)-4:], uint32(len(raw)))
	}
	return append(header, raw...)
}

func syntheticRaw(ended bool, gameStartLen int) []byte {
	gameStartSize := uint16(gameStartLen - 1)
	gameEndSize := uint16(0)
	payload := []byte{
		eventPayloads,
		0x07,
		gameStart,
		byte(gameStartSize >> 8),
		byte(gameStartSize),
		gameEnd,
		byte(gameEndSize >> 8),
		byte(gameEndSize),
	}

	start := make([]byte, gameStartLen)
	start[0] = gameStart
	writeASCII(start, displayNameOffset, displayNameLen, "Tafo")
	writeASCII(start, displayNameOffset+displayNameStride, displayNameLen, "Mang")
	writeConnectCode(start, connectCodeOffset, "TAFO", "001")
	writeConnectCode(start, connectCodeOffset+connectCodeStride, "MANG", "000")
	writeASCII(start, slippiUIDOffset, slippiUIDLen, "uid-tafo")
	writeASCII(start, slippiUIDOffset+slippiUIDStride, slippiUIDLen, "uid-mang")
	writeASCII(start, sessionIDOffset, sessionIDLen, "mode.unranked-2022-12-20T06:52:39.18-0")
	binary.BigEndian.PutUint32(start[gameNumberOffset:gameNumberOffset+4], 1)
	if len(start) >= tiebreakerOffset+4 {
		binary.BigEndian.PutUint32(start[tiebreakerOffset:tiebreakerOffset+4], 0)
	}

	raw := append(payload, start...)
	if ended {
		raw = append(raw, gameEnd)
	}
	return raw
}

func writeASCII(buf []byte, offset, maxLen int, value string) {
	copy(buf[offset:offset+maxLen], []byte(value))
}

func writeConnectCode(buf []byte, offset int, name, digits string) {
	i := offset
	copy(buf[i:], []byte(name))
	i += len(name)
	buf[i] = 0x81
	buf[i+1] = 0x94
	i += 2
	copy(buf[i:], []byte(digits))
}
