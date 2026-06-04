package audio

import (
	"testing"

	"lylatlink/assets"
)

func TestDecodePCM16MonoWAVChimes(t *testing.T) {
	cases := map[string][]byte{
		"start": assets.StartWAV,
		"end":   assets.EndWAV,
	}
	for name, data := range cases {
		samples, err := DecodePCM16MonoWAV(data)
		if err != nil {
			t.Fatalf("%s chime decode: %v", name, err)
		}
		if len(samples) == 0 {
			t.Fatalf("expected %s chime samples", name)
		}
		if len(samples) > SampleRate {
			t.Fatalf("%s chime is longer than expected: samples=%d", name, len(samples))
		}
	}
}

func TestDownmixPCM16Stereo(t *testing.T) {
	got := downmixPCM16([]int16{100, 300, -100, -300}, 2)
	want := []int16{200, -200}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample %d = %d, want %d", i, got[i], want[i])
		}
	}
}
