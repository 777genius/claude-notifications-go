import AppKit
import Foundation

/// Notification service using the legacy NSUserNotificationCenter.
/// Does not require explicit notification permission, works on all macOS versions.
/// Used as primary sender when UNUserNotificationCenter permission is not available.
final class NSNotificationService: NotificationSending {

    func send(config: NotificationConfig, completion: @escaping (Result<Void, Error>) -> Void) {
        let notification = NSUserNotification()
        notification.title = config.title
        notification.informativeText = config.message
        notification.soundName = config.silent ? nil : NSUserNotificationDefaultSoundName

        if let subtitle = config.subtitle {
            notification.subtitle = subtitle
        }

        if let actionJSON = config.action.toJSON() {
            notification.userInfo = ["action": actionJSON]
        }

        // Use group as identifier for notification replacement
        if let group = config.group {
            notification.identifier = group
        }

        // Persistent notifications use alert style (stays until dismissed).
        // Non-persistent use banner style (auto-dismisses).
        notification.hasActionButton = config.persistent
        if config.persistent {
            notification.actionButtonTitle = "Open"
        }

        NSUserNotificationCenter.default.deliver(notification)
        completion(.success(()))
    }
}

/// Delegate for NSUserNotificationCenter to handle click callbacks
/// and ensure notifications are always displayed (even when app is frontmost).
final class NSNotificationDelegate: NSObject, NSUserNotificationCenterDelegate {

    private let actionExecutor: ActionExecuting

    init(actionExecutor: ActionExecuting = ActionExecutor()) {
        self.actionExecutor = actionExecutor
        super.init()
    }

    func userNotificationCenter(
        _ center: NSUserNotificationCenter,
        didActivate notification: NSUserNotification
    ) {
        if let actionJSON = notification.userInfo?["action"] as? String,
           let action = ClickAction.fromJSON(actionJSON) {
            actionExecutor.execute(action)
        }

        DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) {
            NSApplication.shared.terminate(nil)
        }
    }

    func userNotificationCenter(
        _ center: NSUserNotificationCenter,
        shouldPresent notification: NSUserNotification
    ) -> Bool {
        return true
    }
}
