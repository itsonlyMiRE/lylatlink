# LylatLink

LylatLink is a small Slippi voice companion. Leave it running in the tray, play Slippi, and it automatically opens a voice call when both players in a match are using LylatLink.

## Setup

1. Download the latest release for your platform.
2. Run LylatLink.
3. Open the tray menu and choose **Replay Folder**.
4. Select the replay folder configured in Slippi Launcher.

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

Developer notes live in [DEV.md](DEV.md).
