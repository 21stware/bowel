package com.oboo.bowel

import android.app.PendingIntent
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService

/** The Quick Settings tile: shows whether the VPN is up and turns it on or off. */
class BowelTileService : TileService() {
    override fun onStartListening() {
        Core.onStatus = ::refresh
        refresh()
    }

    override fun onStopListening() {
        Core.onStatus = null
    }

    override fun onClick() {
        if (isLocked) unlockAndRun(::toggle) else toggle()
    }

    private fun toggle() {
        val service = Intent(this, BowelVpnService::class.java)
        when {
            Core.vpnActive -> startService(service.setAction(BowelVpnService.ACTION_STOP))
            // The consent dialog needs an activity, and the app has the switch.
            VpnService.prepare(this) != null -> openApp()
            else -> startForegroundService(service.setAction(BowelVpnService.ACTION_START))
        }
    }

    private fun openApp() {
        val app = Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startActivityAndCollapse(PendingIntent.getActivity(this, 0, app, PendingIntent.FLAG_IMMUTABLE))
        } else {
            @Suppress("DEPRECATION")
            startActivityAndCollapse(app)
        }
    }

    private fun refresh() {
        val tile = qsTile ?: return
        tile.state = if (Core.vpnActive) Tile.STATE_ACTIVE else Tile.STATE_INACTIVE
        tile.updateTile()
    }
}
