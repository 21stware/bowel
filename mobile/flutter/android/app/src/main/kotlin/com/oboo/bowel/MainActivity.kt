package com.oboo.bowel

import android.Manifest
import android.app.AlertDialog
import android.content.ActivityNotFoundException
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.net.VpnService
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.provider.MediaStore
import android.provider.OpenableColumns
import android.provider.Settings
import bridge.Bridge
import com.google.zxing.integration.android.IntentIntegrator
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel
import java.io.File
import java.io.FileNotFoundException
import java.text.Collator
import java.util.UUID
import java.util.concurrent.Executors

class MainActivity : FlutterActivity() {
    private val background = Executors.newCachedThreadPool()
    private val main = Handler(Looper.getMainLooper())
    private var pendingVpn: MethodChannel.Result? = null
    private var pendingScan: MethodChannel.Result? = null
    private var pendingPick: MethodChannel.Result? = null

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        val messenger = flutterEngine.dartExecutor.binaryMessenger
        MethodChannel(messenger, "bowel/shell").setMethodCallHandler(::onShell)
        Core.channel = MethodChannel(messenger, "bowel/core").also { it.setMethodCallHandler(::onCore) }
    }

    override fun onDestroy() {
        if (isFinishing) Core.channel = null
        super.onDestroy()
    }

    // Android has no native shell: Flutter draws the bar, and only the
    // preferences come here.
    private fun onShell(call: MethodCall, result: MethodChannel.Result) {
        val prefs = Core.prefs(this)
        when (call.method) {
            "getPrefs" -> result.success(prefs.all.mapValues { it.value.toString() })
            "setPref" -> {
                prefs.edit().putString(call.argument<String>("key"), call.argument<String>("value")).apply()
                result.success(null)
            }
            "scanQR" -> scanQR(result)
            // YAML has no MIME type Android applies reliably, so any file can be chosen.
            "pickFile" -> pick(result, documentPicker("*/*"))
            "pickImage" -> pick(result, imagePicker())
            "openVPNSettings" -> openVpnSettings(result)
            "listApps" -> offMain(result) { launcherApps() }
            else -> result.notImplemented()
        }
    }

    private fun scanQR(result: MethodChannel.Result) {
        if (pendingScan != null) {
            result.success(null)
            return
        }
        pendingScan = result
        if (checkSelfPermission(Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED) {
            openScanner()
        } else {
            requestPermissions(arrayOf(Manifest.permission.CAMERA), REQUEST_CAMERA)
        }
    }

    private fun openScanner() {
        val scan = IntentIntegrator(this)
            .setDesiredBarcodeFormats(IntentIntegrator.QR_CODE)
            .setPrompt(getString(R.string.scan_prompt))
            .setBeepEnabled(false)
            .setOrientationLocked(true)
            .createScanIntent()
        startActivityForResult(scan, REQUEST_SCAN)
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode != REQUEST_CAMERA) return
        if (grantResults.firstOrNull() == PackageManager.PERMISSION_GRANTED) {
            openScanner()
            return
        }
        pendingScan?.success(null)
        pendingScan = null
        // Android stops asking after a refusal or two: only Settings can grant it then.
        AlertDialog.Builder(this)
            .setMessage(R.string.camera_needed)
            .setPositiveButton(R.string.open_settings) { _, _ ->
                startActivity(Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.parse("package:$packageName")))
            }
            .setNegativeButton(android.R.string.cancel, null)
            .show()
    }

    private fun documentPicker(type: String) =
        Intent(Intent.ACTION_OPEN_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE).setType(type)

    private fun imagePicker() =
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            Intent(MediaStore.ACTION_PICK_IMAGES)
        } else {
            documentPicker("image/*")
        }

    private fun pick(result: MethodChannel.Result, picker: Intent) {
        if (pendingPick != null) {
            result.success(null)
            return
        }
        try {
            startActivityForResult(picker, REQUEST_PICK)
            pendingPick = result
        } catch (e: ActivityNotFoundException) {
            result.error("pick", e.message, null)
        }
    }

    // Only the latest pick is kept, so a subscription file does not linger here.
    private fun copyToCache(uri: Uri): String {
        val picked = File(cacheDir, "picked").apply { deleteRecursively() }
        val folder = File(picked, UUID.randomUUID().toString()).apply { mkdirs() }
        val target = File(folder, displayName(uri))
        val source = contentResolver.openInputStream(uri) ?: throw FileNotFoundException(uri.toString())
        source.use { from -> target.outputStream().use { from.copyTo(it) } }
        return target.path
    }

    private fun displayName(uri: Uri): String {
        val name = contentResolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use {
            if (it.moveToFirst()) it.getString(0) else null
        }
        // The name comes from another app: it must stay inside the folder.
        return name?.let { File(it).name }?.takeUnless { it.isEmpty() || it == "." || it == ".." } ?: "picked"
    }

    private fun openVpnSettings(result: MethodChannel.Result) {
        try {
            startActivity(Intent(Settings.ACTION_VPN_SETTINGS))
            result.success(null)
        } catch (e: ActivityNotFoundException) {
            result.error("settings", e.message, null)
        }
    }

    private fun launcherApps(): List<Map<String, String>> {
        val launchable = Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER)
        return packageManager.queryIntentActivities(launchable, 0)
            .map { it.activityInfo.applicationInfo }
            .filter { it.packageName != packageName }
            .distinctBy { it.packageName }
            .map { mapOf("id" to it.packageName, "name" to it.loadLabel(packageManager).toString()) }
            .sortedWith(compareBy(Collator.getInstance()) { it.getValue("name") })
    }

    private fun onCore(call: MethodCall, result: MethodChannel.Result) {
        when (call.method) {
            "start" -> offMain(result) { Core.start(applicationContext); null }
            "call" -> {
                val method = call.argument<String>("method") ?: ""
                val payload = call.argument<String>("payload") ?: ""
                offMain(result) { Bridge.call(method, payload) }
            }
            "setVPN" -> setVpn(call.argument<Boolean>("on") == true, result)
            else -> result.notImplemented()
        }
    }

    private fun offMain(result: MethodChannel.Result, work: () -> Any?) {
        background.execute {
            try {
                val value = work()
                main.post { result.success(value) }
            } catch (e: Exception) {
                main.post { result.error("core", e.message, null) }
            }
        }
    }

    private fun setVpn(on: Boolean, result: MethodChannel.Result) {
        if (!on) {
            startService(Intent(this, BowelVpnService::class.java).setAction(BowelVpnService.ACTION_STOP))
            result.success(null)
            return
        }
        askForNotifications()
        val consent = VpnService.prepare(this)
        if (consent == null) {
            startVpn()
            result.success(null)
            return
        }
        pendingVpn = result
        startActivityForResult(consent, REQUEST_VPN)
    }

    private fun startVpn() {
        startForegroundService(Intent(this, BowelVpnService::class.java).setAction(BowelVpnService.ACTION_START))
    }

    private fun askForNotifications() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), REQUEST_NOTIFICATIONS)
        }
    }

    @Deprecated("The VPN consent dialog, the scanner and the pickers only report back through this")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == REQUEST_PICK) {
            val result = pendingPick ?: return
            pendingPick = null
            val uri = data?.data
            if (resultCode == RESULT_OK && uri != null) offMain(result) { copyToCache(uri) } else result.success(null)
            return
        }
        if (requestCode == REQUEST_SCAN) {
            val code = IntentIntegrator.parseActivityResult(resultCode, data).contents
            pendingScan?.success(code)
            pendingScan = null
            return
        }
        if (requestCode != REQUEST_VPN) return
        val result = pendingVpn
        pendingVpn = null
        if (resultCode == RESULT_OK) {
            startVpn()
        } else {
            Core.setStatus("off")
        }
        result?.success(null)
    }

    private companion object {
        const val REQUEST_VPN = 1
        const val REQUEST_NOTIFICATIONS = 2
        const val REQUEST_SCAN = 3
        const val REQUEST_CAMERA = 4
        const val REQUEST_PICK = 5
    }
}
