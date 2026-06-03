package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

func DecodePCM16MonoWAV(data []byte) ([]int16, error) {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, errors.New("not a RIFF/WAVE file")
	}

	reader := bytes.NewReader(data[12:])
	var formatOK bool
	var pcm []int16
	for reader.Len() >= 8 {
		var chunkID [4]byte
		if _, err := io.ReadFull(reader, chunkID[:]); err != nil {
			return nil, err
		}
		var chunkSize uint32
		if err := binary.Read(reader, binary.LittleEndian, &chunkSize); err != nil {
			return nil, err
		}
		if uint64(chunkSize) > uint64(reader.Len()) {
			return nil, fmt.Errorf("invalid WAV chunk %q size %d", string(chunkID[:]), chunkSize)
		}

		chunk := make([]byte, chunkSize)
		if _, err := io.ReadFull(reader, chunk); err != nil {
			return nil, err
		}
		if chunkSize%2 == 1 && reader.Len() > 0 {
			if _, err := reader.ReadByte(); err != nil {
				return nil, err
			}
		}

		switch string(chunkID[:]) {
		case "fmt ":
			if err := validatePCM16MonoFormat(chunk); err != nil {
				return nil, err
			}
			formatOK = true
		case "data":
			if len(chunk)%2 != 0 {
				return nil, errors.New("WAV data chunk has odd byte length")
			}
			pcm = make([]int16, len(chunk)/2)
			for i := range pcm {
				pcm[i] = int16(binary.LittleEndian.Uint16(chunk[i*2 : i*2+2]))
			}
		}
	}

	if !formatOK {
		return nil, errors.New("WAV fmt chunk not found")
	}
	if len(pcm) == 0 {
		return nil, errors.New("WAV data chunk not found")
	}
	return pcm, nil
}

func validatePCM16MonoFormat(chunk []byte) error {
	if len(chunk) < 16 {
		return errors.New("WAV fmt chunk too short")
	}
	audioFormat := binary.LittleEndian.Uint16(chunk[0:2])
	channels := binary.LittleEndian.Uint16(chunk[2:4])
	sampleRate := binary.LittleEndian.Uint32(chunk[4:8])
	bitsPerSample := binary.LittleEndian.Uint16(chunk[14:16])

	if audioFormat != 1 {
		return fmt.Errorf("unsupported WAV format %d", audioFormat)
	}
	if channels != Channels {
		return fmt.Errorf("unsupported WAV channel count %d", channels)
	}
	if sampleRate != SampleRate {
		return fmt.Errorf("unsupported WAV sample rate %d", sampleRate)
	}
	if bitsPerSample != 16 {
		return fmt.Errorf("unsupported WAV bit depth %d", bitsPerSample)
	}
	return nil
}
