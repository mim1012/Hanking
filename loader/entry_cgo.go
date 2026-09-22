//go:build windows && cgo

package main

/*
#include <stdlib.h>
*/
import "C"

// EntryPoint is the rundll32 export used by WMI consumer + MSI CustomAction:
//
//	rundll32.exe DocViewerCore.dll,EntryPoint
//
//export EntryPoint
func EntryPoint(hwnd uintptr, hinst uintptr, cmdLine *C.char, nCmdShow int) {
	entryPointMain()
}

// Start is an alternate export for loaders that call by name after LoadLibrary.
//
//export Start
func Start() {
	entryPointMain()
}
