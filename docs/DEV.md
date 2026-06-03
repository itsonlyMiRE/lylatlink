# LylatLink Developer Notes

LylatLink is a local companion app for Slippi replay files. It watches the user's replay directory, extracts live match identity from `.slp` files, and pairs the two players through a signaling server so a voice room can be created without knowing which connect code belongs to the local user.

This document tracks implementation and developer setup details.

## Current Shape

Implemented:

- Go client daemon parses minimal Slippi `Game Start` / `Game End` data.
- It watches replay folders for live `.slp` files, including both flat `Slippi/*.slp` and grouped `Slippi/YYYY-MM/*.slp` layouts. Startup scans only recently modified replays so stale files do not briefly look live, and missing `Game End` is handled with file-stability fallback.
- It can pair matches with either two unique connect codes or one deduped connect code, which allows self-match testing where both replay files expose the same Slippi connect code.
- It submits match start/end events to the signaling server.
- It uses an ephemeral per-launch `clientNonce` for duplicate suppression.
- Once paired, it opens a Pion WebRTC peer connection over `/signal`.
- It proves WebRTC connectivity with a data channel and audio RTP.
- It logs remote audio stats: packets/sec, RTP bitrate, payload bitrate, sequence gaps, RMS, and peak dBFS.
- It lists input devices, captures a selected/default microphone in standalone diagnostics mode, and logs RMS/peak dBFS.
- It sends live microphone frames through WebRTC as Opus by default, with PCMU as a fallback/test codec.
- It decodes incoming Opus/PCMU audio and plays it through the system default speaker.
- It has a system tray mode with status, match label, auto-join toggle, end call, input/output device choosers, replay folder picker, config-file opener, codec/playback display, and quit.
- The Node signaling server exposes `POST /match/start`, `POST /match/end`, and `WSS /signal`.
- Terraform infra can run the signaling server and coturn on one EC2-backed ECS host.

Not implemented yet:

- Global end-call hotkey.
- GitHub Release publishing.

## Run The Signaling Server

```bash
npm run server
```

This is the match-pairing and WebSocket signaling server. It is not a TURN server.

`POST /match/start` is a long-poll endpoint: the first player may wait until the peer submits the same match or the pending match expires. The tray client sends this request in the background and re-polls while the match is active, so replay watching and match-end handling continue while signaling is pending.

In the tray, this appears as `Waiting for other player` because the server may intentionally hold the request open instead of immediately returning a response.

Optional TURN env:

```bash
TURN_SECRET="shared-coturn-secret" \
TURN_URLS="turn:your-turn.example:3478,turn:your-turn.example:443?transport=tcp" \
npm run server
```

## Run The Client

Create or edit the LylatLink config file. The default path is resolved through XDG-style app config directories; on macOS this is usually `~/Library/Application Support/lylatlink/config.toml`, and on Windows it is under `%APPDATA%\lylatlink\config.toml`.

```toml
replay_dir = "/Users/you/Documents/Slippi"
auto_join = true
input_device_id = "" # optional; empty means system default microphone
output_device_id = "" # optional; empty means system default speaker/headphones
audio_codec = "opus" # opus or pcmu
input_gain_db = 0.0
output_gain_db = -1.5
noise_gate_threshold_db = -55.0
end_call_hotkey = "f8"
```

Then:

```bash
go run ./cmd/lylatlink
```

The default app mode is the system tray. Run in foreground console mode for debugging:

```bash
go run ./cmd/lylatlink -console
```

In tray mode, the Replay Folder item opens a native folder picker, and Edit Config File opens the active config file. Choosing a replay folder saves `replay_dir`, restarts the watcher, and works from a first-run "Not Ready" state.

Useful overrides:

```bash
go run ./cmd/lylatlink -replay-dir /path/to/Slippi -auto-join -signal-url http://127.0.0.1:8787
```

The production signaling server is compiled into the client. `-signal-url` is a development/testing override and is not persisted to `config.toml`.

Append these audio flags as needed:

```bash
-audio-codec pcmu     # force PCMU instead of Opus
-audio-output-device "<device-id-or-name>"
-synthetic-audio      # generated PCMU instead of microphone capture
-no-playback          # receive/log audio without speaker output
-ignore-match-end     # test mode: keep voice open after copied replay Game End
```

Opus uses the system `libopus` library through `pkg-config`. On macOS:

```bash
brew install opus pkg-config
```

Build an unsigned portable macOS app bundle:

```bash
scripts/build-macos.sh
```

The script writes `dist/macos/LylatLink.app` and `dist/lylatlink-macos-<arch>.zip`. The bundle includes `assets/icon.icns` and a copied `libopus.0.dylib`, so users do not need Homebrew for the packaged app. It also sets `LSUIElement=true` for menu-bar behavior and includes `NSMicrophoneUsageDescription` for macOS microphone permission prompts.

This app is not Developer ID signed or notarized. First launch may require right-clicking the app and choosing Open, or clearing quarantine:

```bash
xattr -dr com.apple.quarantine LylatLink.app
```

On Windows builds, install Go and MSYS2, then install the mingw64 cgo/Opus dependencies:

```powershell
C:\msys64\usr\bin\bash.exe -lc "pacman -S --needed --noconfirm mingw-w64-x86_64-gcc mingw-w64-x86_64-binutils mingw-w64-x86_64-pkgconf mingw-w64-x86_64-opus"
```

Build Windows release binaries from PowerShell on a Windows machine:

```powershell
.\scripts\build-windows.ps1
```

The script writes `dist\windows-amd64\lylatlink.exe` for default no-console tray use, `dist\windows-amd64\lylatlink-console.exe` for foreground diagnostics, and the needed Opus/mingw runtime DLLs beside the executables. The no-console app can also attach/open a console for diagnostic flags such as `-console`, `-list-audio-devices`, and `-audio-device-test`.

GitHub Actions also builds macOS and Windows artifacts. Run the `Build` workflow from the Actions tab, then download `lylatlink-macos` or `lylatlink-windows-amd64` from the workflow run artifacts.

No Windows installer is planned for the normal release path. The app is intended to run as a portable tray executable, with settings stored in AppData rather than beside the executable. An installer would only be useful later for optional conveniences such as Start Menu shortcuts, auto-update wiring, or a bundled Slippi launcher shortcut.

The Windows portable zip should contain `lylatlink.exe`, `lylatlink-console.exe`, and the required DLLs. User config persists separately under `%APPDATA%\lylatlink\config.toml`.

To test WebRTC pairing locally, run the signaling server and two clients with `auto_join=true`, then copy the same replay into both watched directories. Successful output includes:

```text
webrtc data-channel connecting
webrtc data channel open
webrtc data channel message
remote media track
remote audio playback started
audio stream stats
```

List input devices:

```bash
go run ./cmd/lylatlink -list-audio-devices
```

List output devices:

```bash
go run ./cmd/lylatlink -list-output-devices
```

Parse one replay:

```bash
go run ./cmd/lylatlink -parse-once /path/to/Game.slp
```

Test the default microphone:

```bash
./lylatlink -audio-device-test -audio-test-duration 10s
```

Test a specific microphone by ID or exact name from `-list-audio-devices`:

```bash
./lylatlink -audio-device-test -audio-input-device "<device-id>" -audio-test-duration 10s
```

Expected output:

```text
mic device opened: device="System Default" sampleRate=48000 captureChannels=1 captureFormat=2
mic stats: elapsed=1s frames=48000 samples=48000 frameRate=48000Hz bytes=96000 rms=-34.2 dBFS peak=-12.8 dBFS peakAmp=7500 lastFrameAge=1ms
```

## Tests

```bash
go test ./...
npm test
```

## Internet Connectivity

LylatLink uses WebRTC for the voice path. The signaling server only pairs players and relays WebRTC offer/answer/ICE messages; it does not carry the audio itself.

With no TURN credentials configured, peers can connect only when ICE can find a viable direct path. That often works on the same machine or same LAN, and may work across some NATs, but it is not reliable for two arbitrary players on different home networks.

For production internet use, run a TURN server such as coturn and configure the signaling server with `TURN_SECRET` and `TURN_URLS`. When TURN credentials are present, the Go client uses relay-only ICE, so clients connect out to TURN and do not need to open inbound ports on their router. Users may still see normal OS firewall prompts for the app's outbound network activity.

## Infra

`infra/` contains Terraform for one EC2-backed ECS host running two services: the Node signaling server and coturn. It also creates ECR repos, CloudWatch logs, security groups, an Elastic IP, and an SSM SecureString for the TURN shared secret.

Local Podman image builds are wired through Terraform `null_resource`s and push to ECR. Keep `infra/prod.tfvars`, state files, and environment files out of git.

For a Cloudflare-managed domain, leave Route 53 variables empty and set `signaling_hostname` / `turn_hostname` in `infra/prod.tfvars`. Create matching Cloudflare `A` records manually, pointing at the Terraform `ecs_public_ip` output. These records must be DNS-only, not proxied:

```text
lylatlink.signal.mire.systems -> <ecs_public_ip>
lylatlink.turn.mire.systems   -> <ecs_public_ip>
```

Route 53 record creation is still available for domains or subdomains delegated to AWS. For that path, set `hosted_zone_id` / `hosted_zone_name`; Terraform creates `signal.<zone>` and `turn.<zone>` records by default.
