package audio

import (
	"testing"

	"lylatlink/assets"
)

func TestDecodePCM16MonoWAVChime(t *testing.T) {
	samples, err := DecodePCM16MonoWAV(assets.ChimeWAV)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) == 0 {
		t.Fatal("expected chime samples")
	}
	if len(samples) > SampleRate {
		t.Fatalf("chime is longer than expected: samples=%d", len(samples))
	}
}
