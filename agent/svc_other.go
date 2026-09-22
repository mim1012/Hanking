//go:build !windows

package main

// runAsService is a no-op on non-Windows builds (agent always runs interactively).
func runAsService() bool {
	return false
}
