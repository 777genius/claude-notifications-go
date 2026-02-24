//go:build darwin

package notifier

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework ApplicationServices -framework AppKit -framework CoreGraphics
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#import <CoreGraphics/CoreGraphics.h>

// Private CGS API declarations (stable, used by Moom/Magnet/Raycast et al.)
typedef int CGSConnectionID;
typedef uint64_t CGSSpaceID;
#define CGSAllSpacesMask 7
extern CGSConnectionID CGSMainConnectionID(void);
extern CFArrayRef CGSCopySpacesForWindows(CGSConnectionID cid, int selector, CFArrayRef windowIDs);
extern CGError CGSManagedDisplaySetCurrentSpace(CGSConnectionID cid, CFStringRef displayID, CGSSpaceID spaceID);
extern CFStringRef CGSCopyBestManagedDisplayForRect(CGSConnectionID cid, CGRect rect);
// Private AX SPI: resolves an AXUIElementRef to its CGWindowID.
// Available since macOS 10.9; used by Moom, Magnet, Amethyst, and others.
// Requires Accessibility permission (same as all AX calls).
extern AXError _AXUIElementGetWindow(AXUIElementRef elem, CGWindowID *idOut);

static int findPID(const char *bundleID) {
	@autoreleasepool {
		NSString *bid = [NSString stringWithUTF8String:bundleID];
		NSArray *apps = [NSRunningApplication runningApplicationsWithBundleIdentifier:bid];
		if (!apps || apps.count == 0) return -1;
		return (int)((NSRunningApplication *)apps[0]).processIdentifier;
	}
}

static void activateByPID(int pid) {
	@autoreleasepool {
		NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:(pid_t)pid];
		if (!app) return;
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		[app activateWithOptions:NSApplicationActivateIgnoringOtherApps];
#pragma clang diagnostic pop
	}
}

// titleMatchesFolder checks if a window title contains folderName as a
// distinct component. VS Code titles use " \u2014 " (em dash) as separator:
// "file.go \u2014 my-project \u2014 Visual Studio Code".
// First tries exact component match (split by " \u2014 "), then falls back
// to substring containsString for non-VS Code apps.
static BOOL titleMatchesFolder(NSString *title, NSString *folder) {
	// Try exact component match (VS Code / Electron-style titles)
	NSArray *components = [title componentsSeparatedByString:@" \u2014 "];
	for (NSString *comp in components) {
		NSString *trimmed = [comp stringByTrimmingCharactersInSet:
			[NSCharacterSet whitespaceCharacterSet]];
		if ([trimmed isEqualToString:folder]) return YES;
	}
	// Also try " - " (regular dash) for other apps
	components = [title componentsSeparatedByString:@" - "];
	for (NSString *comp in components) {
		NSString *trimmed = [comp stringByTrimmingCharactersInSet:
			[NSCharacterSet whitespaceCharacterSet]];
		if ([trimmed isEqualToString:folder]) return YES;
	}
	return NO;
}

// findWindowID returns the CGWindowID of the first window owned by pid whose
// title contains folderName as a distinct component, searching across all Spaces.
// Requires Screen Recording permission; caller must check CGPreflightScreenCaptureAccess first.
static CGWindowID findWindowID(int pid, const char *folderName, CGRect *outBounds) {
	@autoreleasepool {
		*outBounds = CGRectZero;
		CFArrayRef allInfo = CGWindowListCopyWindowInfo(
			kCGWindowListOptionAll | kCGWindowListExcludeDesktopElements,
			kCGNullWindowID
		);
		if (!allInfo) return 0;

		NSString *folder = [NSString stringWithUTF8String:folderName];
		CGWindowID targetWID = 0;

		for (NSDictionary *info in (__bridge NSArray *)allInfo) {
			NSNumber *pidNum = info[(__bridge NSString *)kCGWindowOwnerPID];
			if (!pidNum || pidNum.intValue != pid) continue;
			NSString *name = info[(__bridge NSString *)kCGWindowName];
			if (!name || !titleMatchesFolder(name, folder)) continue;
			NSNumber *wid = info[(__bridge NSString *)kCGWindowNumber];
			if (!wid) continue;
			targetWID = (CGWindowID)wid.unsignedIntValue;
			CFDictionaryRef boundsDict = (__bridge CFDictionaryRef)info[(__bridge NSString *)kCGWindowBounds];
			if (boundsDict) CGRectMakeWithDictionaryRepresentation(boundsDict, outBounds);
			break;
		}
		CFRelease(allInfo);
		return targetWID;
	}
}

// switchToWindowSpace switches the current visible Space to the one containing
// windowID, using bounds to select the correct display.
static void switchToWindowSpace(CGWindowID windowID, CGRect bounds) {
	@autoreleasepool {
		CGSConnectionID conn = CGSMainConnectionID();
		CFArrayRef spaces = CGSCopySpacesForWindows(conn, CGSAllSpacesMask,
			(__bridge CFArrayRef)@[@(windowID)]);
		if (!spaces) return;
		if (CFArrayGetCount(spaces) > 0) {
			CGSSpaceID spaceID = [(NSNumber *)CFArrayGetValueAtIndex(spaces, 0) unsignedLongLongValue];
			CFStringRef displayID = CGSCopyBestManagedDisplayForRect(conn, bounds);
			if (displayID) {
				CGSManagedDisplaySetCurrentSpace(conn, displayID, spaceID);
				CFRelease(displayID);
			}
		}
		CFRelease(spaces);
	}
}

static int hasScreenRecordingAccess(void) {
	return CGPreflightScreenCaptureAccess() ? 1 : 0;
}

static void requestScreenRecordingAccess(void) {
	CGRequestScreenCaptureAccess();
}

// raiseWindowByAXDocument enumerates AXWindows for the given PID and raises
// the first window whose AXDocument attribute exactly matches fileURL. Ghostty
// sets AXDocument to the shell CWD (via OSC 7) as a file:// URL.
// Returns 1 on match, 0 if not found, -1 if Accessibility permission is missing.
// NOTE: AXWindows only populates after the app has been activated; callers
// must call activateByPID and wait before calling this function.
static int raiseWindowByAXDocument(int pid, const char *fileURL) {
	if (!AXIsProcessTrusted()) {
		return -1;
	}

	AXUIElementRef appEl = AXUIElementCreateApplication((pid_t)pid);
	if (!appEl) return 0;

	// AXAllWindows returns windows across all Spaces; AXWindows is current-Space only.
	CFTypeRef windowsRef = NULL;
	if (AXUIElementCopyAttributeValue(appEl, CFSTR("AXAllWindows"), &windowsRef) != kAXErrorSuccess || !windowsRef) {
		CFRelease(appEl);
		return 0;
	}

	CFArrayRef windows = (CFArrayRef)windowsRef;
	CFIndex count = CFArrayGetCount(windows);
	int found = 0;

	for (CFIndex i = 0; i < count; i++) {
		AXUIElementRef w = (AXUIElementRef)CFArrayGetValueAtIndex(windows, i);
		CFTypeRef docRef = NULL;
		if (AXUIElementCopyAttributeValue(w, CFSTR("AXDocument"), &docRef) != kAXErrorSuccess) continue;

		CFIndex len = CFStringGetMaximumSizeForEncoding(
			CFStringGetLength((CFStringRef)docRef), kCFStringEncodingUTF8) + 1;
		char *buf = (char *)malloc(len);
		BOOL ok = buf && CFStringGetCString((CFStringRef)docRef, buf, len, kCFStringEncodingUTF8);
		CFRelease(docRef);

		if (ok && strcmp(buf, fileURL) == 0) {
			AXUIElementPerformAction(w, CFSTR("AXRaise"));
			AXUIElementSetAttributeValue(appEl, CFSTR("AXFrontmost"), kCFBooleanTrue);
			found = 1;
		}
		free(buf);
		if (found) break;
	}

	CFRelease(windowsRef);
	CFRelease(appEl);
	return found;
}

// findSwitchAndActivate locates a window by title across Spaces, switches to
// its Space and activates the app. The AX raise step is handled separately by
// raiseWindowByAXTitle so that Go can retry it with backoff.
// Returns 1 ok, 0 window not found, -1 no Screen Recording permission.
static int findSwitchAndActivate(int pid, const char *folderName) {
	if (!CGPreflightScreenCaptureAccess()) {
		return -1;
	}

	CGRect bounds;
	CGWindowID targetWID = findWindowID(pid, folderName, &bounds);
	if (!targetWID) return 0;

	switchToWindowSpace(targetWID, bounds);
	usleep(300000); // wait for Space transition animation

	activateByPID(pid);
	return 1;
}

// raiseWindowByAXTitle enumerates AXAllWindows for the given PID, switches to the
// matching window's Space (via _AXUIElementGetWindow + CGS), then raises it.
// Uses AXAllWindows (not AXWindows) to find windows across all Spaces.
// Space-switching uses only Accessibility permission — no Screen Recording needed.
// Returns 1 on match, 0 if not found, -1 if Accessibility permission is missing.
static int raiseWindowByAXTitle(int pid, const char *folderName) {
	if (!AXIsProcessTrusted()) {
		return -1;
	}

	AXUIElementRef appEl = AXUIElementCreateApplication((pid_t)pid);
	if (!appEl) return 0;

	// Try AXAllWindows first (returns windows across all Spaces).
	// Fall back to AXWindows if the app doesn't implement it.
	CFTypeRef windowsRef = NULL;
	AXError allErr = AXUIElementCopyAttributeValue(appEl, CFSTR("AXAllWindows"), &windowsRef);
	if (allErr != kAXErrorSuccess || !windowsRef) {
		allErr = AXUIElementCopyAttributeValue(appEl, CFSTR("AXWindows"), &windowsRef);
		if (allErr != kAXErrorSuccess || !windowsRef) {
			CFRelease(appEl);
			return 0;
		}
	}

	CFArrayRef windows = (CFArrayRef)windowsRef;
	CFIndex count = CFArrayGetCount(windows);
	int found = 0;

	NSString *folder = [NSString stringWithUTF8String:folderName];
	for (CFIndex i = 0; i < count; i++) {
		AXUIElementRef w = (AXUIElementRef)CFArrayGetValueAtIndex(windows, i);
		CFTypeRef titleRef = NULL;
		if (AXUIElementCopyAttributeValue(w, CFSTR("AXTitle"), &titleRef) != kAXErrorSuccess) continue;

		NSString *title = (__bridge NSString *)titleRef;
		BOOL matched = titleMatchesFolder(title, folder);
		CFRelease(titleRef);
		if (matched) {
			// Attempt Space-switching via _AXUIElementGetWindow (AX SPI) + CGS.
			// This path requires only Accessibility permission; no Screen Recording needed.
			// NOTE: AXPosition/AXSize may be stale for off-Space windows on multi-monitor
			// setups; CGSCopyBestManagedDisplayForRect may pick the wrong display in that
			// case, but the Space-switch still fires on the correct Space.
			CGWindowID wid = 0;
			if (_AXUIElementGetWindow(w, &wid) == kAXErrorSuccess && wid != 0) {
				// Get window bounds from AX position + size attributes.
				CGPoint pos = CGPointZero;
				CGSize sz = CGSizeZero;
				CFTypeRef posRef = NULL, sizeRef = NULL;
				if (AXUIElementCopyAttributeValue(w, CFSTR("AXPosition"), &posRef) == kAXErrorSuccess && posRef) {
					AXValueGetValue((AXValueRef)posRef, kAXValueCGPointType, &pos);
					CFRelease(posRef);
				}
				if (AXUIElementCopyAttributeValue(w, CFSTR("AXSize"), &sizeRef) == kAXErrorSuccess && sizeRef) {
					AXValueGetValue((AXValueRef)sizeRef, kAXValueCGSizeType, &sz);
					CFRelease(sizeRef);
				}
				CGRect bounds = CGRectMake(pos.x, pos.y, sz.width, sz.height);
				switchToWindowSpace(wid, bounds);
				usleep(300000); // wait for Space transition animation
			}
			AXUIElementPerformAction(w, CFSTR("AXRaise"));
			AXUIElementSetAttributeValue(appEl, CFSTR("AXFrontmost"), kCFBooleanTrue);
			found = 1;
			break;
		}
	}

	CFRelease(windowsRef);
	CFRelease(appEl);
	return found;
}
*/
import "C"

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
	"unsafe"

	"github.com/777genius/claude-notifications/internal/config"
)

// retryWindowFocus calls fn with increasing delays until a non-zero result.
// Returns 1 (found), -1 (no permission), or 0 (not found after all attempts).
// Worst case: 150+250+400 = 800ms. Best case: 150ms.
func retryWindowFocus(fn func() C.int) C.int {
	delays := []time.Duration{
		150 * time.Millisecond,
		250 * time.Millisecond,
		400 * time.Millisecond,
	}
	var result C.int
	for _, d := range delays {
		time.Sleep(d)
		result = fn()
		if result != 0 {
			break
		}
	}
	return result
}

// activateViaAppleScript sends a bare "activate" AppleScript to the app.
// Used as a last-resort when NSRunningApplication.activate may not work
// (e.g. no window-server context in certain subprocess scenarios).
// Does not enumerate or focus any specific window — just brings the app to front.
func activateViaAppleScript(bundleID string) {
	script := fmt.Sprintf(`tell application id "%s" to activate`, bundleID)
	_ = exec.Command("osascript", "-e", script).Run()
}

// FocusAppWindow raises the window matching cwd for the given bundleID app.
// For Ghostty: activates then matches via AXDocument (OSC 7 file:// URL).
// For other apps: uses CGS to find the window across Spaces then raises via AXTitle. macOS only.
func FocusAppWindow(bundleID, cwd string) error {
	cBundleID := C.CString(bundleID)
	defer C.free(unsafe.Pointer(cBundleID))

	pid := int(C.findPID(cBundleID))
	if pid < 0 {
		return fmt.Errorf("app not running: %s", bundleID)
	}

	if isGhosttyBundleID(bundleID) {
		if cwd == "" {
			return fmt.Errorf("invalid cwd: %s", cwd)
		}
		C.activateByPID(C.int(pid))
		fileURL := cwdToFileURL(cwd)
		cFileURL := C.CString(fileURL)
		defer C.free(unsafe.Pointer(cFileURL))
		result := retryWindowFocus(func() C.int {
			return C.raiseWindowByAXDocument(C.int(pid), cFileURL)
		})
		switch {
		case result < 0:
			promptAccessibilityOnce()
			return fmt.Errorf("Accessibility permission required: grant it in System Settings → Privacy & Security → Accessibility, then try again")
		case result == 0:
			return fmt.Errorf("window not found for %s (cwd: %s)", bundleID, cwd)
		}
		return nil
	}

	folderName := filepath.Base(cwd)
	if folderName == "" || folderName == "." || folderName == string(filepath.Separator) {
		return fmt.Errorf("invalid cwd: %s", cwd)
	}
	cFolder := C.CString(folderName)
	defer C.free(unsafe.Pointer(cFolder))

	prepResult := C.findSwitchAndActivate(C.int(pid), cFolder)
	if prepResult < 0 {
		// No Screen Recording permission: space-switching is unavailable.
		// Prompt once, then fall through to the AX path — Accessibility is
		// independent and can still raise the correct window without space-switching.
		promptScreenRecordingOnce()
		C.activateByPID(C.int(pid))
	} else if prepResult == 0 {
		// CGWindowListCopyWindowInfo did not find the window by title.
		// macOS 15+ no longer returns kCGWindowName for third-party app windows
		// via this API even with Screen Recording permission granted.
		// Fall back to activating by PID and letting AX-based title search handle it.
		C.activateByPID(C.int(pid))
	}
	result := retryWindowFocus(func() C.int {
		return C.raiseWindowByAXTitle(C.int(pid), cFolder)
	})
	switch {
	case result < 0:
		// No Accessibility permission: AX is unavailable, but activateByPID was
		// already called above. Use AppleScript as a safety net (works without
		// Accessibility) to ensure the app is visible, then surface the prompt.
		activateViaAppleScript(bundleID)
		promptAccessibilityOnce()
		return nil
	case result == 0:
		// Window not found — likely on a different Space. AXAllWindows does not cross
		// Space boundaries for Electron apps. The app is already activated above;
		// the user can switch to it manually.
		return nil
	}
	return nil
}

// promptScreenRecordingOnce sends a one-time notification explaining why Screen
// Recording access is needed for VS Code cross-Space focus.
// Clicking reveals ClaudeNotifier.app in Finder (for dragging into the list)
// and opens System Settings → Privacy & Security → Screen Recording.
func promptScreenRecordingOnce() {
	stableDir, err := config.GetStableConfigDir()
	if err != nil {
		return
	}
	markerPath := filepath.Join(stableDir, ".screen-recording-prompted")

	if _, err := os.Stat(markerPath); err == nil {
		return // already prompted
	}

	// Mark as prompted before sending (avoid duplicate prompts on error)
	_ = os.MkdirAll(stableDir, 0755)
	_ = os.WriteFile(markerPath, []byte("1"), 0644)

	executeCmd := `open "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture"`
	if exe, err := os.Executable(); err == nil {
		appPath := filepath.Join(filepath.Dir(exe), "ClaudeNotifier.app")
		if _, err := os.Stat(appPath); err == nil {
			// Copy the app path to the clipboard so the user can paste it into
			// Finder's Go To Folder dialog (⌘⇧G). Then open Settings.
			executeCmd = fmt.Sprintf(
				`printf %%s %q | pbcopy; open "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture"`,
				appPath,
			)
		}
	}

	_ = SendQuickNotification(
		"Screen Recording Access Needed",
		"Click to open Settings (path copied to clipboard). In Finder press ⌘⇧G, paste, then drag ClaudeNotifier.app into the Screen Recording list.",
		executeCmd,
	)
}

// promptAccessibilityOnce sends a one-time notification explaining why
// Accessibility access is needed for click-to-focus.
func promptAccessibilityOnce() {
	stableDir, err := config.GetStableConfigDir()
	if err != nil {
		return
	}
	markerPath := filepath.Join(stableDir, ".accessibility-prompted")

	if _, err := os.Stat(markerPath); err == nil {
		return // already prompted
	}

	// Mark as prompted before sending (avoid duplicate prompts on error)
	_ = os.MkdirAll(stableDir, 0755)
	_ = os.WriteFile(markerPath, []byte("1"), 0644)

	// Derive ClaudeNotifier.app path (same bin/ dir as this binary).
	// Clicking the notification reveals the app in Finder so the user can
	// drag it into the Accessibility list if it is not already there.
	executeCmd := `open "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"`
	if exe, err := os.Executable(); err == nil {
		appPath := filepath.Join(filepath.Dir(exe), "ClaudeNotifier.app")
		if _, err := os.Stat(appPath); err == nil {
			executeCmd = fmt.Sprintf(
				`printf %%s %q | pbcopy; open "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"`,
				appPath,
			)
		}
	}

	_ = SendQuickNotification(
		"Accessibility Access Needed",
		"Click to open Settings (path copied to clipboard). In Finder press ⌘⇧G, paste, then drag ClaudeNotifier.app into the Accessibility list.",
		executeCmd,
	)
}
