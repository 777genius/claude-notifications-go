//go:build windows

package notifier

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// maxAncestryDepth bounds the parent-process walk so a malformed or cyclic
// snapshot can never spin.
const maxAncestryDepth = 64

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")

	// Seams so tests can drive pidFocused without real Win32 calls.
	foregroundProcessPID = defaultForegroundProcessPID
	processSnapshot      = defaultProcessSnapshot
)

// terminalHasFocus reports whether the foreground window belongs to the terminal
// running Claude Code.
//
// GetForegroundWindow yields the focused window's owning process. The terminal
// emulator is an ancestor of this short-lived hook process, so we walk our own
// parent chain (via a Toolhelp process snapshot) and treat a match as "the
// terminal is focused". Detection is best-effort: multi-window terminal hosts
// that run several windows in one process cannot be disambiguated, and any API
// failure reports unknown (false) so the notification is still delivered.
func terminalHasFocus() bool {
	fgPID, ok := foregroundProcessPID()
	if !ok {
		return false
	}
	pidToPPID, err := processSnapshot()
	if err != nil {
		return false
	}
	return pidFocused(fgPID, uint32(os.Getpid()), pidToPPID)
}

// pidFocused reports whether fgPID is the current process or one of its
// ancestors. The walk is bounded by maxAncestryDepth and guards against cycles.
func pidFocused(fgPID, selfPID uint32, pidToPPID map[uint32]uint32) bool {
	if fgPID == 0 {
		return false
	}
	cur := selfPID
	seen := make(map[uint32]bool, maxAncestryDepth)
	for i := 0; i < maxAncestryDepth; i++ {
		if cur == 0 || seen[cur] {
			return false
		}
		if cur == fgPID {
			return true
		}
		seen[cur] = true
		parent, ok := pidToPPID[cur]
		if !ok {
			return false
		}
		cur = parent
	}
	return false
}

// defaultForegroundProcessPID returns the PID owning the foreground window.
func defaultForegroundProcessPID() (uint32, bool) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return 0, false
	}
	var pid uint32
	ret, _, _ := procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if ret == 0 || pid == 0 {
		return 0, false
	}
	return pid, true
}

// defaultProcessSnapshot builds a pid->parent-pid map from a Toolhelp snapshot.
func defaultProcessSnapshot() (map[uint32]uint32, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snap, &entry); err != nil {
		return nil, err
	}

	pidToPPID := make(map[uint32]uint32)
	for {
		pidToPPID[entry.ProcessID] = entry.ParentProcessID
		if err := windows.Process32Next(snap, &entry); err != nil {
			break // ERROR_NO_MORE_FILES terminates iteration
		}
	}
	return pidToPPID, nil
}
