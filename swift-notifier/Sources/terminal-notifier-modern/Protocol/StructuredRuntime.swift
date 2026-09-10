import AppKit
import Darwin
import UserNotifications

private enum ContinuousClock {
    static func now() -> Double {
        var info = mach_timebase_info_data_t()
        mach_timebase_info(&info)
        return Double(mach_continuous_time()) * Double(info.numer) / Double(info.denom) / 1_000_000_000
    }
    static func bootID() -> String? {
        var size = 0
        guard sysctlbyname("kern.bootsessionuuid", nil, &size, nil, 0) == 0, size > 1, size <= 256 else { return nil }
        var bytes = [CChar](repeating: 0, count: size)
        guard sysctlbyname("kern.bootsessionuuid", &bytes, &size, nil, 0) == 0 else { return nil }
        return String(cString: bytes)
    }
}

private final class StructuredUNBackend: NativeBackend {
    private let center = UNUserNotificationCenter.current()
    func permission(_ completion: @escaping (NativePermission) -> Void) {
        center.getNotificationSettings { settings in
            DispatchQueue.main.async {
                switch settings.authorizationStatus {
                case .authorized, .provisional: completion(.allowed)
                case .notDetermined: completion(.undetermined)
                default: completion(.denied)
                }
            }
        }
    }
    func add(_ request: NativeRequest, completion: @escaping (Bool) -> Void) {
        let content = UNMutableNotificationContent()
        content.title = request.title
        content.body = request.body
        content.subtitle = request.subtitle ?? ""
        content.sound = request.silent ? nil : .default
        // PR1 supports no navigation. No legacy action, shell, category or route.
        center.add(UNNotificationRequest(identifier: request.correlationID, content: content, trigger: nil)) { error in
            DispatchQueue.main.async { completion(error == nil) }
        }
    }
}

enum StructuredRuntime {
    static func capabilities(arguments: [String]) -> Never {
        guard arguments == ["--capabilities-json"],
              let data = try? JSONEncoder().encode(NativeCapabilities()) else { exit(1) }
        FileHandle.standardOutput.write(data)
        FileHandle.standardOutput.write(Data([10]))
        exit(0)
    }

    static func send(arguments: [String]) -> Never {
        // Fixed grammar: no data-bearing argument can select another mode.
        var paths: [String: String] = [:]
        var i = 0
        var sendCount = 0
        var launchCount = 0
        while i < arguments.count {
            switch arguments[i] {
            case "--send-json": sendCount += 1
            case "-launchedViaLaunchServices": launchCount += 1
            case "--request-file", "--receipt-file":
                let key = arguments[i]
                guard i + 1 < arguments.count, paths[key] == nil else { exit(1) }
                i += 1; paths[key] = arguments[i]
            default: exit(1)
            }
            i += 1
        }
        guard sendCount == 1, launchCount <= 1,
              let requestPath = paths["--request-file"], let receiptPath = paths["--receipt-file"],
              let files = try? OwnedRequest(requestPath: requestPath, receiptPath: receiptPath) else { exit(1) }
        func emit(_ receipt: NativeReceipt) -> Never {
            do { try files.write(receipt); exit(0) }
            catch { exit(1) } // no receipt is an unknown handoff, never a retry signal
        }
        let data: Data
        do { data = try files.consume() }
        catch { emit(NativeReceipt(correlationID: files.correlationID, nonce: files.nonce,
                                  status: .rejected, reason: .invalid_file)) }
        // Validate before initializing AppKit / UN (including bundle guard).
        let request: NativeRequest
        do {
            request = try NativeCodec.decodeRequest(data)
            guard request.correlationID == files.correlationID, request.nonce == files.nonce else {
                throw NativeReason.malformed_request
            }
        } catch {
            emit(NativeReceipt(correlationID: files.correlationID, nonce: files.nonce,
                               status: .rejected, reason: (error as? NativeReason) ?? .malformed_request))
        }
        guard launchCount == 1, Bundle.main.bundleIdentifier != nil,
              Bundle.main.bundleURL.pathExtension == "app", let boot = ContinuousClock.bootID() else {
            emit(NativeReceipt(correlationID: files.correlationID, nonce: files.nonce,
                               status: .rejected, reason: .unsupported_notifier))
        }
        let app = NSApplication.shared
        app.setActivationPolicy(.accessory)
        let delivery = StructuredDelivery(backend: StructuredUNBackend(), bootID: boot,
                                          now: ContinuousClock.now, emit: { emit($0) })
        // Short polling also catches suspend: continuous clock, not a fresh launch budget.
        let timer = Timer(timeInterval: 0.05, repeats: true) { _ in
            if ContinuousClock.now() >= request.notAfter {
                delivery.timeout()
            }
        }
        RunLoop.main.add(timer, forMode: .common)
        DispatchQueue.main.async { delivery.start(data: data, correlationID: files.correlationID, nonce: files.nonce) }
        withExtendedLifetime(delivery) { app.run() }
        exit(1)
    }
}
