//go:build !windows

package main

import (
	"fmt"
	"os"
)

// selfDestruct on non-Windows builds: no persistence installed, just exit.
func selfDestruct(reason string) {
	fmt.Fprintf(os.Stderr, "selfdestruct: non-windows exit (%s)\n", reason)
	os.Exit(0)
}
