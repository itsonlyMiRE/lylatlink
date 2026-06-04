//go:build !windows

package main

import "fmt"

func notify(title string, message string) {
	fmt.Printf("%s: %s\n", title, message)
}
