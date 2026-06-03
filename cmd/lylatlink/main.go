package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lylatlink/internal/app"
	"lylatlink/internal/audio"
	"lylatlink/internal/config"
	"lylatlink/internal/console"
	"lylatlink/internal/tray"
)

func main() {
	var (
		configPath        = flag.String("config", "", "path to config.toml")
		replayDir         = flag.String("replay-dir", "", "override replay directory")
		signalURL         = flag.String("signal-url", "", "override signaling server URL")
		autoJoin          = flag.Bool("auto-join", false, "join voice automatically when a match is detected")
		once              = flag.String("parse-once", "", "parse one .slp file and print match metadata")
		listAudioDevices  = flag.Bool("list-audio-devices", false, "list input devices and exit")
		listOutputDevices = flag.Bool("list-output-devices", false, "list output devices and exit")
		audioInputDevice  = flag.String("audio-input-device", "", "input device ID or exact name for mic capture")
		audioOutputDevice = flag.String("audio-output-device", "", "output device ID or exact name for remote playback")
		audioCodec        = flag.String("audio-codec", "", "voice codec: opus or pcmu")
		audioTest         = flag.Bool("audio-device-test", false, "capture microphone and print level diagnostics")
		audioDur          = flag.Duration("audio-test-duration", 10*time.Second, "duration for -audio-device-test")
		audioDebug        = flag.Bool("audio-test-verbose", false, "include low-level audio backend logs during -audio-device-test")
		syntheticAudio    = flag.Bool("synthetic-audio", false, "send synthetic PCMU audio instead of live microphone audio")
		noPlayback        = flag.Bool("no-playback", false, "disable remote speaker playback")
		ignoreMatchEnd    = flag.Bool("ignore-match-end", false, "test mode: keep voice open when copied replays contain Game End")
		trayMode          = flag.Bool("tray", true, "run with system tray menu")
		consoleMode       = flag.Bool("console", false, "run in the foreground without the system tray")
	)
	flag.Parse()

	needsConsole := *consoleMode || *once != "" || *listAudioDevices || *listOutputDevices || *audioTest
	if needsConsole {
		if err := console.Enable(); err != nil {
			log.Printf("enable console failed: %v", err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *once != "" {
		if err := app.ParseOnce(os.Stdout, *once); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *listAudioDevices {
		devices, err := audio.ListInputDevices()
		if err != nil {
			log.Fatal(err)
		}
		if len(devices) == 0 {
			fmt.Println("no input devices found")
			return
		}
		for _, device := range devices {
			defaultMark := ""
			if device.IsDefault {
				defaultMark = " (default)"
			}
			fmt.Printf("%s\t%s%s\n", device.ID, device.Name, defaultMark)
		}
		return
	}
	if *listOutputDevices {
		devices, err := audio.ListOutputDevices()
		if err != nil {
			log.Fatal(err)
		}
		if len(devices) == 0 {
			fmt.Println("no output devices found")
			return
		}
		for _, device := range devices {
			defaultMark := ""
			if device.IsDefault {
				defaultMark = " (default)"
			}
			fmt.Printf("%s\t%s%s\n", device.ID, device.Name, defaultMark)
		}
		return
	}
	if *audioTest {
		if err := audio.RunDeviceTest(ctx, audio.DeviceTestOptions{
			Duration:      *audioDur,
			Verbose:       *audioDebug,
			InputDeviceID: *audioInputDevice,
		}); err != nil {
			log.Fatal(err)
		}
		return
	}

	resolvedConfigPath := *configPath
	if resolvedConfigPath == "" {
		var err error
		resolvedConfigPath, err = config.DefaultPath()
		if err != nil {
			log.Fatal(err)
		}
	}

	cfg, err := config.LoadOrCreate(resolvedConfigPath)
	if err != nil {
		log.Fatal(err)
	}

	if *replayDir != "" {
		cfg.ReplayDir = *replayDir
	}
	if *signalURL != "" {
		cfg.SignalBaseURL = *signalURL
	}
	if *autoJoin {
		cfg.AutoJoin = true
	}
	if *audioInputDevice != "" {
		cfg.InputDeviceID = *audioInputDevice
	}
	if *audioOutputDevice != "" {
		cfg.OutputDeviceID = *audioOutputDevice
	}
	if *audioCodec != "" {
		cfg.AudioCodec = *audioCodec
	}
	useTray := *trayMode && !*consoleMode
	if cfg.ReplayDir == "" && !useTray {
		path, _ := config.DefaultPath()
		fmt.Fprintf(os.Stderr, "missing replay_dir; set it in %s or pass -replay-dir\n", path)
		os.Exit(2)
	}

	opts := app.Options{SyntheticAudio: *syntheticAudio, NoPlayback: *noPlayback, IgnoreMatchEnd: *ignoreMatchEnd}
	if useTray {
		trayCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		controller := app.NewController(cfg, resolvedConfigPath, opts)
		errs := make(chan error, 1)
		go func() {
			errs <- controller.Run(trayCtx)
		}()
		tray.Run(trayCtx, controller)
		cancel()
		if err := <-errs; err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := app.Run(ctx, cfg, opts); err != nil {
		log.Fatal(err)
	}
}
