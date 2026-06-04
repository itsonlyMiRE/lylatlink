//go:build !windows

package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func watchExitProcess(ctx context.Context, cancel context.CancelFunc, pid int) {
	log.Printf("will exit when process exits: pid=%d", pid)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !processRunning(pid) {
					log.Printf("watched process exited; shutting down: pid=%d", pid)
					cancel()
					return
				}
			}
		}
	}()
}

func processRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errors.Is(err, syscall.EPERM) {
		return true
	}
	return false
}

func watchExitProcessName(ctx context.Context, cancel context.CancelFunc, name string) {
	log.Printf("will exit when process exits: name=%q", name)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		seen := false
		startupDeadline := time.Now().Add(30 * time.Second)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				running := processNameRunning(name)
				if running {
					seen = true
					continue
				}
				if !seen && time.Now().Before(startupDeadline) {
					continue
				}
				log.Printf("watched process exited; shutting down: name=%q", name)
				cancel()
				return
			}
		}
	}()
}

func processNameRunning(name string) bool {
	return exec.Command("pgrep", "-x", name).Run() == nil
}
