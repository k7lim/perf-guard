import SwiftUI
import AppKit
import ServiceManagement

@main
struct PerfGuardMenuApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) var appDelegate

    var body: some Scene {
        Settings {
            EmptyView()
        }
    }
}

class AppDelegate: NSObject, NSApplicationDelegate {
    var statusBarController: StatusBarController?

    func applicationDidFinishLaunching(_ notification: Notification) {
        if CommandLine.arguments.contains("--enable-login") {
            do {
                try SMAppService.mainApp.register()
                NSLog("Performance Guard login status: %ld", SMAppService.mainApp.status.rawValue)
            } catch {
                NSLog("Performance Guard login registration failed: %@", error.localizedDescription)
            }
        }
        statusBarController = StatusBarController()
    }
}
