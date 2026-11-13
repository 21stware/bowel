package com.oboo.bowel

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import android.graphics.drawable.Icon
import android.net.LocalSocket
import android.net.LocalSocketAddress
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.util.Log
import bridge.Bridge
import java.io.File
import java.io.FileWriter
import java.io.IOException
import java.util.concurrent.TimeUnit

/**
 * The VPN. mihomo runs as a process of its own, started from the app's native
 * library folder. The app is excluded from its own VPN, so mihomo, under the
 * same UID, reaches the network directly and never loops back into the TUN.
 */
class BowelVpnService : VpnService() {
    @Volatile
    private var stopping = false

    @Volatile
    private var process: Process? = null
    private var worker: Thread? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            stop()
            return START_NOT_STICKY
        }
        showNotification()
        if (worker?.isAlive != true) {
            stopping = false
            // mihomo is told to exit when the thread that started it ends, so
            // this thread lives exactly as long as the core does.
            worker = Thread(::run, "bowel-core").apply { start() }
        }
        return START_NOT_STICKY
    }

    override fun onRevoke() {
        stop()
    }

    override fun onDestroy() {
        stop()
        super.onDestroy()
    }

    private fun stop() {
        stopping = true
        process?.destroy()
        val core = process
        if (core != null) {
            Thread {
                if (!core.waitFor(STOP_TIMEOUT_SECONDS, TimeUnit.SECONDS)) core.destroyForcibly()
            }.start()
        }
        if (worker?.isAlive != true) finish()
    }

    private fun finish() {
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    private fun run() {
        var failure = ""
        var tun: ParcelFileDescriptor? = null
        try {
            // Before the status: the tile can start the VPN with no activity
            // having started the bridge.
            Core.start(this)
            Core.setStatus("connecting")
            val dir = Core.dir(this)
            copyGeo(dir)
            Bridge.call("prepareTunnel", "")
            tun = openTun()
            val core = startCore(dir)
            process = core
            // Without its TUN the core would run on while the phone's traffic
            // goes nowhere: end it, so the failure shows.
            val log = CoreLog(File(dir, "core.log"), File(dir, TAILNET_LOG)) { core.destroy() }
            Thread({ log.readAll(core) }, "bowel-core-log").start()
            sendDescriptor(File(dir, "tunfd.sock"), tun, core)
            waitForController(File(dir, "c.sock"), core)
            if (!stopping) Core.setStatus("on")
            val code = core.waitFor()
            if (!stopping) failure = log.fatal ?: log.lastLines().ifEmpty { "core exited with $code" }
        } catch (e: Exception) {
            Log.e(TAG, "vpn failed", e)
            failure = e.message ?: e.toString()
            process?.destroyForcibly()
        } finally {
            tun?.close()
            process = null
            Core.setStatus(if (failure.isEmpty()) "off" else "error", failure)
            finish()
        }
    }

    private fun openTun(): ParcelFileDescriptor {
        val builder = Builder()
            .setSession("bowel")
            .setMtu(1500)
            .addAddress("198.18.0.1", 30)
            .addRoute("0.0.0.0", 0)
            // Anything inside the TUN: mihomo answers all of port 53 itself.
            .addDnsServer("198.18.0.2")
            .addDisallowedApplication(packageName)
        excludeChosenApps(builder)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) builder.setMetered(false)
        return builder.establish() ?: throw IOException(getString(R.string.vpn_permission_lost))
    }

    private fun excludeChosenApps(builder: Builder) {
        val chosen = Core.prefs(this).getString(PREF_EXCLUDED, null).orEmpty().split(',')
        for (app in chosen.map { it.trim() }.filter { it.isNotEmpty() && it != packageName }.distinct()) {
            try {
                builder.addDisallowedApplication(app)
            } catch (_: PackageManager.NameNotFoundException) {
                // Uninstalled since it was chosen.
            }
        }
    }

    private fun startCore(dir: File): Process {
        val libs = applicationInfo.nativeLibraryDir
        return ProcessBuilder("$libs/liblauncher.so", dir.path, "$libs/libmihomo.so")
            .directory(dir)
            .redirectErrorStream(true)
            .start()
    }

    // The launcher listens once on a socket in the app's private folder.
    private fun sendDescriptor(path: File, tun: ParcelFileDescriptor, core: Process) {
        val deadline = System.currentTimeMillis() + DESCRIPTOR_TIMEOUT_MS
        while (true) {
            val socket = LocalSocket()
            try {
                socket.connect(LocalSocketAddress(path.path, LocalSocketAddress.Namespace.FILESYSTEM))
                socket.setFileDescriptorsForSend(arrayOf(tun.fileDescriptor))
                socket.outputStream.write(1)
                socket.outputStream.flush()
                // The launcher closes first, once it holds the descriptor.
                socket.inputStream.read()
                return
            } catch (e: IOException) {
                if (!core.isAlive || System.currentTimeMillis() > deadline) throw e
                Thread.sleep(POLL_MS)
            } finally {
                socket.close()
            }
        }
    }

    private fun waitForController(path: File, core: Process) {
        val deadline = System.currentTimeMillis() + CONTROLLER_TIMEOUT_MS
        while (!stopping) {
            if (!core.isAlive) return
            val socket = LocalSocket()
            try {
                socket.connect(LocalSocketAddress(path.path, LocalSocketAddress.Namespace.FILESYSTEM))
                return
            } catch (e: IOException) {
                if (System.currentTimeMillis() > deadline) throw IOException(getString(R.string.core_no_answer), e)
                Thread.sleep(POLL_MS)
            } finally {
                socket.close()
            }
        }
    }

    private fun copyGeo(dir: File) {
        for (name in listOf("GeoSite.dat", "GeoIP.dat")) {
            val target = File(dir, name)
            if (target.exists()) continue
            assets.open(name).use { from -> target.outputStream().use { from.copyTo(it) } }
        }
    }

    private fun showNotification() {
        val manager = getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(
            NotificationChannel(CHANNEL, getString(R.string.vpn_channel), NotificationManager.IMPORTANCE_LOW),
        )
        val open = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE,
        )
        val disconnect = PendingIntent.getService(
            this, 0, Intent(this, BowelVpnService::class.java).setAction(ACTION_STOP), PendingIntent.FLAG_IMMUTABLE,
        )
        val notification = Notification.Builder(this, CHANNEL)
            .setSmallIcon(R.drawable.ic_stat_bowel)
            .setContentTitle("bowel")
            .setContentText(getString(R.string.vpn_running))
            .setContentIntent(open)
            .setOngoing(true)
            .addAction(
                Notification.Action.Builder(
                    Icon.createWithResource(this, R.drawable.ic_stat_bowel), getString(R.string.vpn_disconnect), disconnect,
                ).build(),
            )
            .build()
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startForeground(NOTIFICATION_ID, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        } else {
            startForeground(NOTIFICATION_ID, notification)
        }
    }

    /**
     * Keeps the core's output for logcat, a file, and the error shown when it
     * dies. Its Tailscale lines also go to a file of their own, which the
     * bridge reads for the node's state: the core can't be asked for it. That
     * file is appended to, so the bridge may empty it.
     */
    private class CoreLog(file: File, tailnetFile: File, private val onFatal: () -> Unit) {
        private val writer = FileWriter(file, false)
        private val tailnet = run {
            tailnetFile.writeText("")
            FileWriter(tailnetFile, true)
        }
        private val last = ArrayDeque<String>()

        @Volatile
        var fatal: String? = null
            private set

        fun readAll(core: Process) {
            try {
                core.inputStream.bufferedReader().forEachLine(::add)
            } catch (_: IOException) {
                // Process.destroy closes the stream under this read.
            } finally {
                close()
            }
        }

        @Synchronized
        private fun add(line: String) {
            Log.i(TAG, line)
            writer.appendLine(line)
            writer.flush()
            if (TAILSCALE in line) {
                tailnet.appendLine(line)
                tailnet.flush()
            }
            last.addLast(line)
            if (last.size > ERROR_LINES) last.removeFirst()
            if (fatal == null && TUN_FAILED in line) {
                fatal = line
                onFatal()
            }
        }

        @Synchronized
        fun lastLines() = last.joinToString("\n")

        @Synchronized
        private fun close() {
            writer.close()
            tailnet.close()
        }
    }

    companion object {
        const val ACTION_START = "com.oboo.bowel.START"
        const val ACTION_STOP = "com.oboo.bowel.STOP"

        private const val TAG = "bowel-core"
        private const val PREF_EXCLUDED = "vpnExcluded"
        private const val CHANNEL = "vpn"
        private const val NOTIFICATION_ID = 1
        private const val ERROR_LINES = 5
        private const val TUN_FAILED = "Start TUN listening error"
        private const val TAILSCALE = "[Tailscale]"
        private const val TAILNET_LOG = "tailnet.log"
        private const val POLL_MS = 100L
        private const val DESCRIPTOR_TIMEOUT_MS = 10_000L
        private const val CONTROLLER_TIMEOUT_MS = 30_000L
        private const val STOP_TIMEOUT_SECONDS = 3L
    }
}
