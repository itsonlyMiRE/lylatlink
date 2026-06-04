//go:build !darwin && !windows

package hotkey

import (
	"context"
	"fmt"
)

func Start(_ context.Context, key string, _ func()) error {
	return fmt.Errorf("global hotkey %q is not supported on this platform", key)
}
