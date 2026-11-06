import Foundation
import AppKit
import SwiftUI

// Standalone, fixture-only test executable. It neither launches the menu app
// nor calls its existing daemon/capture/login-item features.
actor FixtureReviewClient: ReviewClient {
    struct Reply { let data: Data; var delay: UInt64 = 0 }
    var replies: [Reply]
    var calls: [[String]] = []
    init(_ replies: [Reply]) { self.replies = replies }
    func run(arguments: [String]) async throws -> Data {
        calls.append(arguments)
        guard !replies.isEmpty else { throw ReviewClientError.message("Fixture reply exhausted") }
        let reply = replies.removeFirst()
        if reply.delay > 0 { try await Task.sleep(nanoseconds: reply.delay) }
        return reply.data
    }
}

@main struct ReviewHarness {
    static let root = URL(fileURLWithPath: FileManager.default.currentDirectoryPath)
    static func payload(_ file: String) throws -> Data {
        try Data(contentsOf: root.appendingPathComponent("contracts/review-v1/fixtures/" + file + ".json"))
    }
    static func wrapped(_ file: String, plan: Bool = false) throws -> Data {
        let value = try JSONSerialization.jsonObject(with: payload(file))
        return try JSONSerialization.data(withJSONObject: ["command": plan ? "review plan" : "review", "success": true, "data": value, "timestamp": "2026-01-02T12:00:00Z"])
    }
    static func altered(_ file: String, change: (inout [String: Any]) -> Void) throws -> Data {
        var object = try JSONSerialization.jsonObject(with: payload(file)) as! [String: Any]
        change(&object)
        return try JSONSerialization.data(withJSONObject: ["command": "review", "success": true, "data": object])
    }
    static func oneSelectionPlan() throws -> Data {
        var object = try JSONSerialization.jsonObject(with: payload("example-plan")) as! [String: Any]
        object["selections"] = [(object["selections"] as! [[String: Any]])[0]]
        return try JSONSerialization.data(withJSONObject: ["command": "review plan", "success": true, "data": object])
    }
    static func changedPlan(_ change: (inout [String: Any]) -> Void) throws -> Data {
        var envelope = try JSONSerialization.jsonObject(with: oneSelectionPlan()) as! [String: Any]
        var object = envelope["data"] as! [String: Any]
        change(&object)
        envelope["data"] = object
        return try JSONSerialization.data(withJSONObject: envelope)
    }
    static func allExcluded() throws -> Data {
        try altered("healthy-review") { object in
            object["records"] = (object["records"] as! [[String: Any]]).filter { !($0["reviewable"] as! Bool) }
            object["total"] = 2; object["reviewable_total"] = 0
        }
    }
    static func require(_ condition: @autoclosure () -> Bool, _ message: String) throws {
        if !condition() { throw ReviewClientError.message("FAIL: " + message) }
    }

    @MainActor static func main() async {
        do {
            if CommandLine.arguments.count > 2 && CommandLine.arguments[1] == "--render" {
                let directory = URL(fileURLWithPath: CommandLine.arguments[2])
                try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
                try await renderFixtures(directory)
                print("PASS: sanitized SwiftUI fixture renders at \(directory.path)")
            } else if CommandLine.arguments.count > 2 && CommandLine.arguments[1] == "--parity" {
                try await cliParity(URL(fileURLWithPath: CommandLine.arguments[2]))
                print("PASS: actual fixture CLI / Swift records, reasons, selections and read-only plan parity")
            } else if CommandLine.arguments.count > 3 && CommandLine.arguments[1] == "--live" {
                // Acceptance-only, explicitly injected built CLI and target. Data
                // and optional rendering remain local to the caller's directory.
                let model = ReviewModel(client: CLIReviewClient(binaryPath: CommandLine.arguments[2]))
                model.endpoint = CommandLine.arguments[3]
                model.session = CommandLine.arguments.count > 4 ? CommandLine.arguments[4] : ""
                await model.refresh()
                try require(model.error == nil, model.error ?? "live review failed")
                // Display IDs for excluded process-only rows can sort before
                // reviewable identities. Exercise actual model pagination.
                var pages = 1
                while !model.records.contains(where: { $0.reviewable }), pages < 10,
                      let response = model.response,
                      response.offset + response.records.count < response.total {
                    await model.refresh(offset: response.offset + response.limit)
                    pages += 1
                    try require(model.error == nil, "live next page failed")
                }
                if let record = model.records.first(where: { $0.reviewable }) {
                    model.toggle(record)
                    await model.requestPlan()
                    try require(model.error == nil && model.plan?.executionSupported == false, "live plan unavailable")
                }
                if CommandLine.arguments.count > 5 {
                    let url = URL(fileURLWithPath: CommandLine.arguments[5])
                    try render(model, to: url)
                    // A live page can contain 50 rows, placing its plan below
                    // the initial viewport. Also capture the exact production
                    // plan component so acceptance inspects its actual content.
                    if model.plan != nil {
                        try render(model, to: url.deletingPathExtension().appendingPathExtension("plan.png"), bottom: true)
                    }
                }
                try require(model.plan != nil, "live inventory had no reviewable selection; real plan gate blocked")
                print("PASS: explicit CLI live review decoded; records=\(model.records.count), plan=\(model.plan == nil ? "no reviewable selection" : "read-only")")
            } else {
                try await tests()
                print("PASS: ReviewHarness model, boundary and async scenarios")
            }
        } catch {
            fputs(error.localizedDescription + "\n", stderr)
            exit(1)
        }
    }

    // Independent acceptance bridge: these envelopes come from the actual Go
    // command runner with its existing frozen-fixture provider injection, not a
    // second implementation of eligibility or plan policy.
    @MainActor static func cliParity(_ directory: URL) async throws {
        let list = try Data(contentsOf: directory.appendingPathComponent("review.json"))
        let plan = try Data(contentsOf: directory.appendingPathComponent("plan.json"))
        let client = FixtureReviewClient([.init(data: list), .init(data: plan)])
        let model = ReviewModel(client: client)
        await model.refresh()
        try require(model.error == nil, "CLI envelope decoded by Swift")
        let fixture = try JSONSerialization.jsonObject(with: payload("healthy-review")) as! [String: Any]
        let expected = fixture["records"] as! [[String: Any]]
        try require(model.records.map { $0.id } == expected.map { $0["id"] as! String }, "identical fixture record ordering and IDs")
        for (actual, frozen) in zip(model.records, expected) {
            try require(actual.reasons == frozen["reasons"] as! [String], "identical fixture reasons")
            try require(actual.reviewable == frozen["reviewable"] as! Bool, "identical fixture reviewability")
            try require(actual.lifecycle.state == (frozen["lifecycle"] as! [String: Any])["state"] as! String, "identical lifecycle")
            model.toggle(actual)
        }
        let ids = model.selection.keys.sorted()
        try require(ids.count == 2, "only two exact reviewable fixture occupants selected")
        await model.requestPlan()
        try require(model.error == nil && model.plan?.executionSupported == false, "CLI plan accepted without execution")
        try require(model.plan?.selections.map { $0.requestedId } == ids, "same CLI and Swift requested identities")
        for item in model.plan!.selections {
            let record = model.records.first { $0.id == item.requestedId }!
            try require(item.status == "reviewable" && item.record?.identity == record.identity && item.reasons == record.reasons && item.preserveShell && !item.recoveryAvailable, "same CLI and Swift selected records/reasons; absent resume stays unavailable")
        }
        let calls = await client.calls
        try require(calls[1] == ["review", "plan", "--output", "json"] + ids.flatMap { ["--session", $0] }, "exact multi-selection arguments sent to CLI")
    }

    @MainActor static func tests() async throws {
        let healthy = try wrapped("healthy-review")
        let client = FixtureReviewClient([.init(data: healthy), .init(data: healthy), .init(data: try wrapped("changed-occupant-review"))])
        let model = ReviewModel(client: client)
        await model.refresh()
        try require(model.error == nil && model.records.count == 4, "healthy fixture decoded")
        try require(model.selection.isEmpty, "age never selects automatically")
        let idle = model.records.first { $0.lifecycle.state == "idle" }!
        let done = model.records.first { $0.lifecycle.state == "done" }!
        model.toggle(idle); model.toggle(done)
        try require(model.selection.count == 2, "explicit multi-selection")
        model.toggle(model.records.first { $0.lifecycle.state == "working" }!)
        try require(model.selection.count == 2, "CLI exclusion disables selection")
        await model.refresh()
        try require(model.selection.count == 2, "matching exact identities survive refresh")
        await model.refresh()
        try require(model.selection.isEmpty && model.notice != nil, "PID reuse/occupant change visibly invalidates selection")

        let planClient = FixtureReviewClient([.init(data: healthy), .init(data: try oneSelectionPlan())])
        let planned = ReviewModel(client: planClient)
        planned.endpoint = "unix:///example/herdr.sock"; planned.session = "session-a"
        await planned.refresh()
        planned.toggle(planned.records.first { $0.lifecycle.state == "idle" }!)
        await planned.requestPlan()
        try require(planned.plan?.executionSupported == false && planned.plan?.selections.count == 1, "shared plan per-selection outcomes")
        let decoder = JSONDecoder(); decoder.keyDecodingStrategy = .convertFromSnakeCase
        let fullPlan = try decoder.decode(ReviewPlan.self, from: payload("example-plan"))
        try require(fullPlan.selections.map { $0.status } == ["reviewable", "missing", "duplicate", "excluded"], "all shared plan outcomes decoded")
        let calls = await planClient.calls
        try require(calls[0] == ["review", "--output", "json", "--limit", "50", "--offset", "0", "--herdr-endpoint", "unix:///example/herdr.sock", "--herdr-session", "session-a"], "frozen list flags")
        try require(calls[1] == ["review", "plan", "--output", "json", "--session", idle.id, "--herdr-endpoint", "unix:///example/herdr.sock", "--herdr-session", "session-a"], "frozen plan exact selection flags")
        try require(calls.allSatisfy { $0.first == "review" && !$0.contains("exit") && !$0.contains("install") && !$0.contains("daemon") }, "zero forbidden client commands")
        for replacement in ["stale-partial-review", "changed-occupant-review"] {
            let invalidated = ReviewModel(client: FixtureReviewClient([
                .init(data: healthy), .init(data: try oneSelectionPlan()), .init(data: try wrapped(replacement))
            ]))
            await invalidated.refresh()
            invalidated.toggle(invalidated.records.first { $0.lifecycle.state == "idle" }!)
            await invalidated.requestPlan()
            try require(invalidated.plan != nil && !invalidated.selection.isEmpty, "plan exists before replacement")
            await invalidated.refresh()
            try require(invalidated.selection.isEmpty && invalidated.plan == nil && invalidated.notice != nil, replacement + " invalidates existing selection and plan visibly")
        }
        planned.endpoint = "unix:///different.sock"
        try require(planned.selection.isEmpty && planned.plan == nil && planned.needsRefresh, "target change invalidates prior data selection")
        planned.toggle(idle)
        try require(planned.selection.isEmpty, "old target rows disabled until refreshed")
        for status in ["missing", "excluded"] {
            let data = try changedPlan { object in
                var item = (object["selections"] as! [[String: Any]])[0]
                item["status"] = status
                item["reasons"] = [status == "missing" ? "selection_missing_or_changed" : "working"]
                if status == "missing" { item["record"] = NSNull() }
                else {
                    var record = item["record"] as! [String: Any]
                    record["reviewable"] = false; record["reasons"] = ["working"]
                    item["record"] = record
                }
                object["selections"] = [item]
            }
            let current = ReviewModel(client: FixtureReviewClient([.init(data: healthy), .init(data: data)]))
            await current.refresh(); current.toggle(current.records.first { $0.id == idle.id }!); await current.requestPlan()
            try require(current.error == nil && current.plan?.selections[0].status == status && current.selection.isEmpty && current.notice != nil, "plan " + status + " invalidates selection visibly")
        }
        for mode in ["execution", "identity"] {
            let data = try changedPlan { object in
                if mode == "execution" { object["execution_supported"] = true }
                else {
                    var items = object["selections"] as! [[String: Any]]
                    var record = items[0]["record"] as! [String: Any]
                    var identity = record["identity"] as! [String: Any]
                    identity["revision"] = "replacement"
                    record["identity"] = identity; items[0]["record"] = record; object["selections"] = items
                }
            }
            let invalid = ReviewModel(client: FixtureReviewClient([.init(data: healthy), .init(data: data)]))
            await invalid.refresh(); invalid.toggle(invalid.records.first { $0.id == idle.id }!); await invalid.requestPlan()
            try require(invalid.error != nil && invalid.plan == nil, "unsupported plan rejected: " + mode)
        }

        for name in ["empty-review", "stale-partial-review", "unavailable-herdr-review", "duplicate-scoped-review", "dirty-review", "two-servers-review"] {
            let fixtureModel = ReviewModel(client: FixtureReviewClient([.init(data: try wrapped(name))]))
            await fixtureModel.refresh()
            try require(fixtureModel.error == nil, name + " decodes")
            if name == "empty-review" { try require(fixtureModel.records.isEmpty && fixtureModel.response?.total == 0, "empty remains zero") }
            if name == "stale-partial-review" { try require(fixtureModel.hasSourceWarnings && !fixtureModel.records[0].reviewable, "stale state/exclusion consumed") }
            if name == "unavailable-herdr-review" { try require(fixtureModel.records[0].cwd == nil && fixtureModel.records[0].worktree.state == "unknown", "unknown remains unknown") }
            if name == "dirty-review" { try require(fixtureModel.records[0].worktree.state == "dirty", "dirty state consumed") }
        }
        let excluded = try allExcluded()
        let excludedModel = ReviewModel(client: FixtureReviewClient([.init(data: excluded)]))
        await excludedModel.refresh()
        for record in excludedModel.records { excludedModel.toggle(record) }
        try require(excludedModel.selection.isEmpty && excludedModel.records.count == 2, "all-excluded fixture")

        let failure = Data(#"{"command":"review","success":false,"error":"fixture source unavailable","data":{"invalid":"must not decode as review"}}"#.utf8)
        let failed = ReviewModel(client: FixtureReviewClient([.init(data: healthy), .init(data: failure)]))
        await failed.refresh(); failed.toggle(failed.records[0]); await failed.refresh()
        try require(failed.error == "fixture source unavailable" && failed.staleDisplay && failed.selection.isEmpty, "failure envelope checked before data; old display marked stale")
        let tooNew = try altered("healthy-review") { $0["schema_version"] = 2 }
        let newer = ReviewModel(client: FixtureReviewClient([.init(data: tooNew)]))
        await newer.refresh()
        try require(newer.error?.contains("newer") == true, "new schema visibly incompatible")
        let malformed = ReviewModel(client: FixtureReviewClient([.init(data: Data("not json".utf8))]))
        await malformed.refresh()
        try require(malformed.error != nil, "malformed JSON visible")

        let delayed = ReviewModel(client: FixtureReviewClient([.init(data: healthy, delay: 150_000_000)]))
        let pending = Task { await delayed.refresh() }
        try await Task.sleep(nanoseconds: 20_000_000)
        try require(delayed.loading && delayed.response == nil, "delayed client does not block main actor")
        delayed.endpoint = "unix:///changed-during-request.sock"
        await pending.value
        try require(delayed.response == nil && delayed.needsRefresh, "late previous-target response ignored")
        try require(idle.ageSeconds == 120 && idle.userActivity == nil && idle.conversationActivity == nil && idle.memory.compressedBytes == nil, "age/activity/memory facts distinct")
        try await cliBoundary(healthy)
    }

    static func quote(_ value: String) -> String { "'" + value.replacingOccurrences(of: "'", with: "'\\''") + "'" }
    static func cliBoundary(_ healthy: Data) async throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent("perf-guard-ui-client-" + UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: directory) }
        let old = directory.appendingPathComponent("old-cli"), current = directory.appendingPathComponent("bundled-cli")
        let log = directory.appendingPathComponent("calls")
        let body = String(data: healthy, encoding: .utf8)!
        let record = "printf '%s\\n' \"$@\" >> " + quote(log.path) + "\n"
        try ("#!/bin/sh\n" + record + "printf '%s' '{\"commands\":{\"status\":{}}}'\n").write(to: old, atomically: true, encoding: .utf8)
        try ("#!/bin/sh\n" + record + "if [ \"$1\" = describe ]; then printf '%s' '{\"commands\":{\"review\":{}}}'; else printf '%s' " + quote(body) + "; fi\n").write(to: current, atomically: true, encoding: .utf8)
        for path in [old, current] { try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: path.path) }
        let client = CLIReviewClient(candidatePaths: [old.path, current.path])
        let result = try await client.run(arguments: ["review", "--output", "json"])
        try require(result == healthy, "older installed CLI falls back to capable bundled CLI")
        let calls = try String(contentsOf: log, encoding: .utf8)
        try require(calls == "describe\ndescribe\nreview\n--output\njson\n", "production runner only describe/review calls")
        do {
            _ = try await client.run(arguments: ["install"])
            throw ReviewClientError.message("FAIL: control command accepted")
        } catch {
            try require(error.localizedDescription.contains("read-only"), "production client denies control commands")
        }
    }

    @MainActor static func renderFixtures(_ directory: URL) async throws {
        for name in ["healthy-review", "empty-review", "stale-partial-review", "unavailable-herdr-review", "dirty-review", "changed-occupant-review"] {
            let model = ReviewModel(client: FixtureReviewClient([.init(data: try wrapped(name))]))
            await model.refresh()
            try render(model, to: directory.appendingPathComponent(name + ".png"))
        }
        let excluded = ReviewModel(client: FixtureReviewClient([.init(data: try allExcluded())]))
        await excluded.refresh()
        try render(excluded, to: directory.appendingPathComponent("all-excluded.png"))
        let failure = Data(#"{"command":"review","success":false,"error":"Fixture source unavailable","data":null}"#.utf8)
        let failed = ReviewModel(client: FixtureReviewClient([.init(data: try wrapped("healthy-review")), .init(data: failure)]))
        await failed.refresh(); await failed.refresh()
        try render(failed, to: directory.appendingPathComponent("error-stale.png"))
        let changed = ReviewModel(client: FixtureReviewClient([.init(data: try wrapped("healthy-review")), .init(data: try wrapped("changed-occupant-review"))]))
        await changed.refresh(); changed.toggle(changed.records[0]); await changed.refresh()
        try render(changed, to: directory.appendingPathComponent("selection-invalidated.png"))
        let model = ReviewModel(client: FixtureReviewClient([.init(data: try wrapped("healthy-review")), .init(data: try oneSelectionPlan())]))
        await model.refresh()
        model.toggle(model.records.first { $0.lifecycle.state == "idle" }!)
        await model.requestPlan()
        try render(model, to: directory.appendingPathComponent("plan.png"), height: 1500)
    }

    @MainActor static func render(_ model: ReviewModel, to url: URL, height: CGFloat = 1100, bottom: Bool = false) throws {
        _ = NSApplication.shared
        // AppKit-backed controls and ScrollView are not supported by
        // ImageRenderer. Render the real NSHostingView, including native fields.
        let host = NSHostingView(rootView: ReviewView(model: model).background(Color(NSColor.windowBackgroundColor)))
        let window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 940, height: height), styleMask: [.titled], backing: .buffered, defer: false)
        window.isReleasedWhenClosed = false
        window.contentView = host
        host.frame = NSRect(x: 0, y: 0, width: 940, height: height)
        host.layoutSubtreeIfNeeded()
        if bottom {
            func scrollView(_ view: NSView) -> NSScrollView? {
                if let scroll = view as? NSScrollView { return scroll }
                return view.subviews.lazy.compactMap { scrollView($0) }.first
            }
            guard let scroll = scrollView(host), let document = scroll.documentView else {
                throw ReviewClientError.message("Live plan scroll view unavailable")
            }
            // LazyVStack refines its estimated document height as rows become
            // visible. Settle layout after each scroll before finding the end.
            for _ in 0..<12 {
                scroll.contentView.scroll(to: NSPoint(x: 0, y: max(0, document.bounds.height - scroll.contentView.bounds.height)))
                scroll.reflectScrolledClipView(scroll.contentView)
                host.layoutSubtreeIfNeeded()
                RunLoop.current.run(until: Date().addingTimeInterval(0.02))
            }
        }
        window.display()
        guard let bitmap = host.bitmapImageRepForCachingDisplay(in: host.bounds) else {
            throw ReviewClientError.message("SwiftUI fixture render failed")
        }
        host.cacheDisplay(in: host.bounds, to: bitmap)
        guard let png = bitmap.representation(using: .png, properties: [:]) else { throw ReviewClientError.message("PNG render failed") }
        try png.write(to: url)
        window.close()
    }
}
