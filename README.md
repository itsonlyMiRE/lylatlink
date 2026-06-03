<h1>
  <img src="assets/icon.png" width="36" height="36" alt="LylatLink logo" align="top">
  LylatLink
</h1>

LylatLink is a small companion app for Slippi that adds voice chat. It is designed to be extremely simple to use - leave it running in the system tray, play Slippi, and it automatically opens a voice call when both players in a match are using LylatLink. 

- Tiny executable size
- Simple UI
- No login
- Only one required config piece: set .slp replay folder

## Download

Get the latest builds from [Releases](https://github.com/itsonlyMiRE/lylatlink/releases/latest).

- [Windows](https://github.com/itsonlyMiRE/lylatlink/releases/latest/download/lylatlink-windows-amd64.zip)
- [macOS](https://github.com/itsonlyMiRE/lylatlink/releases/latest/download/lylatlink-macos-arm64.zip)

## Setup

1. Download the latest release for your platform.
2. Run LylatLink.
3. Open the tray menu and choose **Replay Folder**.
4. Select the replay folder configured in Slippi Launcher.

If needed, you can find the folder location in Slippi Launcher under Replay settings:

![Slippi Launcher replay folder setting](docs/images/slippi-replay-folder.png)

That is the only required setup.

LylatLink supports both Slippi replay layouts:

```text
Slippi/*.slp
Slippi/YYYY-MM/*.slp
```

## Audio

LylatLink uses your system default microphone and speaker/headphones by default.

The tray menu includes input and output device pickers. Choosing **System Default** lets your operating system continue routing audio to the current default devices.

Default audio settings:

```toml
input_gain_db = 0.0
output_gain_db = -1.5
noise_gate_threshold_db = -55.0
```

You can edit these from the tray menu with **Edit Config File**. Changes apply to the next voice session.

## Notes

- LylatLink runs from the system tray by default.
- The call ends automatically when the match ends.
- **End Call** in the tray menu manually disconnects the current voice session.
- No login is required.

Developer notes live in [docs/DEV.md](docs/DEV.md).
