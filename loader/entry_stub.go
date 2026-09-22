//go:build !cgo

package main

// Non-cgo builds (loader.exe) have no PE exports; WMI uses the DLL build.
