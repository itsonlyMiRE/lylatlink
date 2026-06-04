//go:build windows

package hotkey

import (
	"context"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	modNoRepeat = 0x4000
	wmHotKey    = 0x0312
	wmQuit      = 0x0012
)

var (
	user32                 = syscall.NewLazyDLL("user32.dll")
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procRegisterHotKey     = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey   = user32.NewProc("UnregisterHotKey")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procPostThreadMessageW = user32.NewProc("PostThreadMessageW")
	procGetCurrentThreadID = kernel32.NewProc("GetCurrentThreadId")
)

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct {
		x int32
		y int32
	}
}

func Start(ctx context.Context, key string, onPress func()) error {
	vk, err := windowsVirtualKey(key)
	if err != nil {
		return err
	}

	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		threadID, _, _ := procGetCurrentThreadID.Call()
		const hotkeyID = 1
		ok, _, registerErr := procRegisterHotKey.Call(0, hotkeyID, modNoRepeat, uintptr(vk))
		if ok == 0 {
			ready <- fmt.Errorf("RegisterHotKey failed: %w", registerErr)
			return
		}
		defer procUnregisterHotKey.Call(0, hotkeyID)

		go func() {
			<-ctx.Done()
			procPostThreadMessageW.Call(threadID, wmQuit, 0, 0)
		}()

		ready <- nil

		var message msg
		for {
			ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
			if int32(ret) <= 0 {
				return
			}
			if message.message == wmHotKey && message.wParam == hotkeyID {
				onPress()
			}
		}
	}()
	return <-ready
}

func windowsVirtualKey(key string) (uint32, error) {
	switch NormalizeKey(key) {
	case "f1":
		return 0x70, nil
	case "f2":
		return 0x71, nil
	case "f3":
		return 0x72, nil
	case "f4":
		return 0x73, nil
	case "f5":
		return 0x74, nil
	case "f6":
		return 0x75, nil
	case "f7":
		return 0x76, nil
	case "f8":
		return 0x77, nil
	case "f9":
		return 0x78, nil
	case "f10":
		return 0x79, nil
	case "f11":
		return 0x7A, nil
	case "f12":
		return 0x7B, nil
	case "backtick":
		return 0xC0, nil
	case "backslash":
		return 0xDC, nil
	default:
		return 0, fmt.Errorf("unsupported hotkey %q", key)
	}
}
