<h1>
  <img src="assets/icon.png" width="36" height="36" alt="LylatLink logo" align="top">
  LylatLink - Slippi Voice Chat
</h1>

**[DOWNLOAD](#download)**

LylatLink is a small open-source companion app for Slippi that adds voice chat with opponents. It is designed with simplicity in mind - leave it running in the system tray, play Slippi, and it automatically opens a voice call when both players in a match are using LylatLink. **No setup required.**

Core details:
- **Tiny executable size** - built fully in Go; no Electron nonsense
- **Fully open source**
- **ZERO setup required** - just run the app and verify the auto-detected .slp folder is where your new .slp files get created
- **Simple UI** - little menu in your system tray + an info and status overlay in-game [overlay WIP] 
- **No login**, just parses new .slp files and matches players accordingly
- **End Call** menu button and F8 hotkey: quickly terminate calls with toxic players
- **Player IP protection** - audio is P2P but routed through a relay server
- Absolutely **zero server-side capture or analysis of audio**, nothing is stored, the server is purely a relay to protect player IPs
- Cost is out of my pocket but cheap :,)
- Matches with >2 players not currently supported, but can be if there is demand (or someone can raise a PR)

## Download

- [Windows](https://github.com/itsonlyMiRE/lylatlink/releases/latest/download/lylatlink-windows-amd64.zip)
- [macOS](https://github.com/itsonlyMiRE/lylatlink/releases/latest/download/lylatlink-macos-arm64.zip) (M1/M2/M3/M4 Apple Silicon)
- Linux and Intel macOS are not currently supported.

## Setup

1. Download the latest release for your platform above.
2. Run LylatLink
    - Windows: Unzip, run `lylatlink.exe`
    - Mac: Unzip, move `LylatLink.app` into your `Applications` folder and run.

      NOTE: I have no interest in paying $99/yr to Apple so you will probably get a security warning when running for the first time. [Click here for the steps to remediate this and trust the app.](docs/apple-security-fix.md)
3. Play!

To use LylatLink, Slippi must be configured to save .slp replays. LylatLink will try to find your Slippi replay folder automatically using your Slippi Launcher and Dolphin settings. If it does not, open the tray menu, choose **Replay Folder**, and select the folder that stores your .slp replays. ***Note:** If you organize replays into monthly subfolders, choose the parent folder; i.e., if they save to [path]/Slippi/2026-06, select the 'Slippi' folder.*

If this is not working for whatever reason, you can find the folder location in Slippi Launcher under Replay settings:

![Slippi Launcher replay folder setting](docs/images/slippi-replay-folder.png)

## Audio

LylatLink uses your system default microphone and speaker/headphones by default.

The tray menu includes input and output device pickers. Choosing **System Default** lets your operating system continue routing audio to the current default devices.

Default audio settings:

```toml
input_gain_db = 0.0
output_gain_db = -1.5
noise_gate_threshold_db = -45.0
```

You can edit these from the tray menu with **Edit Config File**. Changes apply to the next voice session.

## Notes

Developer notes live in [docs/DEV.md](docs/DEV.md).
