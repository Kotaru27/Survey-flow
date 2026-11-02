//go:build !windows
package main
func prepareConsole() bool { return false }
func consoleWidth() int { return 80 }
