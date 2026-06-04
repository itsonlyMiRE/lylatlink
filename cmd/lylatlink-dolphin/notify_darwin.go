//go:build darwin

package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func notify(title string, message string) {
	script := "display dialog " + appleScriptString(message) + " with title " + appleScriptString(title) + ` buttons {"OK"} default button "OK"`
	if err := exec.Command("osascript", "-e", script).Start(); err != nil {
		fmt.Printf("%s: %s\n", title, message)
	}
}

func appleScriptString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}
