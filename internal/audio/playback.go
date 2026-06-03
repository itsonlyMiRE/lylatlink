package audio

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/gen2brain/malgo"
)

type PlaybackOptions struct {
	OutputDeviceID    string
	FallbackToDefault bool
	Logger            *log.Logger
	Verbose           bool
	BufferFrames      int
}

type Playback struct {
	DeviceName string
	SampleRate uint32
	Channels   uint32
	Format     string

	buffer   *sampleBuffer
	audioCtx *malgo.AllocatedContext
	device   *malgo.Device
	stopped  atomic.Bool
	stopOnce sync.Once
}

type sampleBuffer struct {
	mu         sync.Mutex
	samples    []int16
	maxSamples int
}

func StartPlayback(ctx context.Context, opts PlaybackOptions) (*Playback, error) {
	logger := opts.Logger
	if logger == nil {
		logger = log.Default()
	}
	bufferFrames := opts.BufferFrames
	if bufferFrames <= 0 {
		bufferFrames = 50
	}

	audioCtx, err := initAudioContext(logger, opts.Verbose)
	if err != nil {
		return nil, fmt.Errorf("init playback context: %w", err)
	}

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Playback)
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = Channels
	deviceConfig.SampleRate = SampleRate
	deviceConfig.PeriodSizeInFrames = FrameSize
	deviceConfig.Alsa.NoMMap = 1

	deviceName := "System Default"
	var selectedID malgo.DeviceID
	var selectedIDPinner runtime.Pinner
	defer selectedIDPinner.Unpin()
	requestedDevice := strings.TrimSpace(opts.OutputDeviceID)
	if requestedDevice != "" {
		id, info, err := resolveOutputDevice(audioCtx.Context, requestedDevice)
		if err != nil {
			if !opts.FallbackToDefault {
				_ = audioCtx.Uninit()
				audioCtx.Free()
				return nil, err
			}
			logger.Printf("output device %q unavailable; falling back to system default: %v", requestedDevice, err)
		} else {
			selectedID = id
			selectedIDPinner.Pin(&selectedID)
			deviceConfig.Playback.DeviceID = unsafe.Pointer(&selectedID)
			deviceName = info.Name
		}
	}

	playback := &Playback{
		DeviceName: deviceName,
		buffer:     newSampleBuffer(bufferFrames * FrameSize),
		audioCtx:   audioCtx,
	}
	callbacks := malgo.DeviceCallbacks{
		Data: func(output, _ []byte, _ uint32) {
			if playback.stopped.Load() {
				clear(output)
				return
			}
			playback.buffer.fillS16(output)
		},
	}

	device, err := malgo.InitDevice(audioCtx.Context, deviceConfig, callbacks)
	if err != nil {
		playback.Stop()
		return nil, fmt.Errorf("init playback device: %w", err)
	}
	playback.device = device

	if err := device.Start(); err != nil {
		playback.Stop()
		return nil, fmt.Errorf("start playback device: %w", err)
	}

	playback.SampleRate = device.SampleRate()
	playback.Channels = device.PlaybackChannels()
	playback.Format = fmt.Sprint(device.PlaybackFormat())

	go func() {
		<-ctx.Done()
		playback.Stop()
	}()

	return playback, nil
}

func (p *Playback) Write(frame []int16) bool {
	if p == nil || p.stopped.Load() {
		return false
	}
	p.buffer.write(frame)
	return true
}

func (p *Playback) PlayPCM(ctx context.Context, samples []int16) bool {
	for len(samples) > 0 {
		frameSize := FrameSize
		if len(samples) < frameSize {
			frameSize = len(samples)
		}
		frame := samples[:frameSize]
		if !p.Write(frame) {
			return false
		}
		samples = samples[frameSize:]
		if !sleepContext(ctx, durationForSamples(frameSize)) {
			return false
		}
	}
	return true
}

func (p *Playback) Stop() {
	p.stopOnce.Do(func() {
		p.stopped.Store(true)
		if p.device != nil {
			_ = p.device.Stop()
			p.device.Uninit()
		}
		if p.audioCtx != nil {
			_ = p.audioCtx.Uninit()
			p.audioCtx.Free()
		}
	})
}

func newSampleBuffer(maxSamples int) *sampleBuffer {
	if maxSamples <= 0 {
		maxSamples = FrameSize
	}
	return &sampleBuffer{maxSamples: maxSamples}
}

func (b *sampleBuffer) write(frame []int16) {
	if len(frame) == 0 {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	overflow := len(b.samples) + len(frame) - b.maxSamples
	if overflow > 0 {
		if overflow >= len(b.samples) {
			b.samples = b.samples[:0]
		} else {
			copy(b.samples, b.samples[overflow:])
			b.samples = b.samples[:len(b.samples)-overflow]
		}
	}
	if len(frame) > b.maxSamples {
		frame = frame[len(frame)-b.maxSamples:]
	}
	b.samples = append(b.samples, frame...)
}

func (b *sampleBuffer) fillS16(output []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	sampleCount := len(output) / 2
	available := len(b.samples)
	if available > sampleCount {
		available = sampleCount
	}

	for i := 0; i < available; i++ {
		binary.LittleEndian.PutUint16(output[i*2:i*2+2], uint16(b.samples[i]))
	}
	for i := available * 2; i < len(output); i++ {
		output[i] = 0
	}
	if available > 0 {
		copy(b.samples, b.samples[available:])
		b.samples = b.samples[:len(b.samples)-available]
	}
}

func durationForSamples(samples int) time.Duration {
	return time.Duration(samples) * time.Second / SampleRate
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
	if duration <= 0 {
		return true
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
