package voice

import (
	"strings"
	"testing"
	"time"
)

func TestDecodePCMUSilence(t *testing.T) {
	if sample := decodePCMU(0xff); sample != 0 {
		t.Fatalf("decodePCMU(0xff) = %d, want silence", sample)
	}
}

func TestEncodePCMUSilence(t *testing.T) {
	if value := encodePCMU(0); value != 0xff {
		t.Fatalf("encodePCMU(0) = %#x, want 0xff", value)
	}
}

func TestEncodePCMUFrameDownsamplesToTwentyMilliseconds(t *testing.T) {
	frame := make([]int16, 960)
	for i := range frame {
		frame[i] = 16000
	}

	payload := encodePCMUFrame(frame)
	if len(payload) != 160 {
		t.Fatalf("payload length = %d, want 160", len(payload))
	}
	if decoded := decodePCMU(payload[0]); decoded <= 0 {
		t.Fatalf("decoded first sample = %d, want positive audio", decoded)
	}
}

func TestAudioMetricsFormatsPCMUStats(t *testing.T) {
	start := time.Unix(0, 0)
	metrics := newAudioMetrics("match-1", "audio/PCMU", 8000, start)

	var line string
	payload := make([]byte, 160)
	for i := range payload {
		payload[i] = 0xff
	}

	for i := 0; i < 50; i++ {
		line = metrics.observe(uint16(i+1), payload, len(payload)+12, start.Add(time.Duration(i+1)*20*time.Millisecond), nil)
	}

	for _, expected := range []string{
		"match=match-1",
		"codec=audio/PCMU",
		"clock=8000Hz",
		"pkts=50",
		"rtp=68.8kbps",
		"payload=64.0kbps",
		"avgPayload=160B",
		"seqGaps=0",
		"levelRMS=-inf dBFS",
	} {
		if !strings.Contains(line, expected) {
			t.Fatalf("expected %q in stats line:\n%s", expected, line)
		}
	}
}

func TestAudioMetricsCountsSequenceGaps(t *testing.T) {
	start := time.Unix(0, 0)
	metrics := newAudioMetrics("match-1", "audio/PCMU", 8000, start)
	payload := []byte{0xff}

	metrics.observe(1, payload, 13, start.Add(200*time.Millisecond), nil)
	line := metrics.observe(4, payload, 13, start.Add(time.Second), nil)
	if !strings.Contains(line, "seqGaps=2") {
		t.Fatalf("expected sequence gap count in stats line:\n%s", line)
	}
}

func TestApplyGainInPlace(t *testing.T) {
	frame := []int16{1000, -1000}
	applyGainInPlace(frame, 6)
	if frame[0] < 1900 || frame[0] > 2100 {
		t.Fatalf("positive sample after gain = %d, want about 2000", frame[0])
	}
	if frame[1] > -1900 || frame[1] < -2100 {
		t.Fatalf("negative sample after gain = %d, want about -2000", frame[1])
	}
}

func TestChimeGainIsGentleAttenuation(t *testing.T) {
	frame := []int16{10000}
	applyGainInPlace(frame, chimeGainDB)
	if frame[0] < 8900 || frame[0] > 9000 {
		t.Fatalf("chime sample after gain = %d, want about 8913", frame[0])
	}
}

func TestMicProcessorNoiseGateMutesQuietFrames(t *testing.T) {
	processor := newMicProcessor(0, -40)
	frame := []int16{100, -100, 100, -100}

	processor.process(frame)

	for _, sample := range frame {
		if sample != 0 {
			t.Fatalf("quiet sample was not gated: %d", sample)
		}
	}
}

func TestMicProcessorNoiseGateKeepsLoudFrames(t *testing.T) {
	processor := newMicProcessor(0, -40)
	frame := []int16{12000, -12000, 12000, -12000}

	processor.process(frame)

	for _, sample := range frame {
		if sample == 0 {
			t.Fatal("loud frame was gated")
		}
	}
}
