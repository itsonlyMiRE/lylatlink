//go:build windows

package main

import "golang.org/x/sys/windows"

const (
	messageBoxOK        = 0x00000000
	messageBoxIconError = 0x00000010
)

func notify(title string, message string) {
	_, _ = windows.MessageBox(0, windows.StringToUTF16Ptr(message), windows.StringToUTF16Ptr(title), messageBoxOK|messageBoxIconError)
}
