package codec

/*
#cgo pkg-config: opus
#include <stdlib.h>
#include <opus.h>

static int lylat_opus_encoder_set_bitrate(OpusEncoder *st, opus_int32 bitrate) {
	return opus_encoder_ctl(st, OPUS_SET_BITRATE(bitrate));
}

static int lylat_opus_encoder_set_complexity(OpusEncoder *st, opus_int32 complexity) {
	return opus_encoder_ctl(st, OPUS_SET_COMPLEXITY(complexity));
}

static int lylat_opus_encoder_set_signal_voice(OpusEncoder *st) {
	return opus_encoder_ctl(st, OPUS_SET_SIGNAL(OPUS_SIGNAL_VOICE));
}

static int lylat_opus_encoder_set_inband_fec(OpusEncoder *st, opus_int32 enabled) {
	return opus_encoder_ctl(st, OPUS_SET_INBAND_FEC(enabled));
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const (
	OpusSampleRate      = 48000
	OpusChannels        = 1
	OpusRTPChannels     = 2
	OpusFrameSize       = 960
	OpusFrameDurationMS = 20
	OpusBitrate         = 32000
	OpusMaxPacketBytes  = 1275
)

type OpusEncoder struct {
	ptr *C.OpusEncoder
}

type OpusDecoder struct {
	ptr *C.OpusDecoder
}

func NewOpusEncoder(bitrate int) (*OpusEncoder, error) {
	var errno C.int
	ptr := C.opus_encoder_create(C.opus_int32(OpusSampleRate), C.int(OpusChannels), C.OPUS_APPLICATION_VOIP, &errno)
	if errno != C.OPUS_OK {
		return nil, opusError("create encoder", errno)
	}
	encoder := &OpusEncoder{ptr: ptr}

	if bitrate <= 0 {
		bitrate = OpusBitrate
	}
	if err := opusCtl("set encoder bitrate", C.lylat_opus_encoder_set_bitrate(ptr, C.opus_int32(bitrate))); err != nil {
		encoder.Close()
		return nil, err
	}
	if err := opusCtl("set encoder complexity", C.lylat_opus_encoder_set_complexity(ptr, 5)); err != nil {
		encoder.Close()
		return nil, err
	}
	if err := opusCtl("set encoder voice signal", C.lylat_opus_encoder_set_signal_voice(ptr)); err != nil {
		encoder.Close()
		return nil, err
	}
	if err := opusCtl("set encoder FEC", C.lylat_opus_encoder_set_inband_fec(ptr, 1)); err != nil {
		encoder.Close()
		return nil, err
	}

	return encoder, nil
}

func (e *OpusEncoder) Encode(frame []int16, out []byte) (int, error) {
	if e == nil || e.ptr == nil {
		return 0, fmt.Errorf("opus encoder is closed")
	}
	if len(frame) != OpusFrameSize {
		return 0, fmt.Errorf("opus frame has %d samples, want %d", len(frame), OpusFrameSize)
	}
	if len(out) == 0 {
		return 0, fmt.Errorf("opus output buffer is empty")
	}

	n := C.opus_encode(
		e.ptr,
		(*C.opus_int16)(unsafe.Pointer(&frame[0])),
		C.int(OpusFrameSize),
		(*C.uchar)(unsafe.Pointer(&out[0])),
		C.opus_int32(len(out)),
	)
	if n < 0 {
		return 0, opusError("encode", n)
	}
	return int(n), nil
}

func (e *OpusEncoder) Close() {
	if e != nil && e.ptr != nil {
		C.opus_encoder_destroy(e.ptr)
		e.ptr = nil
	}
}

func NewOpusDecoder() (*OpusDecoder, error) {
	var errno C.int
	ptr := C.opus_decoder_create(C.opus_int32(OpusSampleRate), C.int(OpusChannels), &errno)
	if errno != C.OPUS_OK {
		return nil, opusError("create decoder", errno)
	}
	return &OpusDecoder{ptr: ptr}, nil
}

func (d *OpusDecoder) Decode(packet []byte, out []int16) (int, error) {
	if d == nil || d.ptr == nil {
		return 0, fmt.Errorf("opus decoder is closed")
	}
	if len(packet) == 0 {
		return 0, fmt.Errorf("opus packet is empty")
	}
	if len(out) == 0 {
		return 0, fmt.Errorf("opus decode buffer is empty")
	}

	n := C.opus_decode(
		d.ptr,
		(*C.uchar)(unsafe.Pointer(&packet[0])),
		C.opus_int32(len(packet)),
		(*C.opus_int16)(unsafe.Pointer(&out[0])),
		C.int(len(out)),
		0,
	)
	if n < 0 {
		return 0, opusError("decode", n)
	}
	return int(n), nil
}

func (d *OpusDecoder) Close() {
	if d != nil && d.ptr != nil {
		C.opus_decoder_destroy(d.ptr)
		d.ptr = nil
	}
}

func opusCtl(action string, result C.int) error {
	if result == C.OPUS_OK {
		return nil
	}
	return opusError(action, result)
}

func opusError(action string, code C.int) error {
	return fmt.Errorf("opus %s failed: %s", action, C.GoString(C.opus_strerror(code)))
}
