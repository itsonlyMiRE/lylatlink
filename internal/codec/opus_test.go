package codec

import (
	"math"
	"testing"
)

func TestOpusEncodeDecodeTwentyMillisecondFrame(t *testing.T) {
	encoder, err := NewOpusEncoder(OpusBitrate)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()

	frame := make([]int16, OpusFrameSize)
	for i := range frame {
		phase := 2 * math.Pi * 440 * float64(i) / OpusSampleRate
		frame[i] = int16(math.Sin(phase) * 12000)
	}

	packet := make([]byte, OpusMaxPacketBytes)
	n, err := encoder.Encode(frame, packet)
	if err != nil {
		t.Fatal(err)
	}
	if n <= 0 {
		t.Fatalf("encoded packet length = %d, want > 0", n)
	}

	decoder, err := NewOpusDecoder()
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()

	decoded := make([]int16, OpusFrameSize)
	samples, err := decoder.Decode(packet[:n], decoded)
	if err != nil {
		t.Fatal(err)
	}
	if samples != OpusFrameSize {
		t.Fatalf("decoded samples = %d, want %d", samples, OpusFrameSize)
	}
}
