package audio

import (
	"encoding/binary"
	"testing"
)

func TestSampleBufferFillS16WritesSamplesAndSilence(t *testing.T) {
	buffer := newSampleBuffer(10)
	buffer.write([]int16{100, -100})

	out := make([]byte, 8)
	buffer.fillS16(out)

	values := []int16{
		int16(binary.LittleEndian.Uint16(out[0:2])),
		int16(binary.LittleEndian.Uint16(out[2:4])),
		int16(binary.LittleEndian.Uint16(out[4:6])),
		int16(binary.LittleEndian.Uint16(out[6:8])),
	}
	if values[0] != 100 || values[1] != -100 || values[2] != 0 || values[3] != 0 {
		t.Fatalf("values = %v, want [100 -100 0 0]", values)
	}
}

func TestSampleBufferDropsOldestSamplesWhenFull(t *testing.T) {
	buffer := newSampleBuffer(3)
	buffer.write([]int16{1, 2, 3})
	buffer.write([]int16{4, 5})

	out := make([]byte, 6)
	buffer.fillS16(out)

	values := []int16{
		int16(binary.LittleEndian.Uint16(out[0:2])),
		int16(binary.LittleEndian.Uint16(out[2:4])),
		int16(binary.LittleEndian.Uint16(out[4:6])),
	}
	if values[0] != 3 || values[1] != 4 || values[2] != 5 {
		t.Fatalf("values = %v, want [3 4 5]", values)
	}
}
