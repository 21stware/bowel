package com.oboo.bowel

import android.content.Context
import android.content.SharedPreferences
import android.os.Build
import android.os.Handler
import android.os.Looper
import bridge.Bridge
import bridge.Sink
import io.flutter.plugin.common.MethodChannel
import org.json.JSONException
import org.json.JSONObject
import java.io.File

/**
 * The Go bridge for the whole process. It outlives the activity, because the
 * VPN keeps running without one: a new activity just takes over the channel.
 */
object Core : Sink {
    @Volatile
    var channel: MethodChannel? = null

    /** Called on any thread when the VPN's status changes. */
    @Volatile
    var onStatus: (() -> Unit)? = null

    /**
     * Called on the bridge's thread with each tick's rates, split by exit,
     * while something listens: the VPN's notification shows them.
     */
    @Volatile
    var onSpeed: ((Speed) -> Unit)? = null

    @Volatile
    private var status = "off"

    val vpnActive get() = status == "on" || status == "connecting"

    val vpnOn get() = status == "on"

    private val main = Handler(Looper.getMainLooper())
    private var started = false

    fun dir(context: Context): File = File(context.filesDir, "core").apply { mkdirs() }

    fun prefs(context: Context): SharedPreferences = context.getSharedPreferences("bowel", Context.MODE_PRIVATE)

    @Synchronized
    fun start(context: Context) {
        if (started) return
        // The VPN can say which app owns a connection from Android 10
        // (ConnectionOwners), so rules for apps work from there.
        Bridge.setAppRules(Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q)
        Bridge.start(dir(context).path, this)
        started = true
    }

    fun setStatus(status: String, error: String = "") {
        this.status = status
        Bridge.setVPNStatus(status, error)
        onStatus?.invoke()
    }

    override fun emit(name: String, payload: String) {
        val speed = onSpeed
        if (speed != null && name == "tick") Speed.of(payload)?.let(speed)
        main.post { channel?.invokeMethod("emit", mapOf("name" to name, "payload" to payload)) }
    }
}

/** What the connections carry now, in bytes a second, through a node or directly. */
data class Speed(val proxyDown: Long, val proxyUp: Long, val directDown: Long, val directUp: Long) {
    companion object {
        fun of(tick: String): Speed? = try {
            val t = JSONObject(tick)
            Speed(t.optLong("proxyDown"), t.optLong("proxyUp"), t.optLong("directDown"), t.optLong("directUp"))
        } catch (_: JSONException) {
            null
        }

        /** Three significant figures, as the app's own speed readouts. */
        fun text(bytes: Long): String {
            val units = arrayOf("B", "KB", "MB", "GB", "TB")
            var v = bytes.toDouble()
            var i = 0
            while (v >= 1024 && i < units.size - 1) {
                v /= 1024
                i++
            }
            if (i == 0) return "$bytes B/s"
            val n = when {
                v >= 100 -> "%.0f"
                v >= 10 -> "%.1f"
                else -> "%.2f"
            }.format(java.util.Locale.ROOT, v)
            return "$n ${units[i]}/s"
        }
    }
}
