//go:build windows

package console

import (
	"log"
	"os"

	"golang.org/x/sys/windows"
)

const attachParentProcess = ^uintptr(0)

var (
	kernel32          = windows.NewLazySystemDLL("kernel32.dll")
	procAttachConsole = kernel32.NewProc("AttachConsole")
	procAllocConsole  = kernel32.NewProc("AllocConsole")
)

func Enable() error {
	r, _, _ := procAttachConsole.Call(attachParentProcess)
	if r == 0 {
		procAllocConsole.Call()
	}

	if f, err := os.OpenFile("CONIN$", os.O_RDONLY, 0); err == nil {
		os.Stdin = f
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY|os.O_APPEND, 0); err == nil {
		os.Stdout = f
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY|os.O_APPEND, 0); err == nil {
		os.Stderr = f
		log.SetOutput(f)
	}
	return nil
}
