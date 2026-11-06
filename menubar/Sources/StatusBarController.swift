import AppKit
import SwiftUI

@MainActor class StatusBarController: NSObject {
    private var statusItem: NSStatusItem
    private var popover: NSPopover
    private let model = LiveStatusModel()
    private var observation: NSKeyValueObservation?
    private var reviewWindow: NSWindow?

    override init() {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        popover = NSPopover()
        super.init()

        popover.contentSize = NSSize(width: 320, height: 540)
        popover.behavior = .transient
        popover.contentViewController = NSHostingController(rootView: StatusView(model: model, onReviewSessions: { [weak self] in self?.showReviewSessions() }))

        updateIcon()

        if let button = statusItem.button {
            button.action = #selector(togglePopover(_:))
            button.target = self
        }

        // Observe level changes to update icon
        Timer.scheduledTimer(withTimeInterval: 3.0, repeats: true) { [weak self] _ in
            DispatchQueue.main.async {
                self?.updateIcon()
            }
        }
    }

    private func showReviewSessions() {
        popover.performClose(nil)
        if let window = reviewWindow {
            window.makeKeyAndOrderFront(nil)
            NSApp.activate(ignoringOtherApps: true)
            return
        }
        let review = ReviewModel()
        let controller = NSHostingController(rootView: ReviewView(model: review))
        let window = NSWindow(contentViewController: controller)
        window.title = "Performance Guard — Review Sessions"
        window.styleMask = [.titled, .closable, .miniaturizable, .resizable]
        window.setContentSize(NSSize(width: 900, height: 760))
        window.isReleasedWhenClosed = false
        window.center()
        reviewWindow = window
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
        Task { await review.refresh() }
    }

    private func updateIcon() {
        let color: NSColor
        let rateText: String

        if !model.daemonRunning {
            color = .gray
            rateText = " PG"
        } else {
            switch model.level {
            case "normal":
                color = .systemGreen
            case "warning":
                color = .systemYellow
            case "high":
                color = .systemOrange
            case "critical":
                color = .systemRed
            default:
                color = .gray
            }
            if model.compressionRate > 0 {
                rateText = String(format: " PG %.0f/s", model.compressionRate)
            } else {
                rateText = " PG"
            }
        }

        let size = CGSize(width: 18, height: 18)
        let image = NSImage(size: size, flipped: false) { rect in
            let circleDiameter: CGFloat = 10
            let circleRect = NSRect(
                x: (rect.width - circleDiameter) / 2,
                y: (rect.height - circleDiameter) / 2,
                width: circleDiameter,
                height: circleDiameter
            )
            color.setFill()
            NSBezierPath(ovalIn: circleRect).fill()
            return true
        }
        image.isTemplate = false

        statusItem.button?.image = image
        statusItem.button?.toolTip = "Performance Guard — \(model.daemonRunning ? model.level : "monitor unavailable")"
        statusItem.button?.setAccessibilityLabel("Performance Guard")

        let font = NSFont.monospacedDigitSystemFont(ofSize: 11, weight: .regular)
        let attrs: [NSAttributedString.Key: Any] = [.font: font]
        statusItem.button?.attributedTitle = NSAttributedString(string: rateText, attributes: attrs)
    }

    @objc private func togglePopover(_ sender: Any?) {
        if popover.isShown {
            popover.performClose(sender)
        } else if let button = statusItem.button {
            popover.show(relativeTo: button.bounds, of: button, preferredEdge: .minY)
        }
    }
}
