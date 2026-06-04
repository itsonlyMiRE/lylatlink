package assets

import _ "embed"

//go:embed icon.ico
var IconICO []byte

//go:embed icon.png
var IconPNG []byte

//go:embed macos/tray-template-32.png
var MacOSTrayTemplate32PNG []byte

//go:embed start.wav
var StartWAV []byte

//go:embed end.wav
var EndWAV []byte
