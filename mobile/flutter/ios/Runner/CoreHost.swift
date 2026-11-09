import Bridge
import Flutter
import Foundation
import NetworkExtension

/// Connects Flutter to the Go bridge and the Packet Tunnel extension.
///
/// Go keeps Providers and config in the App Group folder, which the extension
/// reads when it starts. Status changes of the VPN are passed on to Go, which
/// attaches to the core's controller once the tunnel is up.
final class CoreHost: NSObject, BridgeSinkProtocol {
  static let groupID = "group.com.oboo.bowel"
  static let tunnelID = "com.oboo.bowel.tunnel"

  private let channel: FlutterMethodChannel
  private var manager: NETunnelProviderManager?
  private var started = false
  private var stopRequested = false
  private var observer: NSObjectProtocol?
  private let work = DispatchQueue(label: "bowel.core", qos: .userInitiated, attributes: .concurrent)

  init(messenger: FlutterBinaryMessenger) {
    channel = FlutterMethodChannel(name: "bowel/core", binaryMessenger: messenger)
    super.init()
    channel.setMethodCallHandler { [weak self] call, result in
      self?.handle(call, result: result)
    }
  }

  deinit {
    if let observer { NotificationCenter.default.removeObserver(observer) }
  }

  func emit(_ name: String?, payload: String?) {
    guard let name, let payload else { return }
    DispatchQueue.main.async { [channel] in
      channel.invokeMethod("emit", arguments: ["name": name, "payload": payload])
    }
  }

  private var dir: URL? {
    #if targetEnvironment(simulator)
      // No App Group without signing, and no VPN to share it with either.
      return FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first.map {
        try? FileManager.default.createDirectory(at: $0, withIntermediateDirectories: true)
        return $0
      }
    #else
      return FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: Self.groupID)
    #endif
  }

  private func handle(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
    let args = call.arguments as? [String: Any] ?? [:]
    switch call.method {
    case "start":
      start(result)
    case "call":
      let method = args["method"] as? String ?? ""
      let payload = args["payload"] as? String ?? ""
      work.async {
        var err: NSError?
        let out = BridgeCall(method, payload, &err)
        DispatchQueue.main.async {
          if let err {
            result(FlutterError(code: "core", message: err.localizedDescription, details: nil))
          } else {
            result(out)
          }
        }
      }
    case "setVPN":
      setVPN(on: (args["on"] as? Bool) == true, result)
    default:
      result(FlutterMethodNotImplemented)
    }
  }

  private func start(_ result: @escaping FlutterResult) {
    if started {
      result(nil)
      return
    }
    guard let dir else {
      result(FlutterError(code: "core", message: "App Group 不可用", details: nil))
      return
    }
    copyGeo(to: dir)
    var err: NSError?
    BridgeStart(dir.path, self, &err)
    if let err {
      result(FlutterError(code: "core", message: err.localizedDescription, details: nil))
      return
    }
    started = true
    observer = NotificationCenter.default.addObserver(
      forName: .NEVPNStatusDidChange, object: nil, queue: .main
    ) { [weak self] note in
      guard let session = note.object as? NETunnelProviderSession else { return }
      self?.report(session.status, session: session)
    }
    NETunnelProviderManager.loadAllFromPreferences { [weak self] list, _ in
      guard let self else { return }
      self.manager = list?.first
      if let session = self.manager?.connection as? NETunnelProviderSession {
        self.report(session.status, session: session, initial: true)
      }
    }
    result(nil)
  }

  /// The rule data the core reads, once. The app ships it so the first start
  /// needs no download and the extension's memory isn't spent on one.
  private func copyGeo(to dir: URL) {
    let fm = FileManager.default
    for file in ["GeoSite.dat", "GeoIP.dat"] {
      let dst = dir.appendingPathComponent(file)
      guard !fm.fileExists(atPath: dst.path),
        let src = Bundle.main.url(forResource: file, withExtension: nil, subdirectory: "geo")
      else { continue }
      try? fm.copyItem(at: src, to: dst)
    }
  }

  private func report(_ status: NEVPNStatus, session: NETunnelProviderSession, initial: Bool = false) {
    let text: String
    switch status {
    case .connected: text = "on"
    case .connecting, .reasserting: text = "connecting"
    case .disconnecting: text = "disconnecting"
    default: text = "off"
    }
    // A failure counts only when it happens now: the system keeps the last
    // disconnect's error around, and a stop the user asked for has no failure.
    if status == .disconnected && !initial && !stopRequested {
      session.fetchLastDisconnectError { error in
        let reason = error?.localizedDescription ?? ""
        self.work.async { BridgeSetVPNStatus(reason.isEmpty ? text : "error", reason) }
      }
    } else {
      if status == .disconnected { stopRequested = false }
      work.async { BridgeSetVPNStatus(text, "") }
    }
  }

  private func setVPN(on: Bool, _ result: @escaping FlutterResult) {
    if !on {
      stopRequested = true
      guard let manager, manager.isOnDemandEnabled else {
        self.manager?.connection.stopVPNTunnel()
        result(nil)
        return
      }
      // iOS would otherwise connect it straight back.
      manager.isOnDemandEnabled = false
      manager.saveToPreferences { _ in
        manager.connection.stopVPNTunnel()
        result(nil)
      }
      return
    }
    work.async { [weak self] in
      var err: NSError?
      BridgeCall("prepareTunnel", "", &err)
      DispatchQueue.main.async {
        guard let self else { return }
        if let err {
          result(FlutterError(code: "core", message: err.localizedDescription, details: nil))
          return
        }
        self.ensureManager { manager, error in
          guard let manager else {
            result(FlutterError(code: "vpn", message: error?.localizedDescription ?? "无法添加 VPN 配置", details: nil))
            return
          }
          do {
            try manager.connection.startVPNTunnel()
            result(nil)
          } catch {
            result(FlutterError(code: "vpn", message: error.localizedDescription, details: nil))
          }
        }
      }
    }
  }

  /// Saving the configuration is what makes iOS ask the user the first time.
  /// It has to be loaded again afterwards or starting it fails.
  private func ensureManager(_ done: @escaping (NETunnelProviderManager?, Error?) -> Void) {
    NETunnelProviderManager.loadAllFromPreferences { [weak self] list, error in
      if let error {
        done(nil, error)
        return
      }
      let manager = list?.first ?? NETunnelProviderManager()
      let proto = manager.protocolConfiguration as? NETunnelProviderProtocol ?? NETunnelProviderProtocol()
      proto.providerBundleIdentifier = Self.tunnelID
      proto.serverAddress = "bowel"
      manager.protocolConfiguration = proto
      manager.localizedDescription = "bowel"
      manager.isEnabled = true
      let onDemand = (UserDefaults.standard.dictionary(forKey: "bowel.prefs")?["vpnOnDemand"] as? String) == "1"
      manager.isOnDemandEnabled = onDemand
      manager.onDemandRules = onDemand ? [NEOnDemandRuleConnect()] : nil
      manager.saveToPreferences { error in
        if let error {
          done(nil, error)
          return
        }
        manager.loadFromPreferences { error in
          self?.manager = manager
          done(error == nil ? manager : nil, error)
        }
      }
    }
  }
}
