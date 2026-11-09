package com.oboo.bowel

import android.content.Context
import android.content.SharedPreferences
import android.os.Handler
import android.os.Looper
import bridge.Bridge
import bridge.Sink
import io.flutter.plugin.common.MethodChannel
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

    @Volatile
    private var status = "off"

    val vpnActive get() = status == "on" || status == "connecting"

    private val main = Handler(Looper.getMainLooper())
    private var started = false

    fun dir(context: Context): File = File(context.filesDir, "core").apply { mkdirs() }

    fun prefs(context: Context): SharedPreferences = context.getSharedPreferences("bowel", Context.MODE_PRIVATE)

    @Synchronized
    fun start(context: Context) {
        if (started) return
        Bridge.start(dir(context).path, this)
        started = true
    }

    fun setStatus(status: String, error: String = "") {
        this.status = status
        Bridge.setVPNStatus(status, error)
        onStatus?.invoke()
    }

    override fun emit(name: String, payload: String) {
        main.post { channel?.invokeMethod("emit", mapOf("name" to name, "payload" to payload)) }
    }
}
