//go:build darwin

package hotkey

/*
#cgo LDFLAGS: -framework Carbon
#include <Carbon/Carbon.h>

OSStatus lylatlink_register_hotkey(UInt32 keyCode, UInt32 modifiers);
void lylatlink_unregister_hotkey(void);
*/
import "C"

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

var callbackMu sync.Mutex
var callback func()

func Start(ctx context.Context, key string, onPress func()) error {
	keyCode, err := darwinKeyCode(key)
	if err != nil {
		return err
	}

	callbackMu.Lock()
	callback = onPress
	callbackMu.Unlock()

	if status := C.lylatlink_register_hotkey(C.UInt32(keyCode), 0); status != 0 {
		callbackMu.Lock()
		callback = nil
		callbackMu.Unlock()
		return fmt.Errorf("RegisterEventHotKey failed: OSStatus %d", int(status))
	}

	go func() {
		<-ctx.Done()
		C.lylatlink_unregister_hotkey()
		callbackMu.Lock()
		callback = nil
		callbackMu.Unlock()
	}()
	return nil
}

//export lylatlinkHotkeyPressed
func lylatlinkHotkeyPressed() {
	callbackMu.Lock()
	cb := callback
	callbackMu.Unlock()
	if cb != nil {
		go cb()
	}
}

func darwinKeyCode(key string) (uint32, error) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "f1":
		return 0x7A, nil
	case "f2":
		return 0x78, nil
	case "f3":
		return 0x63, nil
	case "f4":
		return 0x76, nil
	case "f5":
		return 0x60, nil
	case "f6":
		return 0x61, nil
	case "f7":
		return 0x62, nil
	case "f8":
		return 0x64, nil
	case "f9":
		return 0x65, nil
	case "f10":
		return 0x6D, nil
	case "f11":
		return 0x67, nil
	case "f12":
		return 0x6F, nil
	default:
		return 0, fmt.Errorf("unsupported hotkey %q", key)
	}
}
