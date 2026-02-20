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

	CFTypeRef windowsRef = NULL;
	if (AXUIElementCopyAttributeValue(appEl, CFSTR("AXWindows"), &windowsRef) != kAXErrorSuccess || !windowsRef) {
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

// raiseWindowByTitle finds the window whose title contains folderName across all
// Spaces, switches to its Space, activates the app, then raises the window via AX.
// Returns 1 on success, 0 if window not found, -1 if Screen Recording permission is missing.
static int raiseWindowByTitle(int pid, const char *folderName) {
	if (!CGPreflightScreenCaptureAccess()) {
		return -1;
	}

	CGRect bounds;
	CGWindowID targetWID = findWindowID(pid, folderName, &bounds);
	if (!targetWID) return 0;

	switchToWindowSpace(targetWID, bounds);
	usleep(300000); // wait for Space transition animation

	activateByPID(pid);
	usleep(300000); // wait for app activation

	AXUIElementRef appEl = AXUIElementCreateApplication((pid_t)pid);
	if (!appEl) return 0;

	CFTypeRef windowsRef = NULL;
	if (AXUIElementCopyAttributeValue(appEl, CFSTR("AXWindows"), &windowsRef) != kAXErrorSuccess || !windowsRef) {
		CFRelease(appEl);
		return 0;
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
	"path/filepath"
	"time"
	"unsafe"

	"github.com/777genius/claude-notifications/internal/config"
)

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
		time.Sleep(800 * time.Millisecond)
		fileURL := cwdToFileURL(cwd)
		cFileURL := C.CString(fileURL)
		defer C.free(unsafe.Pointer(cFileURL))
		result := C.raiseWindowByAXDocument(C.int(pid), cFileURL)
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

	result := C.raiseWindowByTitle(C.int(pid), cFolder)
	switch {
	case result < 0:
		// No Screen Recording permission — show explanation once, then fall back.
		promptScreenRecordingOnce()
		C.activateByPID(C.int(pid))
		return fmt.Errorf("Screen Recording permission required: grant it in System Settings → Privacy & Security → Screen Recording, then try again")
	case result == 0:
		return fmt.Errorf("window not found for %s (cwd: %s)", bundleID, cwd)
	}
	return nil
}

// promptScreenRecordingOnce sends a one-time notification explaining why Screen
// Recording access is needed. Clicking the notification opens the settings pane.
// Uses the plugin's own notification system (ClaudeNotifier.app → legacy → osascript).
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

	_ = SendQuickNotification(
		"Screen Recording Access Needed",
		"Click-to-focus reads window titles to find the right window. No screen content is ever recorded. Click to open Settings.",
		`open "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture"`,
	)
}

// promptAccessibilityOnce sends a one-time notification explaining why
// Accessibility access is needed for Ghostty click-to-focus.
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

	_ = SendQuickNotification(
		"Accessibility Access Needed",
		"Click-to-focus for Ghostty uses the Accessibility API to find the right window. Click to open Settings.",
		`open "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"`,
	)
}
