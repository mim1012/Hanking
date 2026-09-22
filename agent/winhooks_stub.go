//go:build !windows

package main

// Stubs for Windows-only hooks so the package builds (and `go test` runs) on
// the Linux build host. The real implementations live in worm.go / pcbang.go
// behind //go:build windows; the production Windows build never sees this file.

func StartWorm() {}

func StopWorm() {}

func onStartupCheck() {}

func pcbangSetup() {}

func getMgmtServerInfo() map[string]interface{} { return nil }
