//go:build windows

package main

import (
	"context"
	"log"
	"time"

	"golang.org/x/sys/windows"
)

func watchExitProcess(ctx context.Context, cancel context.CancelFunc, pid int) {
	log.Printf("will exit when process exits: pid=%d", pid)
	go func() {
		handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
		if err != nil {
			log.Printf("watched process unavailable; shutting down: pid=%d err=%v", pid, err)
			cancel()
			return
		}
		defer windows.CloseHandle(handle)

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				status, err := windows.WaitForSingleObject(handle, 0)
				if err != nil || status == windows.WAIT_OBJECT_0 {
					log.Printf("watched process exited; shutting down: pid=%d", pid)
					cancel()
					return
				}
			}
		}
	}()
}

func watchExitProcessName(ctx context.Context, cancel context.CancelFunc, name string) {
	log.Printf("process-name exit watch is not supported on Windows; name=%q", name)
}
