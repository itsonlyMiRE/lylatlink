package audio

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

type DeviceTestOptions struct {
	Duration      time.Duration
	Logger        *log.Logger
	Verbose       bool
	InputDeviceID string
}

type levelWindow struct {
	mu sync.Mutex

	frames      uint64
	samples     uint64
	bytes       uint64
	sumSquares  float64
	peakSample  int
	startedAt   time.Time
	lastFrameAt time.Time
}

func RunDeviceTest(ctx context.Context, opts DeviceTestOptions) error {
	logger := opts.Logger
	if logger == nil {
		logger = log.Default()
	}
	duration := opts.Duration
	if duration <= 0 {
		duration = 10 * time.Second
	}

	logger.Printf(
		"mic test starting: requestedSampleRate=%d requestedChannels=%d frameSize=%d format=s16 duration=%s inputDevice=%q",
		SampleRate,
		Channels,
		FrameSize,
		duration,
		opts.InputDeviceID,
	)

	capture, err := StartCapture(ctx, CaptureOptions{
		InputDeviceID: opts.InputDeviceID,
		Logger:        logger,
		Verbose:       opts.Verbose,
	})
	if err != nil {
		return err
	}
	defer capture.Stop()

	logger.Printf(
		"mic device opened: device=%q sampleRate=%d captureChannels=%d captureFormat=%s",
		capture.DeviceName,
		capture.SampleRate,
		capture.Channels,
		capture.Format,
	)

	stats := &levelWindow{startedAt: time.Now()}
	testCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-testCtx.Done():
			logger.Print("mic test finished")
			return nil
		case frame, ok := <-capture.Frames:
			if !ok {
				return fmt.Errorf("mic capture stopped")
			}
			stats.observeFrame(frame)
		case <-ticker.C:
			logger.Print(stats.snapshot().String())
		}
	}
}

func (w *levelWindow) observeFrame(frame []int16) {
	if len(frame) == 0 {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.startedAt.IsZero() {
		w.startedAt = time.Now()
	}
	w.lastFrameAt = time.Now()
	w.frames += uint64(len(frame))
	w.samples += uint64(len(frame))
	w.bytes += uint64(len(frame) * 2)

	for _, value := range frame {
		sample := int(value)
		if sample < 0 {
			sample = -sample
		}
		if sample > w.peakSample {
			w.peakSample = sample
		}
		w.sumSquares += float64(sample * sample)
	}
}

type MicStats struct {
	Elapsed       time.Duration
	Frames        uint64
	Samples       uint64
	Bytes         uint64
	FrameRate     float64
	RMSDBFS       string
	PeakDBFS      string
	LastFrameAge  time.Duration
	PeakAmplitude int
}

func (w *levelWindow) snapshot() MicStats {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(w.startedAt)
	if w.startedAt.IsZero() {
		elapsed = 0
	}

	frameRate := 0.0
	if elapsed > 0 {
		frameRate = float64(w.frames) / elapsed.Seconds()
	}

	rms := 0.0
	if w.samples > 0 {
		rms = math.Sqrt(w.sumSquares / float64(w.samples))
	}

	lastFrameAge := time.Duration(0)
	if !w.lastFrameAt.IsZero() {
		lastFrameAge = now.Sub(w.lastFrameAt)
	}

	return MicStats{
		Elapsed:       elapsed,
		Frames:        w.frames,
		Samples:       w.samples,
		Bytes:         w.bytes,
		FrameRate:     frameRate,
		RMSDBFS:       formatDBFS(rms),
		PeakDBFS:      formatDBFS(float64(w.peakSample)),
		LastFrameAge:  lastFrameAge,
		PeakAmplitude: w.peakSample,
	}
}

func (s MicStats) String() string {
	return fmt.Sprintf(
		"mic stats: elapsed=%s frames=%d samples=%d frameRate=%.0fHz bytes=%d rms=%s peak=%s peakAmp=%d lastFrameAge=%s",
		s.Elapsed.Round(time.Millisecond),
		s.Frames,
		s.Samples,
		s.FrameRate,
		s.Bytes,
		s.RMSDBFS,
		s.PeakDBFS,
		s.PeakAmplitude,
		s.LastFrameAge.Round(time.Millisecond),
	)
}

func formatDBFS(sample float64) string {
	if sample <= 0 {
		return "-inf dBFS"
	}
	db := 20 * math.Log10(sample/32768.0)
	return fmt.Sprintf("%.1f dBFS", db)
}
