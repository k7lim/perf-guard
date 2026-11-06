import Foundation
import Combine

// Wire consumer of contracts/review-v1. Unknown additive fields are ignored;
// policy/reason codes belong to the CLI, never to this presentation layer.
struct ReviewTarget: Codable, Equatable {
    let endpoint: String
    let sessionId: String
}
struct ReviewProcessIdentity: Codable, Equatable {
    let pid: Int
    let startId: String
}
struct ReviewIdentity: Codable, Equatable {
    let endpoint, serverId, sessionId, workspaceId, tabId, paneId, revision: String
    let process: ReviewProcessIdentity
}
struct ReviewWarning: Codable, Equatable {
    let code, source, message: String
}
struct ReviewSource: Codable, Equatable {
    let source, status: String
    let collectedAt: String?
    let provenance: String
}
struct ReviewMemory: Codable, Equatable {
    let residentRssBytes: UInt64?
    let summedRssIsEstimate: Bool
    let physicalFootprintBytes, compressedBytes, swapBytes: UInt64?
    let provenance: String
}
struct ReviewActivity: Codable, Equatable {
    let at, provenance: String
}
struct ReviewLifecycle: Codable, Equatable {
    let state: String
    let observedAt: String?
    let provenance: String
}
struct ReviewChildWork: Codable, Equatable {
    let state, provenance: String
    let observedAt: String?
}
struct ReviewWorktree: Codable, Equatable {
    let state: String
    let observedAt: String?
    let provenance: String
    let warnings: [ReviewWarning]
}
struct ReviewRecord: Codable, Equatable, Identifiable {
    let id: String
    let identity: ReviewIdentity?
    let process: ReviewProcessIdentity?
    let agent: String
    let project, cwd: String?
    let ageSeconds: Double?
    let processObservedAt: String?
    let processProvenance: String
    let memory: ReviewMemory
    let lifecycle: ReviewLifecycle
    let userActivity, conversationActivity: ReviewActivity?
    let childWork: ReviewChildWork
    let worktree: ReviewWorktree
    let resumeReference: String?
    let reviewable: Bool
    let reasons: [String]
    let warnings: [ReviewWarning]
}
struct ReviewResponse: Codable {
    let schemaVersion: Int
    let collectedAt: String
    let target: ReviewTarget
    let sources: [ReviewSource]
    let records: [ReviewRecord]
    let total, reviewableTotal, limit, offset: Int
    let warnings: [ReviewWarning]
}
struct ReviewPlanSelection: Codable {
    let requestedId, status: String
    let record: ReviewRecord?
    let reasons: [String]
    let recoveryAvailable, preserveShell: Bool
}
struct ReviewPlan: Codable {
    let schemaVersion: Int
    let collectedAt: String
    let target: ReviewTarget
    let executionSupported: Bool
    let selections: [ReviewPlanSelection]
    let sources: [ReviewSource]
    let warnings: [ReviewWarning]
}
private struct ReviewEnvelope<T: Decodable>: Decodable {
    let command: String
    let success: Bool
    let error: String?
    let data: T?
}
private struct ReviewEnvelopeHeader: Decodable {
    let command: String
    let success: Bool
    let error: String?
}
enum ReviewClientError: LocalizedError {
    case message(String)
    var errorDescription: String? {
        switch self { case .message(let text): return text }
    }
}
protocol ReviewClient {
    func run(arguments: [String]) async throws -> Data
}

// Starts only its own bounded CLI helper. No shell interpolation, daemon start,
// installation or Herdr control is available through this boundary.
struct CLIReviewClient: ReviewClient {
    let binaryPath: String?
    let timeout: TimeInterval
    let candidatePaths: [String]?

    init(binaryPath: String? = nil, timeout: TimeInterval = 8, candidatePaths: [String]? = nil) {
        self.binaryPath = binaryPath
        self.timeout = timeout
        self.candidatePaths = candidatePaths
    }

    static func binaryCandidates() -> [String] {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        let appDir = URL(fileURLWithPath: Bundle.main.bundlePath).deletingLastPathComponent().path
        return [
            home + "/Library/Application Support/perf-guard/bin/perf-guard",
            home + "/.local/bin/perf-guard", home + "/go/bin/perf-guard",
            "/opt/homebrew/bin/perf-guard", "/usr/local/bin/perf-guard",
            Bundle.main.bundlePath + "/Contents/MacOS/perf-guard", appDir + "/perf-guard"
        ].filter { FileManager.default.isExecutableFile(atPath: $0) }
    }

    func run(arguments: [String]) async throws -> Data {
        guard arguments.first == "review", arguments.dropFirst().first != "exit" else {
            throw ReviewClientError.message("Review client only supports read-only review commands.")
        }
        if let path = binaryPath { return try await runProcess(path: path, arguments: arguments) }
        // An older installed CLI may predate review. Its read-only describe
        // response decides whether to use it or the existing bundled fallback.
        let deadline = Date().addingTimeInterval(timeout)
        for path in candidatePaths ?? Self.binaryCandidates() {
            let remaining = deadline.timeIntervalSinceNow
            if remaining <= 0 { break }
            guard let schema = try? await runProcess(path: path, arguments: ["describe"], budget: min(1, remaining)),
                  let root = try? JSONSerialization.jsonObject(with: schema) as? [String: Any],
                  let commands = root["commands"] as? [String: Any], commands["review"] != nil else { continue }
            return try await runProcess(path: path, arguments: arguments, budget: max(0.1, deadline.timeIntervalSinceNow))
        }
        throw ReviewClientError.message("No available CLI supports review. Use a companion bundled with the M1 CLI.")
    }

    private func runProcess(path: String, arguments: [String], budget: TimeInterval? = nil) async throws -> Data {
        try await withCheckedThrowingContinuation { continuation in
            DispatchQueue.global(qos: .userInitiated).async {
                let task = Process()
                task.executableURL = URL(fileURLWithPath: path)
                task.arguments = arguments
                let output = Pipe()
                task.standardOutput = output
                task.standardError = FileHandle.nullDevice
                do {
                    try task.run()
                    let result = ReviewRunResult(continuation)
                    let expiry = DispatchWorkItem {
                        // This process was created above for this one read request.
                        if task.isRunning { task.terminate() }
                        result.finish(.failure(ReviewClientError.message("Review timed out. Refresh to retry.")))
                    }
                    DispatchQueue.global().asyncAfter(deadline: .now() + (budget ?? timeout), execute: expiry)
                    // Drain before waiting: a large JSON response cannot fill the pipe
                    // and deadlock the helper. This work never runs on the main actor.
                    let data = output.fileHandleForReading.readDataToEndOfFile()
                    task.waitUntilExit()
                    expiry.cancel()
                    if data.isEmpty {
                        result.finish(.failure(ReviewClientError.message("Review CLI returned no JSON (exit \(task.terminationStatus)).")))
                    } else {
                        result.finish(.success(data))
                    }
                } catch { continuation.resume(throwing: error) }
            }
        }
    }
}
private final class ReviewRunResult {
    private let lock = NSLock()
    private var continuation: CheckedContinuation<Data, Error>?
    init(_ continuation: CheckedContinuation<Data, Error>) { self.continuation = continuation }
    func finish(_ result: Result<Data, Error>) {
        lock.lock()
        let next = continuation
        continuation = nil
        lock.unlock()
        next?.resume(with: result)
    }
}

// UI state is main-actor isolated. Each request carries a generation; an older
// response cannot overwrite a refresh, selection change or target change.
@MainActor final class ReviewModel: ObservableObject {
    @Published private(set) var response: ReviewResponse?
    @Published private(set) var plan: ReviewPlan?
    @Published private(set) var selection: [String: ReviewIdentity] = [:]
    @Published private(set) var loading = false
    @Published private(set) var planning = false
    @Published private(set) var error: String?
    @Published private(set) var notice: String?
    @Published private(set) var needsRefresh = true
    @Published var endpoint = "" { didSet { targetEdited() } }
    @Published var session = "" { didSet { targetEdited() } }
    private let client: ReviewClient
    private var generation = 0
    private var queriedTarget: [String] = []
    let pageSize = 50

    init(client: ReviewClient = CLIReviewClient()) { self.client = client }

    var records: [ReviewRecord] { response?.records ?? [] }
    var staleDisplay: Bool { error != nil && response != nil }
    var hasSourceWarnings: Bool {
        response?.sources.contains { $0.status != "fresh" } ?? false
    }

    private var targetArguments: [String] {
        var result: [String] = []
        if !endpoint.isEmpty { result += ["--herdr-endpoint", endpoint] }
        if !session.isEmpty { result += ["--herdr-session", session] }
        return result
    }

    private func targetEdited() {
        generation += 1
        selection = [:]
        plan = nil
        loading = false
        planning = false
        needsRefresh = true
        notice = "Target changed. Refresh to query it; previous observations are shown below."
    }

    func toggle(_ record: ReviewRecord) {
        guard !loading && !planning && !needsRefresh && error == nil, record.reviewable, let identity = record.identity else { return }
        generation += 1
        plan = nil
        if selection[record.id] == identity { selection.removeValue(forKey: record.id) }
        else { selection[record.id] = identity }
        notice = nil
    }

    func refresh(offset: Int = 0) async {
        generation += 1
        let request = generation
        let target = targetArguments
        let samePage = response?.offset == offset && queriedTarget == target
        loading = true
        planning = false
        error = nil
        let hadPlan = plan != nil
        plan = nil
        do {
            let data = try await client.run(arguments: ["review", "--output", "json", "--limit", String(pageSize), "--offset", String(offset)] + target)
            let next: ReviewResponse = try await Task.detached { try Self.decode(data, command: "review") }.value
            guard next.schemaVersion == 1 else { throw ReviewClientError.message("Review schema is newer than this app. Update the menu companion.") }
            guard request == generation else { return }
            let oldCount = selection.count
            // Keep only exactly matching identities on this refreshed page. Page
            // navigation clears selection explicitly instead of guessing off-page.
            selection = samePage ? selection.filter { id, identity in
                next.records.contains { $0.id == id && $0.identity == identity && $0.reviewable }
            } : [:]
            notice = oldCount != selection.count ? "Selection invalidated by refresh or page/target change. Select the exact occupants again." : (hadPlan ? "Plan cleared after refresh. Request a new plan for current observations." : nil)
            response = next
            queriedTarget = target
            needsRefresh = false
            loading = false
        } catch {
            guard request == generation else { return }
            selection = [:]
            needsRefresh = true
            notice = "Selections and plan cleared because current observations are unavailable."
            self.error = error.localizedDescription
            loading = false
        }
    }

    func requestPlan() async {
        guard !selection.isEmpty, !loading, !planning, !needsRefresh else { return }
        generation += 1
        let request = generation
        planning = true
        error = nil
        plan = nil
        let ids = selection.keys.sorted()
        do {
            let data = try await client.run(arguments: ["review", "plan", "--output", "json"] + ids.flatMap { ["--session", $0] } + queriedTarget)
            let next: ReviewPlan = try await Task.detached { try Self.decode(data, command: "review plan") }.value
            guard next.schemaVersion == 1, !next.executionSupported else { throw ReviewClientError.message("Unsupported plan: this app accepts read-only v1 plans only.") }
            guard request == generation else { return }
            guard next.selections.map({ $0.requestedId }) == ids else {
                throw ReviewClientError.message("Plan response does not match the requested selections.")
            }
            guard next.selections.allSatisfy({ item in
                item.status != "reviewable" || (item.record?.id == item.requestedId && item.record?.identity == selection[item.requestedId])
            }) else {
                selection = [:]
                notice = "Plan identity changed. Select current occupants again."
                throw ReviewClientError.message("Plan returned a different occupant for a selected identity.")
            }
            plan = next
            for item in next.selections {
                if item.status != "reviewable" || item.record?.identity != selection[item.requestedId] {
                    selection.removeValue(forKey: item.requestedId)
                }
            }
            if selection.count != ids.count { notice = "One or more selected occupants changed or are excluded. Inspect each plan result." }
            planning = false
        } catch {
            guard request == generation else { return }
            self.error = error.localizedDescription
            planning = false
        }
    }

    nonisolated private static func decode<T: Decodable>(_ data: Data, command: String) throws -> T {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        let header = try decoder.decode(ReviewEnvelopeHeader.self, from: data)
        guard header.success else { throw ReviewClientError.message(header.error ?? "Review unavailable.") }
        let envelope = try decoder.decode(ReviewEnvelope<T>.self, from: data)
        guard envelope.command == command, let value = envelope.data else { throw ReviewClientError.message("Invalid review CLI response.") }
        return value
    }
}
