package slp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

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
	slippiUIDOffset   = 0x249
	slippiUIDStride   = 0x1D
	slippiUIDLen      = 0x1D
	sessionIDOffset   = 0x2BE
	sessionIDLen      = 0x33
	gameNumberOffset  = 0x2F1
	tiebreakerOffset  = 0x2F5
	minGameStartLen   = connectCodeOffset + connectCodeStride*4
)

var (
	ErrNoRawElement    = errors.New("slp raw element not found")
	ErrNoEventPayloads = errors.New("slp event payload table not found")
	ErrNoGameStart     = errors.New("slp game start event not found")
)

func ParseFile(path string) (*Match, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func Parse(data []byte) (*Match, error) {
	raw, err := extractRawStream(data)
	if err != nil {
		return nil, err
	}
	return parseRaw(raw)
}

func extractRawStream(data []byte) ([]byte, error) {
	if len(data) > 0 && data[0] == eventPayloads {
		return data, nil
	}

	patterns := [][]byte{
		{'U', 0x03, 'r', 'a', 'w', '[', '$', 'U', '#', 'l'},
		{'i', 0x03, 'r', 'a', 'w', '[', '$', 'U', '#', 'l'},
	}

	for _, pattern := range patterns {
		idx := bytes.Index(data, pattern)
		if idx < 0 {
			continue
		}
		lenOffset := idx + len(pattern)
		if len(data) < lenOffset+4 {
			return nil, ErrNoRawElement
		}
		rawLen := int(binary.BigEndian.Uint32(data[lenOffset : lenOffset+4]))
		start := lenOffset + 4
		if rawLen > 0 && start+rawLen <= len(data) {
			return data[start : start+rawLen], nil
		}
		if start <= len(data) {
			return data[start:], nil
		}
	}

	return nil, ErrNoRawElement
}

func parseRaw(raw []byte) (*Match, error) {
	if len(raw) < 2 || raw[0] != eventPayloads {
		return nil, ErrNoEventPayloads
	}

	payloadSize := int(raw[1])
	eventPayloadsEnd := 1 + payloadSize
	if len(raw) < eventPayloadsEnd {
		return nil, ErrNoEventPayloads
	}

	sizes := map[byte]int{}
	for off := 2; off+2 < eventPayloadsEnd; off += 3 {
		command := raw[off]
		size := int(binary.BigEndian.Uint16(raw[off+1 : off+3]))
		sizes[command] = size
	}

	var match *Match
	pos := eventPayloadsEnd
	for pos < len(raw) {
		command := raw[pos]
		payloadSize, ok := sizes[command]
		if !ok {
			break
		}
		eventSize := 1 + payloadSize
		if pos+eventSize > len(raw) {
			break
		}

		event := raw[pos : pos+eventSize]
		switch command {
		case gameStart:
			parsed, err := parseGameStart(event)
			if err != nil {
				return nil, err
			}
			match = parsed
		case gameEnd:
			if match != nil {
				match.GameEnded = true
			}
		}
		pos += eventSize
	}

	if match == nil {
		return nil, ErrNoGameStart
	}
	return match, nil
}

func parseGameStart(event []byte) (*Match, error) {
	if len(event) < minGameStartLen {
		return nil, fmt.Errorf("game start event too short: got %d, need %d", len(event), minGameStartLen)
	}

	players := make([]Player, 0, 2)
	codes := make([]string, 0, 2)
	for port := 0; port < 4; port++ {
		code := normalizeConnectCode(readSlippiString(event, connectCodeOffset+connectCodeStride*port, connectCodeLen))
		if code == "" {
			continue
		}
		player := Player{
			Port:        port + 1,
			DisplayName: readSlippiString(event, displayNameOffset+displayNameStride*port, displayNameLen),
			ConnectCode: code,
			SlippiUID:   readSlippiString(event, slippiUIDOffset+slippiUIDStride*port, slippiUIDLen),
		}
		players = append(players, player)
		codes = append(codes, code)
	}

	sort.Strings(codes)
	codes = uniqueStrings(codes)

	sessionID := readSlippiString(event, sessionIDOffset, sessionIDLen)
	gameNumber := readOptionalUint32(event, gameNumberOffset)
	tiebreakerNumber := readOptionalUint32(event, tiebreakerOffset)

	matchID := ""
	if sessionID != "" {
		matchID = fmt.Sprintf("%s:%d:%d", sessionID, gameNumber, tiebreakerNumber)
	}

	return &Match{
		MatchID:          matchID,
		SessionID:        sessionID,
		GameNumber:       gameNumber,
		TiebreakerNumber: tiebreakerNumber,
		PlayerCodes:      codes,
		Players:          players,
	}, nil
}

func readSlippiString(data []byte, offset, maxLen int) string {
	if offset >= len(data) || maxLen <= 0 {
		return ""
	}
	end := offset + maxLen
	if end > len(data) {
		end = len(data)
	}

	var b strings.Builder
	for i := offset; i < end; i++ {
		c := data[i]
		if c == 0x00 {
			break
		}
		if c == 0x81 && i+1 < end && data[i+1] == 0x94 {
			b.WriteByte('#')
			i++
			continue
		}
		if c >= 0x20 && c <= 0x7E {
			b.WriteByte(c)
		}
	}
	return strings.TrimSpace(b.String())
}

func normalizeConnectCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func readOptionalUint32(data []byte, offset int) uint32 {
	if offset >= len(data) {
		return 0
	}
	end := offset + 4
	if end > len(data) {
		end = len(data)
	}
	var buf [4]byte
	copy(buf[4-(end-offset):], data[offset:end])
	return binary.BigEndian.Uint32(buf[:])
}

func uniqueStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:0]
	var last string
	for i, value := range values {
		if i == 0 || value != last {
			out = append(out, value)
			last = value
		}
	}
	return out
}
