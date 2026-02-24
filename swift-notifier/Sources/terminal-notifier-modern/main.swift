import AppKit
import ApplicationServices
import Foundation
import UserNotifications

let arguments = Array(CommandLine.arguments.dropFirst())

if arguments.contains("-help") || arguments.contains("--help") {
    print("Usage: terminal-notifier-modern -title <title> -message <message> [options]")
    print("")
    print("  -title          Notification title (required)")
    print("  -message        Notification body (required)")
    print("  -subtitle       Notification subtitle (e.g. branch and folder)")
    print("  -activate       Bundle ID of app to activate on click")
    print("  -execute        Shell command to run on click")
    print("  -group          Group ID (replaces notifications with same group)")
    print("  -threadID       Thread ID for grouping notifications in a stack")
    print("  -timeSensitive  Mark as time-sensitive (breaks through Focus Mode)")
    print("  -nosound        Suppress notification sound")
    print("  -persistent     Show as alert (stays until dismissed) instead of banner")
    exit(ExitCode.success)
} else if ArgumentParser.isSendMode(arguments) {
    runSendMode(arguments: arguments)
} else {
    runCallbackMode()
}

// MARK: - Send Mode

func runSendMode(arguments: [String]) {
    let config: NotificationConfig
    do {
        config = try ArgumentParser.parse(arguments)
    } catch {
        fputs("Error: \(error)\n", stderr)
        exit(ExitCode.invalidArgs)
    }

    // Without a valid app bundle, UNUserNotificationCenter.current() crashes
    // with "bundleProxyForCurrentProcess is nil". Use osascript directly.
    guard Bundle.main.bundleIdentifier != nil else {
        OsascriptNotificationService().send(config: config) { result in
            switch result {
            case .success:
                exit(ExitCode.success)
            case .failure(let error):
                fputs("Error: \(error)\n", stderr)
                exit(ExitCode.failed)
            }
        }
        return
    }

    let app = NSApplication.shared
    app.setActivationPolicy(.accessory)

    // Safety timeout on a background queue — fires even if main queue is blocked.
    // UNUserNotificationCenter may hang when the .app is launched from CLI
    // (not via LaunchServices), especially on macOS Sequoia.
    // Falls back to NSUserNotificationCenter which works without permission.
    DispatchQueue.global().asyncAfter(deadline: .now() + 3.0) {
        fputs("Warning: UNUserNotificationCenter timed out, using NSNotificationService fallback\n", stderr)
        NSNotificationService().send(config: config) { _ in
            exit(ExitCode.success)
        }
    }

    // Schedule all async work on the main queue — no data races.
    // UNUserNotificationCenter requires the run loop to be active,
    // so register() and all send logic must run AFTER app.run() starts.
    DispatchQueue.main.async {
        NotificationCategory.register()
        checkAuthAndSend(config: config)
    }

    // Run event loop (processes main queue dispatches + RunLoop sources)
    app.run()
}

func checkAuthAndSend(config: NotificationConfig) {
    UNUserNotificationCenter.current().getNotificationSettings { settings in
        DispatchQueue.main.async {
            handleAuthStatus(settings.authorizationStatus, config: config)
        }
    }
}

func handleAuthStatus(_ status: UNAuthorizationStatus, config: NotificationConfig) {
    if status == .notDetermined {
        // Request authorization — prompts user on first launch
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge]) { granted, _ in
            DispatchQueue.main.async {
                let newStatus: UNAuthorizationStatus = granted ? .authorized : .denied
                sendNotification(config: config, authStatus: newStatus)
            }
        }
    } else {
        sendNotification(config: config, authStatus: status)
    }
}

func sendNotification(config: NotificationConfig, authStatus: UNAuthorizationStatus) {
    let service: NotificationSending
    if authStatus == .authorized || authStatus == .provisional {
        service = UNNotificationService()
    } else {
        // Fall back to NSUserNotificationCenter (deprecated but works without permission).
        // Avoids osascript which attributes notifications to Script Editor, causing
        // Script Editor to activate when the user clicks the notification.
        service = NSNotificationService()
    }

    service.send(config: config) { result in
        DispatchQueue.main.async {
            switch result {
            case .success:
                // Small delay for delivery, then exit
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.3) {
                    exit(ExitCode.success)
                }
            case .failure(let error):
                fputs("Error: \(error)\n", stderr)
                exit(ExitCode.failed)
            }
        }
    }
}

// MARK: - Callback Mode

func runCallbackMode() {
    // Request Accessibility access for this app (ClaudeNotifier.app) so child
    // processes spawned by executeCommand are covered by this app's TCC trust.
    // If not already granted, macOS shows the native "wants to control your
    // computer" dialog, which takes the user directly to the right Settings pane.
    let axOptions = [kAXTrustedCheckOptionPrompt.takeUnretainedValue(): kCFBooleanTrue] as CFDictionary
    AXIsProcessTrustedWithOptions(axOptions)

    // Request Screen Recording access for this app (ClaudeNotifier.app) so child
    // processes spawned by executeCommand are covered by this app's TCC trust.
    // Required for CGWindowListCopyWindowInfo to return window names across Spaces.
    CGRequestScreenCaptureAccess()

    // Request authorization when launched via LaunchServices (no args)
    UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge]) { _, _ in }

    let app = NSApplication.shared
    app.setActivationPolicy(.accessory)

    let appDelegate = AppDelegate()
    app.delegate = appDelegate
    UNUserNotificationCenter.current().delegate = appDelegate

    // Wire up NSUserNotificationCenter delegate for NSNotificationService callbacks
    let nsNotifDelegate = NSNotificationDelegate()
    NSUserNotificationCenter.default.delegate = nsNotifDelegate

    DispatchQueue.main.asyncAfter(deadline: .now() + 10) {
        NSApplication.shared.terminate(nil)
    }

    withExtendedLifetime((appDelegate, nsNotifDelegate)) {
        app.run()
    }
}
