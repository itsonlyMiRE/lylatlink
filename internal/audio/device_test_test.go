package audio

import (
	"strings"
	"testing"
)

func TestLevelWindowS16Stats(t *testing.T) {
	var w levelWindow

	w.observeFrame([]int16{16384, -16384})
	stats := w.snapshot()

	if stats.Frames != 2 {
		t.Fatalf("frames = %d, want 2", stats.Frames)
	}
	if stats.Samples != 2 {
		t.Fatalf("samples = %d, want 2", stats.Samples)
	}
	if stats.RMSDBFS != "-6.0 dBFS" {
		t.Fatalf("rms = %s, want -6.0 dBFS", stats.RMSDBFS)
	}
	if stats.PeakDBFS != "-6.0 dBFS" {
		t.Fatalf("peak = %s, want -6.0 dBFS", stats.PeakDBFS)
	}
}

func TestFrameAssemblerEmitsTwentyMillisecondFrames(t *testing.T) {
	out := make(chan []int16, 2)
	assembler := &frameAssembler{out: out}
	input := make([]byte, FrameSize*2)

	assembler.observeS16(input)

	frame := <-out
	if len(frame) != FrameSize {
		t.Fatalf("frame length = %d, want %d", len(frame), FrameSize)
	}
}

func TestParseDeviceIDRejectsInvalidHex(t *testing.T) {
	if _, err := parseDeviceID("not hex"); err == nil {
		t.Fatal("expected invalid device ID error")
	}
}

func TestMicStatsString(t *testing.T) {
	stats := MicStats{
		Frames:        48000,
		Samples:       48000,
		FrameRate:     48000,
		Bytes:         96000,
		RMSDBFS:       "-34.2 dBFS",
		PeakDBFS:      "-12.8 dBFS",
		PeakAmplitude: 7500,
	}
	line := stats.String()
	for _, expected := range []string{"mic stats:", "frameRate=48000Hz", "rms=-34.2 dBFS", "peak=-12.8 dBFS", "peakAmp=7500"} {
		if !strings.Contains(line, expected) {
			t.Fatalf("expected %q in %q", expected, line)
		}
	}
}
