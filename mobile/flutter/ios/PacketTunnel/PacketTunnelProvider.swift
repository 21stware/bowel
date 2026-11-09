import Darwin
import Foundation
import NetworkExtension
import Tunnel
import os

let groupID = "group.com.oboo.bowel"

final class PacketTunnelProvider: NEPacketTunnelProvider {
    private var sampler: DispatchSourceTimer?
    private var peak: UInt64 = 0
    private let log = OSLog(subsystem: "com.oboo.bowel.tunnel", category: "tunnel")

    private var dir: URL { FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: groupID)! }

    override func startTunnel(options: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        startSampler()
        let s = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: "127.0.0.1")
        let v4 = NEIPv4Settings(addresses: ["198.18.0.1"], subnetMasks: ["255.255.255.252"])
        v4.includedRoutes = [NEIPv4Route.default()]
        s.ipv4Settings = v4
        s.dnsSettings = NEDNSSettings(servers: ["198.18.0.2"])
        s.mtu = 1500
        setTunnelNetworkSettings(s) { [weak self] error in
            guard let self else { return }
            if let error { completionHandler(error); return }
            guard let fd = Self.tunnelFD() else {
                completionHandler(NSError(domain: "bowel", code: 1, userInfo: [NSLocalizedDescriptionKey: "找不到 utun 文件描述符"]))
                return
            }
            let path = self.dir.path
            let limit = Self.limitMB()
            DispatchQueue.global(qos: .userInitiated).async {
                var err: NSError?
                // Leave room for what isn't Go heap: stacks, the binary's own pages.
                TunnelStart(path, fd, max(limit - 12, 20), &err)
                if let err { os_log("start failed: %{public}@", log: self.log, type: .error, err.localizedDescription) }
                completionHandler(err)
            }
        }
    }

    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        os_log("stop reason %d", log: log, Int(reason.rawValue))
        sample(note: "stop:\(reason.rawValue)")
        DispatchQueue.global().async {
            TunnelStop()
            completionHandler()
        }
    }

    override func sleep(completionHandler: @escaping () -> Void) { completionHandler() }
    override func wake() {}

    /// The kernel-control socket of the utun the system opened for us.
    static func tunnelFD() -> Int32? {
        var name = [CChar](repeating: 0, count: Int(IFNAMSIZ))
        for fd in Int32(0)...1024 {
            var len = socklen_t(name.count)
            if getsockopt(fd, 2 /* SYSPROTO_CONTROL */, 2 /* UTUN_OPT_IFNAME */, &name, &len) == 0, String(cString: name).hasPrefix("utun") {
                return fd
            }
        }
        return nil
    }

    static func footprint() -> UInt64 {
        var info = task_vm_info_data_t()
        var count = mach_msg_type_number_t(MemoryLayout<task_vm_info_data_t>.size / MemoryLayout<natural_t>.size)
        let r = withUnsafeMutablePointer(to: &info) {
            $0.withMemoryRebound(to: integer_t.self, capacity: Int(count)) { task_info(mach_task_self_, task_flavor_t(TASK_VM_INFO), $0, &count) }
        }
        return r == KERN_SUCCESS ? info.phys_footprint : 0
    }

    /// The real limit of this process: what it uses plus what it may still use.
    static func limitMB() -> Int32 {
        Int32((footprint() + UInt64(os_proc_available_memory())) / 1_048_576)
    }

    private func startSampler() {
        let file = dir.appendingPathComponent("tunnel-mem.jsonl")
        try? FileManager.default.removeItem(at: file)
        FileManager.default.createFile(atPath: file.path, contents: nil)
        let t = DispatchSource.makeTimerSource(queue: .global(qos: .utility))
        t.schedule(deadline: .now(), repeating: 1)
        t.setEventHandler { [weak self] in self?.sample(note: nil) }
        t.resume()
        sampler = t
    }

    private func sample(note: String?) {
        let f = Self.footprint(), mb = { (b: UInt64) in Double(b) / 1_048_576 }
        peak = max(peak, f)
        var row: [String: Any] = ["t": Date().timeIntervalSince1970, "footprintMB": mb(f), "peakMB": mb(peak), "limitMB": Self.limitMB(), "go": TunnelGoStats()]
        if let note { row["note"] = note }
        guard let data = try? JSONSerialization.data(withJSONObject: row), let h = try? FileHandle(forWritingTo: dir.appendingPathComponent("tunnel-mem.jsonl")) else { return }
        _ = try? h.seekToEnd()
        try? h.write(contentsOf: data + Data([10]))
        try? h.close()
    }
}
