<h1>
  <img src="assets/icon.png" width="36" height="36" alt="LylatLink logo" align="top">
  LylatLink - Slippi Voice Chat
</h1>

**[DOWNLOAD](#download)** | **[SETUP](#setup)**

LylatLink is a small open-source companion app for Slippi that adds voice chat with opponents. It is designed with simplicity in mind - run the launcher app, play Slippi, and a voice call will start automatically when both players in a match are using LylatLink. **No setup required.**

Core details:
- **Tiny <15MB executable size** - built fully in Go; no large resource-hungry Electron app
- **Negligible resource consumption** - <50MB memory
- **Fully open source**
- **ZERO setup required** in most cases - use the launch-together app and play normally
- **Simple UI** - little menu in your system tray + an info and status overlay in-game [overlay WIP] 
- **No login** - just parses new .slp files and matches players accordingly
- **End Call** menu button and configurable hotkey; quickly terminate calls with toxic players
- **Player IP protection** - audio is P2P but routed through a relay server
- Absolutely **zero server-side capture or analysis of audio**, nothing is stored, the server is purely a relay to protect player IPs
- Cost is out of my pocket but cheap :,)
- Matches with >2 players not currently supported, but can be if there is demand (or someone can raise a PR)

## Download

- [Windows](https://github.com/itsonlyMiRE/lylatlink/releases/latest/download/lylatlink-windows-amd64.zip)
- [macOS](https://github.com/itsonlyMiRE/lylatlink/releases/latest/download/lylatlink-macos-arm64.zip) (M1/M2/M3/M4 Apple Silicon)
- Linux and Intel macOS are not currently supported.

**See setup below before running.**

## Setup

1. Download the latest release for your platform above.
2. Run the launch-together app. Make a shortcut to this file if you want; **this can basically replace your normal Slippi Dolphin shortcut!** More info below.
    - Windows: Unzip, run **`Slippi Dolphin with LylatLink.exe`**. The zip also includes `lylatlink.exe` if you only want the standalone tray app.
    - Mac: Unzip, move the apps into your `Applications` folder, then run **`Slippi Dolphin with LylatLink.app`**. The zip also includes `LylatLink.app` if you only want the standalone tray app.

      NOTE: I have no interest in paying $99/yr to Apple so you will probably get a security warning when running for the first time. [Click here for the steps to remediate this and trust the app.](docs/apple-security-fix.md), you may have to do this first for the LylatLink app, then the launcher app.
3. Play! By default, a chime will play when a call starts with your opponent.

**The launch-together app starts LylatLink and your existing Slippi Dolphin app together, and closes LylatLink when you close Dolphin.** If your opponent also has LylatLink, voice connects automatically. If not, nothing changes; LylatLink waits quietly and closes when Dolphin closes. On first connection, your OS/firewall may ask to allow network access. Allow it.

To use LylatLink, Slippi must be configured to save .slp replays. LylatLink will try to find your Slippi Dolphin install and replay folder automatically using your Slippi Launcher and Dolphin settings. If it does not, open the tray menu, choose **Replay Folder**, and select the folder that stores your .slp replays. ***Note:** If you organize replays into monthly subfolders, choose the parent folder; i.e., if they save to [path]/Slippi/2026-06, select the 'Slippi' folder.*

To enable replays or find your replay folder if LylatLink can't find it for some reason, refer to this settings page in the Slippi Launcher:

![Slippi Launcher replay folder setting](docs/images/slippi-replay-folder.png)

## Audio

LylatLink uses your system default microphone and speaker/headphones by default.

The tray menu includes input and output device pickers. Choosing **System Default** lets your operating system continue routing audio to the current default devices.

Default audio settings:

```toml
input_gain_db = 0.0
output_gain_db = -1.0
noise_gate_threshold_db = -45.0
```

You can choose output gain from the tray menu; it affects voice and connection sounds. The other values can be edited from the tray menu with **Edit Config File**.

## Notes

For debugging, Windows users can run `lylatlink.exe -console` from a terminal to show logs.

Developer notes live in [docs/DEV.md](docs/DEV.md).
