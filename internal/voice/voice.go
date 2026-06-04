package voice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	"lylatlink/assets"
	"lylatlink/internal/audio"
	llcodec "lylatlink/internal/codec"
	"lylatlink/internal/signaling"
)

type Controller interface {
	Start(ctx context.Context, matchID string, room signaling.StartResponse) error
	Stop(ctx context.Context, matchID string) error
}

type NoopController struct{}

func (NoopController) Start(_ context.Context, _ string, room signaling.StartResponse) error {
	if room.Status == "ready" {
		log.Printf("voice room ready: token=%s initiator=%v", redactToken(room.RoomToken), room.Initiator)
	}
	return nil
}

func (NoopController) Stop(_ context.Context, matchID string) error {
	log.Printf("voice stopped for %s", matchID)
	return nil
}

type WebRTCController struct {
	SignalClient *signaling.Client
	Options      Options

	mu       sync.Mutex
	sessions map[string]*session
}

type Options struct {
	InputDeviceID     string
	OutputDeviceID    string
	AudioCodec        string
	InputGainDB       float64
	OutputGainDB      float64
	NoiseGateDB       float64
	PlayChimes        bool
	UseSyntheticAudio bool
	DisablePlayback   bool
	Verbose           bool
}

type session struct {
	cancel      context.CancelFunc
	pc          *webrtc.PeerConnection
	ws          *websocket.Conn
	audioPlayed atomic.Bool
}

type signalEnvelope struct {
	Type      string                   `json:"type"`
	SDP       string                   `json:"sdp,omitempty"`
	Candidate *webrtc.ICECandidateInit `json:"candidate,omitempty"`
}

const (
	audioCodecOpus = "opus"
	audioCodecPCMU = "pcmu"
	chimeGainDB    = -1.0
)

var startChimeSamplesOnce sync.Once
var startChimeSamples []int16
var startChimeSamplesErr error

var endChimeSamplesOnce sync.Once
var endChimeSamples []int16
var endChimeSamplesErr error

func NewWebRTCController(signalClient *signaling.Client, options ...Options) *WebRTCController {
	opts := Options{}
	if len(options) > 0 {
		opts = options[0]
	}
	return &WebRTCController{
		SignalClient: signalClient,
		Options:      opts,
		sessions:     map[string]*session{},
	}
}

func normalizeAudioCodec(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "", audioCodecOpus:
		return audioCodecOpus
	case audioCodecPCMU, "g711", "g.711", "ulaw", "mu-law":
		return audioCodecPCMU
	default:
		return strings.ToLower(strings.TrimSpace(codec))
	}
}

func redactToken(token string) string {
	if len(token) <= 8 {
		return "<redacted>"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

func connectionChimeSamples() ([]int16, error) {
	startChimeSamplesOnce.Do(func() {
		startChimeSamples, startChimeSamplesErr = audio.DecodePCM16MonoWAV(assets.StartWAV)
	})
	return startChimeSamples, startChimeSamplesErr
}

func endChimeSamplesPCM() ([]int16, error) {
	endChimeSamplesOnce.Do(func() {
		endChimeSamples, endChimeSamplesErr = audio.DecodePCM16MonoWAV(assets.EndWAV)
	})
	return endChimeSamples, endChimeSamplesErr
}

func (c *WebRTCController) Start(ctx context.Context, matchID string, room signaling.StartResponse) error {
	if room.Status != "ready" {
		return nil
	}
	if c.SignalClient == nil {
		return errors.New("signal client is required")
	}

	wsURL, err := c.SignalClient.WebSocketURL(room.RoomToken)
	if err != nil {
		return err
	}

	log.Printf("voice room ready: token=%s initiator=%v", redactToken(room.RoomToken), room.Initiator)
	log.Printf("webrtc data-channel connecting: %s", matchID)

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	ws, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("connect signaling websocket: %w", err)
	}

	pc, err := webrtc.NewPeerConnection(peerConfig(room.TurnCredentials))
	if err != nil {
		ws.Close()
		return fmt.Errorf("create peer connection: %w", err)
	}

	sessionCtx, cancel := context.WithCancel(ctx)
	current := &session{cancel: cancel, pc: pc, ws: ws}
	c.store(matchID, current)

	sendCh := make(chan signalEnvelope, 32)
	go writeSignals(sessionCtx, ws, sendCh)

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		sendSignal(sessionCtx, sendCh, signalEnvelope{
			Type:      "ice-candidate",
			Candidate: ptr(candidate.ToJSON()),
		})
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("webrtc state %s: %s", matchID, state)
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			cancel()
		}
	})

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		configureDataChannel(matchID, dc)
	})

	if err := c.addAudio(sessionCtx, matchID, pc); err != nil {
		c.Stop(context.Background(), matchID)
		return err
	}

	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if c.Options.Verbose {
			log.Printf("remote media track: %s kind=%s codec=%s", matchID, track.Kind(), track.Codec().MimeType)
		}
		go c.drainRemoteTrack(sessionCtx, matchID, track)
	})

	if room.Initiator {
		dc, err := pc.CreateDataChannel("lylatlink-smoke", nil)
		if err != nil {
			c.Stop(context.Background(), matchID)
			return fmt.Errorf("create data channel: %w", err)
		}
		configureDataChannel(matchID, dc)

		offer, err := pc.CreateOffer(nil)
		if err != nil {
			c.Stop(context.Background(), matchID)
			return fmt.Errorf("create offer: %w", err)
		}
		if err := pc.SetLocalDescription(offer); err != nil {
			c.Stop(context.Background(), matchID)
			return fmt.Errorf("set local offer: %w", err)
		}
		sendSignal(sessionCtx, sendCh, signalEnvelope{Type: "offer", SDP: offer.SDP})
	}

	go readSignals(sessionCtx, matchID, pc, ws, sendCh, cancel)
	return nil
}

func (c *WebRTCController) Stop(ctx context.Context, matchID string) error {
	c.mu.Lock()
	current := c.sessions[matchID]
	delete(c.sessions, matchID)
	c.mu.Unlock()
	if current == nil {
		return nil
	}
	current.cancel()
	if current.ws != nil {
		_ = current.ws.Close()
	}
	if current.pc != nil {
		_ = current.pc.Close()
	}
	if current.audioPlayed.Load() {
		c.playEndChime(ctx, matchID)
	}
	log.Printf("voice stopped for %s", matchID)
	return nil
}

func (c *WebRTCController) store(matchID string, current *session) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing := c.sessions[matchID]; existing != nil {
		existing.cancel()
		if existing.ws != nil {
			_ = existing.ws.Close()
		}
		if existing.pc != nil {
			_ = existing.pc.Close()
		}
	}
	c.sessions[matchID] = current
}

func peerConfig(turn signaling.TurnCredentials) webrtc.Configuration {
	config := webrtc.Configuration{}
	if len(turn.URLs) > 0 {
		config.ICEServers = []webrtc.ICEServer{{
			URLs:       turn.URLs,
			Username:   turn.Username,
			Credential: turn.Credential,
		}}
		config.ICETransportPolicy = webrtc.ICETransportPolicyRelay
	}
	return config
}

func configureDataChannel(matchID string, dc *webrtc.DataChannel) {
	dc.OnOpen(func() {
		log.Printf("webrtc data channel open: %s label=%s", matchID, dc.Label())
		if err := dc.SendText(fmt.Sprintf("hello from LylatLink match %s", matchID)); err != nil {
			log.Printf("data channel send failed: %v", err)
		}
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		log.Printf("webrtc data channel message: %s %q", matchID, string(msg.Data))
	})
	dc.OnClose(func() {
		log.Printf("webrtc data channel closed: %s", matchID)
	})
}

func (c *WebRTCController) addAudio(ctx context.Context, matchID string, pc *webrtc.PeerConnection) error {
	if c.Options.UseSyntheticAudio {
		log.Printf("using synthetic PCMU audio: %s", matchID)
		return addSyntheticAudio(ctx, matchID, pc)
	}

	codec := normalizeAudioCodec(c.Options.AudioCodec)
	switch codec {
	case audioCodecOpus:
		if err := addMicOpus(ctx, matchID, pc, c.Options); err != nil {
			log.Printf("Opus mic audio unavailable; falling back to PCMU: %s: %v", matchID, err)
			if fallbackErr := addMicPCMU(ctx, matchID, pc, c.Options); fallbackErr == nil {
				return nil
			} else {
				log.Printf("live PCMU mic audio unavailable; falling back to synthetic PCMU: %s: %v", matchID, fallbackErr)
			}
			return addSyntheticAudio(ctx, matchID, pc)
		}
		return nil
	case audioCodecPCMU:
		if err := addMicPCMU(ctx, matchID, pc, c.Options); err != nil {
			log.Printf("live PCMU mic audio unavailable; falling back to synthetic PCMU: %s: %v", matchID, err)
			return addSyntheticAudio(ctx, matchID, pc)
		}
		return nil
	default:
		log.Printf("unknown audio codec %q; falling back to synthetic PCMU: %s", c.Options.AudioCodec, matchID)
		return addSyntheticAudio(ctx, matchID, pc)
	}
}

func addMicOpus(ctx context.Context, matchID string, pc *webrtc.PeerConnection, opts Options) error {
	capture, err := audio.StartCapture(ctx, audio.CaptureOptions{
		InputDeviceID:     opts.InputDeviceID,
		FallbackToDefault: true,
		Verbose:           opts.Verbose,
	})
	if err != nil {
		return err
	}

	encoder, err := llcodec.NewOpusEncoder(llcodec.OpusBitrate)
	if err != nil {
		capture.Stop()
		return err
	}

	track, err := newOpusTrack("lylatlink-mic-opus")
	if err != nil {
		capture.Stop()
		encoder.Close()
		return err
	}

	sender, err := pc.AddTrack(track)
	if err != nil {
		capture.Stop()
		encoder.Close()
		return fmt.Errorf("add Opus audio track: %w", err)
	}
	go drainSenderRTCP(sender)

	if opts.Verbose {
		log.Printf(
			"live mic audio started: %s device=%q sampleRate=%d channels=%d format=%s codec=Opus bitrate=%dbps inputGain=%.1fdB noiseGate=%.1fdBFS",
			matchID,
			capture.DeviceName,
			capture.SampleRate,
			capture.Channels,
			capture.Format,
			llcodec.OpusBitrate,
			opts.InputGainDB,
			normalizeNoiseGateDB(opts.NoiseGateDB),
		)
	}

	go func() {
		defer capture.Stop()
		defer encoder.Close()
		sendMicOpus(ctx, matchID, track, encoder, capture.Frames, newMicProcessor(opts.InputGainDB, opts.NoiseGateDB))
	}()
	return nil
}

func addMicPCMU(ctx context.Context, matchID string, pc *webrtc.PeerConnection, opts Options) error {
	capture, err := audio.StartCapture(ctx, audio.CaptureOptions{
		InputDeviceID:     opts.InputDeviceID,
		FallbackToDefault: true,
		Verbose:           opts.Verbose,
	})
	if err != nil {
		return err
	}

	track, err := newPCMUTrack("lylatlink-mic-audio")
	if err != nil {
		capture.Stop()
		return err
	}

	sender, err := pc.AddTrack(track)
	if err != nil {
		capture.Stop()
		return fmt.Errorf("add mic audio track: %w", err)
	}
	go drainSenderRTCP(sender)

	if opts.Verbose {
		log.Printf(
			"live mic audio started: %s device=%q sampleRate=%d channels=%d format=%s codec=PCMU inputGain=%.1fdB noiseGate=%.1fdBFS",
			matchID,
			capture.DeviceName,
			capture.SampleRate,
			capture.Channels,
			capture.Format,
			opts.InputGainDB,
			normalizeNoiseGateDB(opts.NoiseGateDB),
		)
	}

	go func() {
		defer capture.Stop()
		sendMicPCMU(ctx, matchID, track, capture.Frames, newMicProcessor(opts.InputGainDB, opts.NoiseGateDB))
	}()
	return nil
}

func newOpusTrack(id string) (*webrtc.TrackLocalStaticSample, error) {
	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType:     webrtc.MimeTypeOpus,
			ClockRate:    llcodec.OpusSampleRate,
			Channels:     llcodec.OpusRTPChannels,
			SDPFmtpLine:  "minptime=10;useinbandfec=1",
			RTCPFeedback: nil,
		},
		id,
		"lylatlink",
	)
	if err != nil {
		return nil, fmt.Errorf("create Opus audio track: %w", err)
	}
	return track, nil
}

func addSyntheticAudio(ctx context.Context, matchID string, pc *webrtc.PeerConnection) error {
	track, err := newPCMUTrack("lylatlink-synthetic-audio")
	if err != nil {
		return err
	}

	sender, err := pc.AddTrack(track)
	if err != nil {
		return fmt.Errorf("add synthetic audio track: %w", err)
	}
	go drainSenderRTCP(sender)

	go sendSyntheticPCMU(ctx, matchID, track)
	return nil
}

func newPCMUTrack(id string) (*webrtc.TrackLocalStaticSample, error) {
	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000},
		id,
		"lylatlink",
	)
	if err != nil {
		return nil, fmt.Errorf("create PCMU audio track: %w", err)
	}
	return track, nil
}

func drainSenderRTCP(sender *webrtc.RTPSender) {
	buf := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buf); err != nil {
			return
		}
	}
}

type micProcessor struct {
	gainDB      float64
	noiseGateDB float64
	holdFrames  int
}

func newMicProcessor(inputGainDB, noiseGateDB float64) *micProcessor {
	return &micProcessor{
		gainDB:      inputGainDB,
		noiseGateDB: normalizeNoiseGateDB(noiseGateDB),
	}
}

func (p *micProcessor) process(frame []int16) []int16 {
	if p == nil || len(frame) == 0 {
		return frame
	}

	if p.noiseGateDB > -100 {
		rms := frameRMS(frame)
		if rms > 0 && dbFS(rms) >= p.noiseGateDB {
			p.holdFrames = 6
		} else if p.holdFrames > 0 {
			p.holdFrames--
		} else {
			clearFrame(frame)
		}
	}

	applyGainInPlace(frame, p.gainDB)
	return frame
}

func normalizeNoiseGateDB(value float64) float64 {
	if value == 0 {
		return -45
	}
	return value
}

func applyGainInPlace(frame []int16, gainDB float64) {
	if len(frame) == 0 || gainDB == 0 {
		return
	}
	gain := math.Pow(10, gainDB/20)
	for i, sample := range frame {
		frame[i] = clipSample(float64(sample) * gain)
	}
}

func clearFrame(frame []int16) {
	for i := range frame {
		frame[i] = 0
	}
}

func frameRMS(frame []int16) float64 {
	if len(frame) == 0 {
		return 0
	}
	sumSquares := 0.0
	for _, sample := range frame {
		v := float64(sample)
		sumSquares += v * v
	}
	return math.Sqrt(sumSquares / float64(len(frame)))
}

func dbFS(sample float64) float64 {
	if sample <= 0 {
		return math.Inf(-1)
	}
	return 20 * math.Log10(sample/32768.0)
}

func clipSample(value float64) int16 {
	if value > 32767 {
		return 32767
	}
	if value < -32768 {
		return -32768
	}
	return int16(math.Round(value))
}

func sendMicOpus(ctx context.Context, matchID string, track *webrtc.TrackLocalStaticSample, encoder *llcodec.OpusEncoder, frames <-chan []int16, processor *micProcessor) {
	out := make([]byte, llcodec.OpusMaxPacketBytes)
	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-frames:
			if !ok {
				return
			}
			frame = processor.process(frame)
			n, err := encoder.Encode(frame, out)
			if err != nil {
				log.Printf("Opus audio encode failed: %s: %v", matchID, err)
				return
			}
			if err := track.WriteSample(media.Sample{Data: append([]byte(nil), out[:n]...), Duration: audio.FrameDuration}); err != nil {
				log.Printf("Opus audio send failed: %s: %v", matchID, err)
				return
			}
		}
	}
}

func sendMicPCMU(ctx context.Context, matchID string, track *webrtc.TrackLocalStaticSample, frames <-chan []int16, processor *micProcessor) {
	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-frames:
			if !ok {
				return
			}
			frame = processor.process(frame)
			payload := encodePCMUFrame(frame)
			if err := track.WriteSample(media.Sample{Data: payload, Duration: audio.FrameDuration}); err != nil {
				log.Printf("mic audio send failed: %s: %v", matchID, err)
				return
			}
		}
	}
}

func sendSyntheticPCMU(ctx context.Context, matchID string, track *webrtc.TrackLocalStaticSample) {
	silence := make([]byte, 160)
	for i := range silence {
		silence[i] = 0xff
	}

	ticker := time.NewTicker(audio.FrameDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := track.WriteSample(media.Sample{Data: silence, Duration: audio.FrameDuration}); err != nil {
				log.Printf("synthetic audio send failed: %s: %v", matchID, err)
				return
			}
		}
	}
}

type remoteAudioDecoder interface {
	Decode(payload []byte) ([]int16, error)
	Close()
}

type pcmuRemoteDecoder struct{}

type opusRemoteDecoder struct {
	decoder *llcodec.OpusDecoder
	buffer  []int16
}

func newRemoteAudioDecoder(mimeType string) (remoteAudioDecoder, error) {
	switch {
	case isPCMU(mimeType):
		return pcmuRemoteDecoder{}, nil
	case isOpus(mimeType):
		decoder, err := llcodec.NewOpusDecoder()
		if err != nil {
			return nil, err
		}
		return &opusRemoteDecoder{
			decoder: decoder,
			buffer:  make([]int16, llcodec.OpusSampleRate*60/1000),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported audio codec")
	}
}

func (pcmuRemoteDecoder) Decode(payload []byte) ([]int16, error) {
	return decodePCMUFrame(payload), nil
}

func (pcmuRemoteDecoder) Close() {}

func (d *opusRemoteDecoder) Decode(payload []byte) ([]int16, error) {
	n, err := d.decoder.Decode(payload, d.buffer)
	if err != nil {
		return nil, err
	}
	frame := make([]int16, n)
	copy(frame, d.buffer[:n])
	return frame, nil
}

func (d *opusRemoteDecoder) Close() {
	d.decoder.Close()
}

func (c *WebRTCController) drainRemoteTrack(ctx context.Context, matchID string, track *webrtc.TrackRemote) {
	codec := track.Codec()
	decoder, err := newRemoteAudioDecoder(codec.MimeType)
	if err != nil {
		log.Printf("remote audio decode disabled: %s codec=%s: %v", matchID, codec.MimeType, err)
	}
	if decoder != nil {
		defer decoder.Close()
	}

	var playback *audio.Playback
	if c.Options.DisablePlayback {
		log.Printf("remote audio playback disabled: %s", matchID)
	} else {
		playback, err = audio.StartPlayback(ctx, audio.PlaybackOptions{
			OutputDeviceID:    c.Options.OutputDeviceID,
			FallbackToDefault: true,
			Verbose:           c.Options.Verbose,
		})
		if err != nil {
			log.Printf("remote audio playback unavailable: %s: %v", matchID, err)
		} else {
			defer playback.Stop()
			c.markAudioPlayed(matchID)
			if c.Options.PlayChimes {
				playConnectionChime(ctx, matchID, playback, c.Options.Verbose)
			}
			if c.Options.Verbose {
				log.Printf(
					"remote audio playback started: %s device=%q sampleRate=%d channels=%d format=%s outputGain=%.1fdB",
					matchID,
					playback.DeviceName,
					playback.SampleRate,
					playback.Channels,
					playback.Format,
					c.Options.OutputGainDB,
				)
			}
		}
	}

	var metrics *audioMetrics
	if c.Options.Verbose {
		metrics = newAudioMetrics(matchID, codec.MimeType, codec.ClockRate, time.Now())
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		packet, _, err := track.ReadRTP()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("remote media read ended: %s: %v", matchID, err)
			}
			return
		}

		var pcm []int16
		if decoder != nil {
			pcm, err = decoder.Decode(packet.Payload)
			if err != nil {
				log.Printf("remote audio decode failed: %s codec=%s: %v", matchID, codec.MimeType, err)
				continue
			}
		}
		if playback != nil && len(pcm) > 0 {
			applyGainInPlace(pcm, c.Options.OutputGainDB)
			playback.Write(pcm)
		}
		if metrics == nil {
			continue
		}
		if line := metrics.observe(packet.SequenceNumber, packet.Payload, len(packet.Payload)+12, time.Now(), pcm); line != "" {
			log.Print(line)
		}
	}
}

func (c *WebRTCController) markAudioPlayed(matchID string) {
	c.mu.Lock()
	current := c.sessions[matchID]
	c.mu.Unlock()
	if current != nil {
		current.audioPlayed.Store(true)
	}
}

func playConnectionChime(ctx context.Context, matchID string, playback *audio.Playback, verbose bool) {
	samples, err := connectionChimeSamples()
	if err != nil {
		log.Printf("connection chime unavailable: %s: %v", matchID, err)
		return
	}
	if verbose {
		log.Printf("connection chime started: %s duration=%s", matchID, durationForAudioSamples(len(samples)).Round(time.Millisecond))
	}
	chime := append([]int16(nil), samples...)
	applyGainInPlace(chime, chimeGainDB)
	playback.PlayPCM(ctx, chime)
}

func (c *WebRTCController) playEndChime(ctx context.Context, matchID string) {
	if c.Options.DisablePlayback || !c.Options.PlayChimes {
		return
	}
	samples, err := endChimeSamplesPCM()
	if err != nil {
		log.Printf("end chime unavailable: %s: %v", matchID, err)
		return
	}
	chimeCtx, cancel := context.WithTimeout(ctx, durationForAudioSamples(len(samples))+500*time.Millisecond)
	defer cancel()
	playback, err := audio.StartPlayback(chimeCtx, audio.PlaybackOptions{
		OutputDeviceID:    c.Options.OutputDeviceID,
		FallbackToDefault: true,
		Verbose:           c.Options.Verbose,
		BufferFrames:      100,
	})
	if err != nil {
		log.Printf("end chime playback unavailable: %s: %v", matchID, err)
		return
	}
	defer playback.Stop()
	if c.Options.Verbose {
		log.Printf("end chime started: %s duration=%s", matchID, durationForAudioSamples(len(samples)).Round(time.Millisecond))
	}
	chime := append([]int16(nil), samples...)
	applyGainInPlace(chime, chimeGainDB)
	playback.PlayPCM(chimeCtx, chime)
}

func durationForAudioSamples(samples int) time.Duration {
	return time.Duration(samples) * time.Second / audio.SampleRate
}

type audioMetrics struct {
	matchID   string
	codec     string
	clockRate uint32

	windowStart time.Time

	totalPackets       uint64
	totalGaps          uint64
	windowPackets      uint64
	windowRTPBytes     uint64
	windowPayloadBytes uint64
	windowGaps         uint64

	haveSeq bool
	lastSeq uint16

	sumSquares  float64
	sampleCount uint64
	peakSample  int
}

func newAudioMetrics(matchID, codec string, clockRate uint32, now time.Time) *audioMetrics {
	return &audioMetrics{
		matchID:     matchID,
		codec:       codec,
		clockRate:   clockRate,
		windowStart: now,
	}
}

func (m *audioMetrics) observe(sequenceNumber uint16, payload []byte, rtpBytes int, now time.Time, levelFrame []int16) string {
	m.observeSequence(sequenceNumber)
	m.totalPackets++
	m.windowPackets++
	m.windowRTPBytes += uint64(rtpBytes)
	m.windowPayloadBytes += uint64(len(payload))
	m.observeAudioLevel(payload, levelFrame)

	elapsed := now.Sub(m.windowStart)
	if elapsed < time.Second {
		return ""
	}

	line := m.format(elapsed)
	m.resetWindow(now)
	return line
}

func (m *audioMetrics) observeSequence(sequenceNumber uint16) {
	if !m.haveSeq {
		m.haveSeq = true
		m.lastSeq = sequenceNumber
		return
	}

	delta := sequenceNumber - m.lastSeq
	if delta > 1 && delta < 0x8000 {
		gaps := uint64(delta - 1)
		m.windowGaps += gaps
		m.totalGaps += gaps
	}
	if delta > 0 && delta < 0x8000 {
		m.lastSeq = sequenceNumber
	}
}

func (m *audioMetrics) observeAudioLevel(payload []byte, levelFrame []int16) {
	if len(levelFrame) > 0 {
		m.observePCM(levelFrame)
		return
	}
	if isPCMU(m.codec) {
		m.observePCM(decodePCMUSamples(payload))
	}
}

func (m *audioMetrics) observePCM(frame []int16) {
	for _, value := range frame {
		sample := int(value)
		if sample < 0 {
			sample = -sample
		}
		if sample > m.peakSample {
			m.peakSample = sample
		}
		m.sumSquares += float64(sample * sample)
		m.sampleCount++
	}
}

func (m *audioMetrics) format(elapsed time.Duration) string {
	seconds := elapsed.Seconds()
	packetRate := float64(m.windowPackets) / seconds
	rtpKbps := float64(m.windowRTPBytes*8) / seconds / 1000
	payloadKbps := float64(m.windowPayloadBytes*8) / seconds / 1000

	avgPayload := 0.0
	if m.windowPackets > 0 {
		avgPayload = float64(m.windowPayloadBytes) / float64(m.windowPackets)
	}

	return fmt.Sprintf(
		"audio stream stats: match=%s codec=%s clock=%dHz pkts=%d pps=%.1f rtp=%.1fkbps payload=%.1fkbps avgPayload=%.0fB seqGaps=%d totalSeqGaps=%d levelRMS=%s levelPeak=%s",
		m.matchID,
		m.codec,
		m.clockRate,
		m.windowPackets,
		packetRate,
		rtpKbps,
		payloadKbps,
		avgPayload,
		m.windowGaps,
		m.totalGaps,
		formatDBFS(m.rms()),
		formatDBFS(float64(m.peakSample)),
	)
}

func (m *audioMetrics) resetWindow(now time.Time) {
	m.windowStart = now
	m.windowPackets = 0
	m.windowRTPBytes = 0
	m.windowPayloadBytes = 0
	m.windowGaps = 0
	m.sumSquares = 0
	m.sampleCount = 0
	m.peakSample = 0
}

func (m *audioMetrics) rms() float64 {
	if m.sampleCount == 0 {
		return 0
	}
	return math.Sqrt(m.sumSquares / float64(m.sampleCount))
}

func encodePCMUFrame(frame []int16) []byte {
	const samplesPerPCMUFrame = 160
	const downsampleRatio = audio.SampleRate / 8000

	payload := make([]byte, samplesPerPCMUFrame)
	for i := range payload {
		start := i * downsampleRatio
		if start >= len(frame) {
			payload[i] = encodePCMU(0)
			continue
		}
		end := start + downsampleRatio
		if end > len(frame) {
			end = len(frame)
		}
		sum := 0
		for _, sample := range frame[start:end] {
			sum += int(sample)
		}
		payload[i] = encodePCMU(int16(sum / (end - start)))
	}
	return payload
}

func decodePCMUFrame(payload []byte) []int16 {
	const upsampleRatio = audio.SampleRate / 8000
	frame := make([]int16, len(payload)*upsampleRatio)
	for i, value := range payload {
		sample := decodePCMU(value)
		start := i * upsampleRatio
		for j := 0; j < upsampleRatio; j++ {
			frame[start+j] = sample
		}
	}
	return frame
}

func decodePCMUSamples(payload []byte) []int16 {
	frame := make([]int16, len(payload))
	for i, value := range payload {
		frame[i] = decodePCMU(value)
	}
	return frame
}

func encodePCMU(sample int16) byte {
	const bias = 0x84
	const clip = 32635

	pcm := int(sample)
	sign := 0
	if pcm < 0 {
		sign = 0x80
		pcm = -pcm
	}
	if pcm > clip {
		pcm = clip
	}
	pcm += bias

	exponent := 7
	for expMask := 0x4000; (pcm&expMask) == 0 && exponent > 0; expMask >>= 1 {
		exponent--
	}
	mantissa := (pcm >> (exponent + 3)) & 0x0f
	return byte(^(sign | (exponent << 4) | mantissa))
}

func decodePCMU(value byte) int16 {
	value = ^value
	sign := value & 0x80
	exponent := (value >> 4) & 0x07
	mantissa := value & 0x0f
	sample := int(((uint16(mantissa) << 3) + 0x84) << exponent)
	sample -= 0x84
	if sign != 0 {
		sample = -sample
	}
	return int16(sample)
}

func formatDBFS(sample float64) string {
	if sample <= 0 {
		return "-inf dBFS"
	}
	db := 20 * math.Log10(sample/32768.0)
	return fmt.Sprintf("%.1f dBFS", db)
}

func isPCMU(codec string) bool {
	return strings.EqualFold(codec, webrtc.MimeTypePCMU)
}

func isOpus(codec string) bool {
	return strings.EqualFold(codec, webrtc.MimeTypeOpus)
}

func readSignals(ctx context.Context, matchID string, pc *webrtc.PeerConnection, ws *websocket.Conn, sendCh chan<- signalEnvelope, cancel context.CancelFunc) {
	defer cancel()

	queuedCandidates := []webrtc.ICECandidateInit{}
	applyQueued := func() {
		if pc.RemoteDescription() == nil {
			return
		}
		for _, candidate := range queuedCandidates {
			if err := pc.AddICECandidate(candidate); err != nil {
				log.Printf("add queued ICE candidate failed: %v", err)
			}
		}
		queuedCandidates = nil
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var msg signalEnvelope
		if err := ws.ReadJSON(&msg); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("signaling websocket read failed: %v", err)
			}
			return
		}

		switch msg.Type {
		case "offer":
			if pc.RemoteDescription() != nil {
				continue
			}
			if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: msg.SDP}); err != nil {
				log.Printf("set remote offer failed: %v", err)
				continue
			}
			applyQueued()
			answer, err := pc.CreateAnswer(nil)
			if err != nil {
				log.Printf("create answer failed: %v", err)
				continue
			}
			if err := pc.SetLocalDescription(answer); err != nil {
				log.Printf("set local answer failed: %v", err)
				continue
			}
			sendSignal(ctx, sendCh, signalEnvelope{Type: "answer", SDP: answer.SDP})
		case "answer":
			if pc.RemoteDescription() != nil {
				continue
			}
			if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: msg.SDP}); err != nil {
				log.Printf("set remote answer failed: %v", err)
				continue
			}
			applyQueued()
		case "ice-candidate":
			if msg.Candidate == nil {
				continue
			}
			if pc.RemoteDescription() == nil {
				queuedCandidates = append(queuedCandidates, *msg.Candidate)
				continue
			}
			if err := pc.AddICECandidate(*msg.Candidate); err != nil {
				log.Printf("add ICE candidate failed: %v", err)
			}
		case "room_closed":
			log.Printf("signaling room closed: %s", matchID)
			return
		default:
			raw, _ := json.Marshal(msg)
			log.Printf("ignored signaling message: %s", raw)
		}
	}
}

func writeSignals(ctx context.Context, ws *websocket.Conn, sendCh <-chan signalEnvelope) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-sendCh:
			if err := ws.WriteJSON(msg); err != nil {
				log.Printf("signaling websocket write failed: %v", err)
				return
			}
		}
	}
}

func sendSignal(ctx context.Context, sendCh chan<- signalEnvelope, msg signalEnvelope) {
	select {
	case <-ctx.Done():
	case sendCh <- msg:
	}
}

func ptr[T any](value T) *T {
	return &value
}
