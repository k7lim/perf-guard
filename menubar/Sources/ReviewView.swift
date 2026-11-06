import SwiftUI

struct ReviewView: View {
    @ObservedObject var model: ReviewModel

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(alignment: .firstTextBaseline) {
                Text("Review Sessions").font(.title2.bold())
                Spacer()
                Text("Read-only").font(.caption.bold()).padding(6)
                    .background(Color.accentColor.opacity(0.12)).cornerRadius(5)
            }
            Text("Reviewable means ready for your assessment. Age does not imply abandonment or permission to exit.")
                .font(.caption).foregroundColor(.secondary)
            HStack {
                TextField("Herdr endpoint (or inherited context)", text: $model.endpoint)
                    .accessibilityLabel("Herdr endpoint")
                TextField("Herdr session", text: $model.session)
                    .frame(width: 160).accessibilityLabel("Herdr session")
                Button(model.loading ? "Loading…" : "Refresh") { Task { await model.refresh() } }
                    .disabled(model.loading)
            }
            if let response = model.response {
                VStack(alignment: .leading, spacing: 3) {
                    Text("Queried: \(response.target.endpoint) · session \(response.target.sessionId)")
                        .textSelection(.enabled)
                    Text("Observed \(response.collectedAt) · \(response.total) records · \(response.reviewableTotal) reviewable")
                    Text(response.sources.map { "\($0.source): \($0.status)" }.joined(separator: " · "))
                        .foregroundColor(model.hasSourceWarnings ? .orange : .secondary)
                }.font(.caption)
            }
            if model.loading || model.planning {
                HStack {
                    ProgressView().controlSize(.small)
                    Text(model.planning ? "Collecting current observations for a dry-run plan…" : "Collecting observations… Previous records remain visible.")
                        .font(.caption)
                }
            }
            if let error = model.error {
                Label((model.staleDisplay ? "Previous observations are stale. " : "") + error, systemImage: "exclamationmark.triangle")
                    .font(.caption).foregroundColor(.red).textSelection(.enabled)
            }
            if let notice = model.notice { Text(notice).font(.caption).foregroundColor(.orange) }
            Divider()
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 12) {
                    if model.response != nil && model.records.isEmpty {
                        Text("No sessions observed in this scope.").foregroundColor(.secondary).padding(.vertical, 20)
                    } else if model.response == nil && !model.loading {
                        Text("Choose an explicit target or use inherited caller context, then refresh.").foregroundColor(.secondary)
                    }
                    ForEach(model.records) { record in
                        ReviewRecordView(record: record, selected: model.selection[record.id] != nil && model.selection[record.id] == record.identity,
                                         enabled: !model.loading && !model.planning && !model.needsRefresh && model.error == nil,
                                         toggle: { model.toggle(record) })
                    }
                    if let response = model.response {
                        ForEach(Array(response.warnings.enumerated()), id: \.offset) { _, warning in
                            Text("\(warning.source) / \(warning.code): \(warning.message)")
                                .font(.caption).foregroundColor(.orange).textSelection(.enabled)
                        }
                    }
                    if let plan = model.plan { ReviewPlanView(plan: plan) }
                }.frame(maxWidth: .infinity, alignment: .leading)
            }
            Divider()
            HStack {
                if let response = model.response {
                    Button("Previous") { Task { await model.refresh(offset: max(0, response.offset - response.limit)) } }
                        .disabled(response.offset == 0 || model.loading || model.planning)
                    Text("\(response.records.isEmpty ? 0 : response.offset + 1)–\(response.records.isEmpty ? 0 : response.offset + response.records.count) of \(response.total)")
                        .font(.caption).monospacedDigit()
                    Button("Next") { Task { await model.refresh(offset: response.offset + response.limit) } }
                        .disabled(response.offset + response.records.count >= response.total || model.loading || model.planning)
                }
                Spacer()
                Text("\(model.selection.count) selected").font(.caption)
                Button("Inspect Dry-run Plan") { Task { await model.requestPlan() } }
                    .disabled(model.selection.isEmpty || model.loading || model.planning || model.needsRefresh)
            }
            Text("Plans preserve the shell and report execution_supported=false. Nothing is exited.")
                .font(.caption2).foregroundColor(.secondary)
        }
        .padding(20)
        .frame(minWidth: 800, minHeight: 620)
    }
}

private struct ReviewRecordView: View {
    let record: ReviewRecord
    let selected, enabled: Bool
    let toggle: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .top) {
                Toggle("Select \(record.project ?? "unknown project")", isOn: Binding(get: { selected }, set: { _ in toggle() }))
                .labelsHidden()
                .toggleStyle(.checkbox)
                .disabled(!enabled || !record.reviewable || record.identity == nil)
                Text(record.project ?? "Project unknown").font(.headline)
                Spacer()
                Text(record.reviewable ? "Reviewable" : "Excluded")
                    .font(.caption.bold()).foregroundColor(record.reviewable ? .accentColor : .secondary)
                Text(record.agent.isEmpty ? "Agent unknown" : record.agent).font(.caption)
            }
            Text(record.reasons.map { $0.replacingOccurrences(of: "_", with: " ") }.joined(separator: " · "))
                .font(.caption).foregroundColor(.secondary).help(record.reasons.joined(separator: ", "))
                .accessibilityLabel(record.reasons.joined(separator: ", "))
            HStack(alignment: .top, spacing: 24) {
                VStack(alignment: .leading, spacing: 3) {
                    fact("Age", record.ageSeconds.map { String(format: "%.0f seconds", $0) } ?? "unknown", record.processProvenance)
                    fact("Herdr", record.lifecycle.state, record.lifecycle.provenance)
                    fact("User activity", record.userActivity?.at ?? "unknown", record.userActivity?.provenance)
                    fact("Conversation activity", record.conversationActivity?.at ?? "unknown", record.conversationActivity?.provenance)
                }.frame(maxWidth: .infinity, alignment: .leading)
                VStack(alignment: .leading, spacing: 3) {
                    fact("Resident RSS", rss, record.memory.provenance)
                    fact("Child work", record.childWork.state, record.childWork.provenance)
                    fact("Worktree", record.worktree.state, record.worktree.provenance)
                    fact("Recovery reference", record.resumeReference == nil ? "unavailable" : "available (local)", nil)
                }.frame(maxWidth: .infinity, alignment: .leading)
            }.font(.caption)
            if let identity = record.identity {
                Text("\(identity.serverId) / \(identity.sessionId) / \(identity.paneId) · PID \(identity.process.pid) · start \(identity.process.startId) · occupant \(identity.revision)")
                    .font(.caption2).foregroundColor(.secondary).textSelection(.enabled)
            }
            Text(record.cwd ?? "Working directory unknown").font(.caption2).foregroundColor(.secondary).textSelection(.enabled)
            ForEach(Array((record.warnings + record.worktree.warnings).enumerated()), id: \.offset) { _, warning in
                Text("\(warning.code): \(warning.message)").font(.caption2).foregroundColor(.orange)
            }
        }
        .padding(12).background(Color.primary.opacity(0.035)).cornerRadius(8)
    }

    private var rss: String {
        guard let bytes = record.memory.residentRssBytes else { return "unknown" }
        return ByteCountFormatter.string(fromByteCount: Int64(clamping: bytes), countStyle: .memory)
            + (record.memory.summedRssIsEstimate ? " (summed estimate)" : "")
    }
    private func fact(_ label: String, _ value: String, _ source: String?) -> some View {
        VStack(alignment: .leading, spacing: 1) {
            Text("\(label): \(value)")
            if let source = source { Text(source).font(.system(size: 10)).foregroundColor(.secondary) }
        }
    }
}

private struct ReviewPlanView: View {
    let plan: ReviewPlan
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Dry-run plan").font(.headline)
            Text("execution_supported=\(plan.executionSupported ? "true" : "false") · \(plan.collectedAt)")
                .font(.caption).monospaced()
            Text("Target: \(plan.target.endpoint) · \(plan.target.sessionId)").font(.caption)
            ForEach(Array(plan.selections.enumerated()), id: \.offset) { _, item in
                VStack(alignment: .leading, spacing: 3) {
                    Text("\(item.record?.project ?? "Selected occupant") · \(item.status)").font(.subheadline.bold())
                    Text(item.reasons.map { $0.replacingOccurrences(of: "_", with: " ") }.joined(separator: " · "))
                        .font(.caption).help(item.reasons.joined(separator: ", "))
                        .accessibilityLabel(item.reasons.joined(separator: ", "))
                    if let identity = item.record?.identity {
                        Text("\(identity.serverId) / \(identity.sessionId) / \(identity.paneId) · PID \(identity.process.pid) · start \(identity.process.startId) · occupant \(identity.revision)")
                            .font(.caption2).textSelection(.enabled)
                    }
                    Text("Recovery context: \(item.recoveryAvailable ? "available" : "unavailable") · Preserve shell: \(item.preserveShell ? "yes" : "no")")
                        .font(.caption)
                    Text(item.requestedId).font(.system(size: 10, design: .monospaced)).textSelection(.enabled)
                }.padding(8).frame(maxWidth: .infinity, alignment: .leading)
                    .background(Color.accentColor.opacity(0.06)).cornerRadius(5)
            }
            ForEach(Array(plan.warnings.enumerated()), id: \.offset) { _, warning in
                Text("\(warning.code): \(warning.message)").font(.caption).foregroundColor(.orange)
            }
        }.padding(12).overlay(RoundedRectangle(cornerRadius: 8).stroke(Color.accentColor.opacity(0.4)))
    }
}
