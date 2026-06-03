package audio

import (
	"context"
	"encoding/binary"
	"encoding/hex"
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

const (
	SampleRate    = 48000
	Channels      = 1
	FrameSize     = 960
	FrameDuration = 20 * time.Millisecond
)

type DeviceInfo struct {
	ID        string
	Name      string
	IsDefault bool
	Formats   []DeviceFormat
}

type DeviceFormat struct {
	Format     string
	Channels   uint32
	SampleRate uint32
}

type CaptureOptions struct {
	InputDeviceID     string
	FallbackToDefault bool
	Logger            *log.Logger
	Verbose           bool
	FrameBuffer       int
}

type Capture struct {
	Frames <-chan []int16

	DeviceName string
	SampleRate uint32
	Channels   uint32
	Format     string

	frameCh  chan []int16
	audioCtx *malgo.AllocatedContext
	device   *malgo.Device
	stopped  atomic.Bool
	stopOnce sync.Once
}

type frameAssembler struct {
	mu      sync.Mutex
	pending []int16
	out     chan<- []int16
}

func ListInputDevices() ([]DeviceInfo, error) {
	return listDevices(malgo.Capture, "input")
}

func ListOutputDevices() ([]DeviceInfo, error) {
	return listDevices(malgo.Playback, "output")
}

func listDevices(deviceType malgo.DeviceType, label string) ([]DeviceInfo, error) {
	audioCtx, err := initAudioContext(nil, false)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = audioCtx.Uninit()
		audioCtx.Free()
	}()

	infos, err := audioCtx.Devices(deviceType)
	if err != nil {
		return nil, fmt.Errorf("list %s devices: %w", label, err)
	}

	devices := make([]DeviceInfo, 0, len(infos))
	for _, info := range infos {
		devices = append(devices, newDeviceInfo(info))
	}
	return devices, nil
}

func StartCapture(ctx context.Context, opts CaptureOptions) (*Capture, error) {
	logger := opts.Logger
	if logger == nil {
		logger = log.Default()
	}
	frameBuffer := opts.FrameBuffer
	if frameBuffer <= 0 {
		frameBuffer = 64
	}

	audioCtx, err := initAudioContext(logger, opts.Verbose)
	if err != nil {
		return nil, fmt.Errorf("init audio context: %w", err)
	}

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatS16
	deviceConfig.Capture.Channels = Channels
	deviceConfig.SampleRate = SampleRate
	deviceConfig.PeriodSizeInFrames = FrameSize
	deviceConfig.Alsa.NoMMap = 1

	deviceName := "System Default"
	var selectedID malgo.DeviceID
	var selectedIDPinner runtime.Pinner
	defer selectedIDPinner.Unpin()
	requestedDevice := strings.TrimSpace(opts.InputDeviceID)
	if requestedDevice != "" {
		id, info, err := resolveInputDevice(audioCtx.Context, requestedDevice)
		if err != nil {
			if !opts.FallbackToDefault {
				_ = audioCtx.Uninit()
				audioCtx.Free()
				return nil, err
			}
			logger.Printf("input device %q unavailable; falling back to system default: %v", requestedDevice, err)
		} else {
			selectedID = id
			selectedIDPinner.Pin(&selectedID)
			deviceConfig.Capture.DeviceID = unsafe.Pointer(&selectedID)
			deviceName = info.Name
		}
	}

	frameCh := make(chan []int16, frameBuffer)
	assembler := &frameAssembler{out: frameCh}
	capture := &Capture{
		Frames:     frameCh,
		frameCh:    frameCh,
		DeviceName: deviceName,
		audioCtx:   audioCtx,
	}

	callbacks := malgo.DeviceCallbacks{
		Data: func(_, input []byte, _ uint32) {
			if capture.stopped.Load() {
				return
			}
			assembler.observeS16(input)
		},
	}

	device, err := malgo.InitDevice(audioCtx.Context, deviceConfig, callbacks)
	if err != nil {
		capture.Stop()
		return nil, fmt.Errorf("init capture device: %w", err)
	}
	capture.device = device

	if err := device.Start(); err != nil {
		capture.Stop()
		return nil, fmt.Errorf("start capture device: %w", err)
	}

	capture.SampleRate = device.SampleRate()
	capture.Channels = device.CaptureChannels()
	capture.Format = fmt.Sprint(device.CaptureFormat())

	go func() {
		<-ctx.Done()
		capture.Stop()
	}()

	return capture, nil
}

func (c *Capture) Stop() {
	c.stopOnce.Do(func() {
		c.stopped.Store(true)
		if c.device != nil {
			_ = c.device.Stop()
			c.device.Uninit()
		}
		if c.audioCtx != nil {
			_ = c.audioCtx.Uninit()
			c.audioCtx.Free()
		}
		close(c.frameCh)
	})
}

func (a *frameAssembler) observeS16(input []byte) {
	if len(input) < 2 {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for i := 0; i+1 < len(input); i += 2 {
		a.pending = append(a.pending, int16(binary.LittleEndian.Uint16(input[i:i+2])))
	}
	for len(a.pending) >= FrameSize {
		frame := make([]int16, FrameSize)
		copy(frame, a.pending[:FrameSize])
		a.pending = a.pending[FrameSize:]
		select {
		case a.out <- frame:
		default:
		}
	}
	if len(a.pending) == 0 {
		a.pending = a.pending[:0]
	}
}

func initAudioContext(logger *log.Logger, verbose bool) (*malgo.AllocatedContext, error) {
	var logProc malgo.LogProc
	if verbose {
		if logger == nil {
			logger = log.Default()
		}
		logProc = func(message string) {
			logger.Printf("malgo: %s", message)
		}
	}
	return malgo.InitContext(nil, malgo.ContextConfig{}, logProc)
}

func resolveInputDevice(ctx malgo.Context, requested string) (malgo.DeviceID, DeviceInfo, error) {
	return resolveDevice(ctx, malgo.Capture, "input", requested)
}

func resolveOutputDevice(ctx malgo.Context, requested string) (malgo.DeviceID, DeviceInfo, error) {
	return resolveDevice(ctx, malgo.Playback, "output", requested)
}

func resolveDevice(ctx malgo.Context, deviceType malgo.DeviceType, label string, requested string) (malgo.DeviceID, DeviceInfo, error) {
	infos, err := ctx.Devices(deviceType)
	if err != nil {
		return malgo.DeviceID{}, DeviceInfo{}, fmt.Errorf("list %s devices: %w", label, err)
	}

	for _, info := range infos {
		device := newDeviceInfo(info)
		if strings.EqualFold(device.ID, requested) || device.Name == requested {
			return info.ID, device, nil
		}
	}

	if _, err := parseDeviceID(requested); err != nil {
		return malgo.DeviceID{}, DeviceInfo{}, fmt.Errorf("%s device %q not found by ID or name, and ID is invalid: %w", label, requested, err)
	}
	return malgo.DeviceID{}, DeviceInfo{}, fmt.Errorf("%s device %q not found", label, requested)
}

func parseDeviceID(raw string) (malgo.DeviceID, error) {
	var id malgo.DeviceID
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return id, fmt.Errorf("empty device ID")
	}
	decoded, err := hex.DecodeString(raw)
	if err != nil {
		return id, err
	}
	if len(decoded) > len(id) {
		return id, fmt.Errorf("device ID has %d bytes, max is %d", len(decoded), len(id))
	}
	copy(id[:], decoded)
	return id, nil
}

func newDeviceInfo(info malgo.DeviceInfo) DeviceInfo {
	formats := make([]DeviceFormat, 0, len(info.Formats))
	for _, format := range info.Formats {
		formats = append(formats, DeviceFormat{
			Format:     fmt.Sprint(format.Format),
			Channels:   format.Channels,
			SampleRate: format.SampleRate,
		})
	}
	return DeviceInfo{
		ID:        info.ID.String(),
		Name:      info.Name(),
		IsDefault: info.IsDefault != 0,
		Formats:   formats,
	}
}
