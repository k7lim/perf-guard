import SwiftUI

struct StatusView: View {
    @ObservedObject var model: LiveStatusModel
    var onReviewSessions: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Performance Guard").font(.headline)
            Text(model.monitorOnly ? "Monitoring and alerts only" : "Automatic process management")
                .font(.caption)
                .foregroundColor(.secondary)
            if let error = model.discoveryError {
                Text("Process discovery unavailable: \(error)").font(.caption).foregroundColor(.orange)
            }
            if model.schemaTooNew {
                HStack {
                    Image(systemName: "exclamationmark.triangle.fill")
                        .foregroundColor(.orange)
                    Text("Daemon is newer than this app. Please update.")
                        .font(.caption)
                }
                .padding(.bottom, 4)
            }

            if !model.daemonRunning {
                HStack {
                    Circle()
                        .fill(Color.gray)
                        .frame(width: 12, height: 12)
                    Text("Daemon not running")
                        .font(.headline)
                        .foregroundColor(.secondary)
                }
            } else {
                // Level
                HStack {
                    Circle()
                        .fill(levelColor)
                        .frame(width: 12, height: 12)
                    Text("Level: \(model.level.uppercased())")
                        .font(.headline)
                }

                // Compression rate
                HStack {
                    Text("Compression:")
                    Spacer()
                    Text("\(Int(model.compressionRate)) pages/s")
                        .monospacedDigit()
                }

                // Browser helpers
                HStack {
                    Text("Browser Helpers:")
                    Spacer()
                    Text("\(model.helperCount)")
                        .monospacedDigit()
                }

                Divider()

                // Top processes
                if !model.topProcesses.isEmpty {
                    Text("Top Processes:")
                        .font(.caption)
                        .foregroundColor(.secondary)
                    ForEach(model.topProcesses) { proc in
                        HStack {
                            Text(shortName(proc.name))
                                .lineLimit(1)
                                .truncationMode(.middle)
                            Spacer()
                            Text(formatBytes(proc.rssBytes))
                                .monospacedDigit()
                            Text(String(format: "%.0f%%", proc.cpuPct))
                                .monospacedDigit()
                                .frame(width: 40, alignment: .trailing)
                        }
                        .font(.system(size: 11))
                    }
                }

                Divider()

                // Spike info
                HStack {
                    Text("Last Spike:")
                    Spacer()
                    Text(relativeTime(model.lastSpikeTime))
                        .foregroundColor(.secondary)
                }
                .font(.caption)

                HStack {
                    Text("Snapshots:")
                    Spacer()
                    Text("\(model.forensicCount)")
                        .monospacedDigit()
                }
                .font(.caption)

                // VM and suspended
                HStack {
                    Text("VM Stopped:")
                    Spacer()
                    Text(model.vmStopped ? "Yes" : "No")
                }
                .font(.caption)

                HStack {
                    Text("Suspended:")
                    Spacer()
                    Text("\(model.suspended.count) processes")
                        .monospacedDigit()
                }
                .font(.caption)
            }

            Divider()

            Button(action: onReviewSessions) {
                Label("Review Sessions", systemImage: "list.bullet.rectangle")
                    .frame(maxWidth: .infinity)
            }
            .controlSize(.large)

            // Daemon toggle
            if !model.daemonRunning {
                Button(action: {
                    if model.daemonPid != nil {
                        model.stopDaemon()
                    } else {
                        model.startDaemon()
                    }
                }) {
                    HStack {
                        Image(systemName: model.daemonPid != nil ? "stop.circle.fill" : "play.circle.fill")
                        Text(model.daemonPid != nil ? "Stop Daemon" : "Start Daemon")
                    }
                    .frame(maxWidth: .infinity)
                }
                .controlSize(.large)
            }

            // Capture button — the main action
            Button(action: { model.captureSnapshot() }) {
                HStack {
                    Image(systemName: model.capturing ? "hourglass" : "camera.fill")
                    Text(model.capturing ? "Capturing..." : "Capture Snapshot")
                }
                .frame(maxWidth: .infinity)
            }
            .disabled(model.capturing)
            .controlSize(.large)
            .keyboardShortcut("s", modifiers: .command)

            if let result = model.lastCaptureResult {
                Text(result)
                    .font(.caption)
                    .foregroundColor(result.hasPrefix("Error") || result.hasPrefix("Capture failed") ? .red : .green)
            }

            Divider()

            Text("Quitting this menu keeps the background monitor running.")
                .font(.caption2).foregroundColor(.secondary)
            Button("Quit Menu") {
                NSApplication.shared.terminate(nil)
            }
            .buttonStyle(.plain)
            .foregroundColor(.secondary)
            .font(.caption)
        }
        .padding(12)
        .frame(width: 300)
    }

    private var levelColor: Color {
        switch model.level {
        case "normal": return .green
        case "warning": return .yellow
        case "high": return .orange
        case "critical": return .red
        default: return .gray
        }
    }

    private func shortName(_ name: String) -> String {
        // Extract just the executable name from full path
        let components = name.split(separator: "/")
        return String(components.last ?? Substring(name))
    }

    private func formatBytes(_ bytes: UInt64) -> String {
        let gb = Double(bytes) / (1024 * 1024 * 1024)
        if gb >= 1.0 {
            return String(format: "%.1f GB", gb)
        }
        let mb = Double(bytes) / (1024 * 1024)
        return String(format: "%.0f MB", mb)
    }

    private func relativeTime(_ isoString: String?) -> String {
        guard let str = isoString, !str.isEmpty else { return "never" }
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        guard let date = formatter.date(from: str) else { return "unknown" }
        let seconds = Int(Date().timeIntervalSince(date))
        if seconds < 60 { return "\(seconds)s ago" }
        if seconds < 3600 { return "\(seconds / 60) min ago" }
        if seconds < 86400 { return "\(seconds / 3600)h ago" }
        return "\(seconds / 86400)d ago"
    }
}
