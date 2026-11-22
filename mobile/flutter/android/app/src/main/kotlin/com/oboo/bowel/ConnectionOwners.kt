package com.oboo.bowel

import android.content.Context
import android.net.ConnectivityManager
import android.net.LocalSocket
import android.net.LocalSocketAddress
import android.os.Build
import android.system.OsConstants
import android.util.Log
import androidx.annotation.RequiresApi
import java.io.File
import java.io.IOException
import java.net.InetAddress
import java.net.InetSocketAddress
import java.util.concurrent.ConcurrentHashMap

/**
 * Tells the core which app opened a connection, so that a rule for an app
 * matches. From Android 10 only the active VPN may ask Android this, and the
 * core is a process of its own: it listens on a socket in its folder, and
 * this connects to it and answers each line it asks (mobile/android/owner):
 *
 *     tcp 198.18.0.1:40312 198.18.0.9:443   →   10234 com.openai.chatgpt
 *
 * A uid's package is kept, as it doesn't change while the VPN runs; -1 says
 * Android doesn't know the connection.
 */
@RequiresApi(Build.VERSION_CODES.Q)
class ConnectionOwners(private val context: Context, private val socket: File, private val running: () -> Boolean) {
    private val connectivity = context.getSystemService(ConnectivityManager::class.java)
    private val packages = ConcurrentHashMap<Int, String>()

    /** Opens the lines the core asks on, each answering one question at a time. */
    fun start() {
        repeat(LINES) { Thread(::serve, "bowel-owners").apply { isDaemon = true }.start() }
    }

    // The core drops a line whose answer came too late: this opens it again
    // for as long as the core runs.
    private fun serve() {
        while (running()) {
            val line = LocalSocket()
            try {
                line.connect(LocalSocketAddress(socket.path, LocalSocketAddress.Namespace.FILESYSTEM))
                val reader = line.inputStream.bufferedReader()
                val writer = line.outputStream.bufferedWriter()
                while (true) {
                    val question = reader.readLine() ?: break
                    writer.write(answer(question))
                    writer.write("\n")
                    writer.flush()
                }
            } catch (e: IOException) {
                if (running()) Log.w(TAG, "owner line: ${e.message}")
            } finally {
                line.close()
            }
            Thread.sleep(RECONNECT_MS)
        }
    }

    internal fun answer(question: String): String {
        val parts = question.split(' ')
        val protocol = when (parts.getOrNull(0)) {
            "tcp" -> OsConstants.IPPROTO_TCP
            "udp" -> OsConstants.IPPROTO_UDP
            else -> return UNKNOWN
        }
        if (parts.size != 3) return UNKNOWN
        val uid = try {
            connectivity.getConnectionOwnerUid(protocol, address(parts[1]), address(parts[2]))
        } catch (e: Exception) {
            // SecurityException once the VPN is no longer this app's, or an
            // address that isn't one.
            android.os.Process.INVALID_UID
        }
        if (uid == android.os.Process.INVALID_UID) return UNKNOWN
        return "$uid ${packageOf(uid)}"
    }

    // Apps sharing a uid are one to Android's network: the first one names it.
    private fun packageOf(uid: Int): String = packages.getOrPut(uid) {
        context.packageManager.getPackagesForUid(uid)?.firstOrNull().orEmpty()
    }

    companion object {
        const val SOCKET = "owner.sock"
        private const val TAG = "bowel-owners"
        private const val LINES = 2
        private const val RECONNECT_MS = 200L
        private const val UNKNOWN = "-1 "

        /** An IP literal and its port, as Go writes them: 1.2.3.4:5 or [::1]:5. */
        internal fun address(text: String): InetSocketAddress {
            val at = text.lastIndexOf(':')
            require(at > 0) { "no port in $text" }
            val host = text.substring(0, at).removePrefix("[").removeSuffix("]")
            require(host.isNotEmpty() && (host.contains(':') || host.all { it.isDigit() || it == '.' })) { "not an IP: $text" }
            // A literal: no lookup happens.
            return InetSocketAddress(InetAddress.getByName(host), text.substring(at + 1).toInt())
        }
    }
}
