import Foundation
import Combine

struct LiveStatus: Codable {
    let schemaVersion: Int
    let level: String
    let compressionRate: Double
    let helperCount: Int
    let topProcesses: [TopProcess]?
    let lastSpikeTime: String?
    let forensicCount: Int
    let suspended: [SuspendedBrief]?
    let vmStopped: Bool
    let updatedAt: String
}

struct TopProcess: Codable, Identifiable, Equatable {
    let pid: Int
    let name: String
    let rssBytes: UInt64
    let cpuPct: Double

    var id: Int { pid }
}

struct SuspendedBrief: Codable, Identifiable, Equatable {
    let pid: Int
    let name: String

    var id: Int { pid }
}

// Concurrency model (P14):
// Single writer: Go daemon writes live-status.json atomically via temp file + os.Rename (APFS guarantee).
// Single reader: This class polls every 3 seconds via Timer on the main run loop.
// No locking needed — the reader sees either the old file or the new file, never a partial write.
// Accepted race: display may be up to 3 seconds stale (one poll interval). Fine for a status indicator.
// Stale detection: if updatedAt is >60 seconds old, the daemon has likely stopped.
class LiveStatusModel: ObservableObject {
    @Published var level: String = "unknown"
    @Published var compressionRate: Double = 0
    @Published var helperCount: Int = 0
    @Published var topProcesses: [TopProcess] = []
    @Published var lastSpikeTime: String?
    @Published var forensicCount: Int = 0
    @Published var suspended: [SuspendedBrief] = []
    @Published var vmStopped: Bool = false
    @Published var updatedAt: String = ""
    @Published var isStale: Bool = true
    @Published var daemonRunning: Bool = false
    @Published var schemaTooNew: Bool = false
    @Published var capturing: Bool = false
    @Published var lastCaptureResult: String?
    @Published var daemonPid: Int?

    private var timer: Timer?
    private let filePath: String
    private let binaryPath: String
    private let maxKnownSchema = 1

    init() {
        let home = FileManager.default.homeDirectoryForCurrentUser
        filePath = home.appendingPathComponent(".config/perf-guard/live-status.json").path
        // Look for perf-guard binary in common locations
        // GUI apps don't inherit shell PATH, so check known locations explicitly.
        // The build.sh script also copies the binary next to the .app for portability.
        let appDir = Bundle.main.bundlePath.components(separatedBy: "/").dropLast().joined(separator: "/")
        let candidates = [
            appDir + "/perf-guard",                                          // next to .app (preferred)
            home.appendingPathComponent("go/bin/perf-guard").path,           // go install
            "/opt/homebrew/bin/perf-guard",                                  // homebrew
            "/usr/local/bin/perf-guard",                                     // manual install
        ]
        binaryPath = candidates.first { FileManager.default.isExecutableFile(atPath: $0) } ?? "perf-guard"
        startPolling()
    }

    func captureSnapshot() {
        guard !capturing else { return }
        capturing = true
        lastCaptureResult = nil

        DispatchQueue.global(qos: .userInitiated).async { [weak self] in
            guard let self = self else { return }
            let task = Process()
            task.executableURL = URL(fileURLWithPath: self.binaryPath)
            task.arguments = ["forensics", "capture", "--output", "json"]

            let pipe = Pipe()
            task.standardOutput = pipe
            task.standardError = pipe

            do {
                try task.run()
                task.waitUntilExit()
                let data = pipe.fileHandleForReading.readDataToEndOfFile()
                let output = String(data: data, encoding: .utf8) ?? ""

                DispatchQueue.main.async {
                    self.capturing = false
                    if task.terminationStatus == 0 {
                        self.lastCaptureResult = "Snapshot captured"
                        self.load() // refresh to pick up new forensic count
                    } else {
                        self.lastCaptureResult = "Capture failed: \(output.prefix(100))"
                    }
                    // Clear the result message after 5 seconds
                    DispatchQueue.main.asyncAfter(deadline: .now() + 5) {
                        self.lastCaptureResult = nil
                    }
                }
            } catch {
                DispatchQueue.main.async {
                    self.capturing = false
                    self.lastCaptureResult = "Error: \(error.localizedDescription)"
                }
            }
        }
    }

    func startDaemon() {
        guard daemonPid == nil else { return }
        DispatchQueue.global(qos: .utility).async { [weak self] in
            guard let self = self else { return }
            let task = Process()
            task.executableURL = URL(fileURLWithPath: self.binaryPath)
            task.arguments = ["daemon"]
            task.standardOutput = FileHandle.nullDevice
            task.standardError = FileHandle.nullDevice

            do {
                try task.run()
                let pid = Int(task.processIdentifier)
                DispatchQueue.main.async { self.daemonPid = pid }

                // Wait in background — when it exits, clear the pid
                task.waitUntilExit()
                DispatchQueue.main.async { self.daemonPid = nil }
            } catch {
                DispatchQueue.main.async { self.daemonPid = nil }
            }
        }
    }

    func stopDaemon() {
        guard let pid = daemonPid else { return }
        kill(Int32(pid), SIGTERM)
        // daemonPid will be cleared by the waitUntilExit handler above
    }

    private func startPolling() {
        timer = Timer.scheduledTimer(withTimeInterval: 3.0, repeats: true) { [weak self] _ in
            self?.load()
        }
        load()
    }

    private func load() {
        guard let data = FileManager.default.contents(atPath: filePath) else {
            DispatchQueue.main.async {
                self.daemonRunning = false
                self.isStale = true
                self.level = "unknown"
            }
            return
        }

        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase

        guard let status = try? decoder.decode(LiveStatus.self, from: data) else {
            DispatchQueue.main.async {
                self.daemonRunning = false
                self.isStale = true
            }
            return
        }

        let stale = isUpdatedAtStale(status.updatedAt)
        let tooNew = status.schemaVersion > maxKnownSchema

        DispatchQueue.main.async {
            if self.level != status.level { self.level = status.level }
            if self.compressionRate != status.compressionRate { self.compressionRate = status.compressionRate }
            if self.helperCount != status.helperCount { self.helperCount = status.helperCount }
            let procs = status.topProcesses ?? []
            if self.topProcesses != procs { self.topProcesses = procs }
            if self.lastSpikeTime != status.lastSpikeTime { self.lastSpikeTime = status.lastSpikeTime }
            if self.forensicCount != status.forensicCount { self.forensicCount = status.forensicCount }
            let susp = status.suspended ?? []
            if self.suspended != susp { self.suspended = susp }
            if self.vmStopped != status.vmStopped { self.vmStopped = status.vmStopped }
            if self.updatedAt != status.updatedAt { self.updatedAt = status.updatedAt }
            self.isStale = stale
            self.daemonRunning = !stale
            self.schemaTooNew = tooNew
        }
    }

    private func isUpdatedAtStale(_ updatedAt: String) -> Bool {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        guard let date = formatter.date(from: updatedAt) else { return true }
        return Date().timeIntervalSince(date) > 60
    }

    deinit {
        timer?.invalidate()
    }
}
