import Foundation
import Combine

struct LiveStatus: Codable {
    let schemaVersion: Int
    let level: String
    let compressionRate: Double
    let helperCount: Int
    let topProcesses: [TopProcess]
    let lastSpikeTime: String?
    let forensicCount: Int
    let suspended: [SuspendedBrief]
    let vmStopped: Bool
    let updatedAt: String
}

struct TopProcess: Codable, Identifiable {
    let pid: Int
    let name: String
    let rssBytes: UInt64
    let cpuPct: Double

    var id: Int { pid }
}

struct SuspendedBrief: Codable, Identifiable {
    let pid: Int
    let name: String

    var id: Int { pid }
}

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

    private var timer: Timer?
    private let filePath: String
    private let maxKnownSchema = 1

    init() {
        let home = FileManager.default.homeDirectoryForCurrentUser
        filePath = home.appendingPathComponent(".config/perf-guard/live-status.json").path
        startPolling()
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
            self.topProcesses = status.topProcesses
            if self.lastSpikeTime != status.lastSpikeTime { self.lastSpikeTime = status.lastSpikeTime }
            if self.forensicCount != status.forensicCount { self.forensicCount = status.forensicCount }
            self.suspended = status.suspended
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
