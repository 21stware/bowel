import 'dart:io' show Platform;

import 'package:flutter/foundation.dart';
import 'package:flutter/painting.dart' show Color, EdgeInsets;
import 'package:flutter/services.dart';

import 'i18n.dart';

/// The bridge to the native iOS shell, which owns the system tab bar.
///
/// Android has no shell: Flutter draws the Material bar there, so `insets`
/// stays null and these calls do nothing, apart from the status bar's style.
class Shell {
  Shell._();
  static final instance = Shell._();

  static const _channel = MethodChannel('bowel/shell');

  bool get native => !kIsWeb && Platform.isIOS;

  /// The screen's top and the height the tab bar covers at the bottom.
  /// UIKit doesn't pass safe areas down to a child of a tab bar controller,
  /// so the shell reports them itself.
  final insets = ValueNotifier<EdgeInsets?>(null);

  void Function(int index)? onTabSelected;

  /// What this device can do, which decides only the ways to add that are
  /// offered: Android reports it at start, and elsewhere (iOS, tests) it is
  /// a phone with a camera and pickers. The layout follows the window's
  /// size, never this (frame.dart).
  bool tv = false;
  bool camera = true;

  /// Whether something answers the document picker (a YAML file) and the
  /// photo picker (a QR code's picture). A TV often has neither.
  bool files = true, images = true;

  /// Back to a phone's, for tests.
  @visibleForTesting
  void resetDevice() {
    tv = false;
    camera = files = images = true;
  }

  Future<void> start() async {
    if (!kIsWeb && Platform.isAndroid) {
      try {
        final m = await _channel.invokeMethod<Map>('device');
        if (m != null) {
          tv = m['tv'] == true;
          camera = m['camera'] != false;
          files = m['files'] != false;
          images = m['images'] != false;
        }
      } catch (_) {} // an older host: a phone's
    }
    if (!native) return;
    _channel.setMethodCallHandler((call) async {
      switch (call.method) {
        case 'tabSelected':
          onTabSelected?.call(call.arguments as int);
        case 'insets':
          _setInsets(call.arguments as Map);
      }
    });
    final now = await _channel.invokeMethod<Map>('getInsets');
    if (now != null) _setInsets(now);
  }

  void _setInsets(Map m) => insets.value = EdgeInsets.only(top: (m['top'] as num).toDouble(), bottom: (m['bottom'] as num).toDouble());

  void setTab(int index) {
    if (native) _channel.invokeMethod('setTab', {'index': index});
  }

  void setStyle(Color tint, {required bool dark, required Color paper}) {
    if (native) {
      _channel.invokeMethod('setStyle', {'tint': tint.toARGB32(), 'dark': dark, 'paper': paper.toARGB32()});
      return;
    }
    // Android follows the system's light or dark, not the skin, until told.
    SystemChrome.setSystemUIOverlayStyle(SystemUiOverlayStyle(statusBarIconBrightness: dark ? Brightness.light : Brightness.dark));
  }

  void setBadge(int index, {required bool on, Color? color}) {
    if (native) _channel.invokeMethod('setBadge', {'index': index, 'on': on, if (color != null) 'color': color.toARGB32()});
  }

  void setTabTitles(List<String> titles) {
    if (native) _channel.invokeMethod('setTabTitles', {'titles': titles});
  }

  void setBarHidden(bool hidden) {
    if (native) _channel.invokeMethod('setBarHidden', {'hidden': hidden});
  }

  Future<Map<String, String>> loadPrefs() async {
    if (kIsWeb) return const {};
    final m = await _channel.invokeMethod<Map>('getPrefs');
    return m?.cast<String, String>() ?? const {};
  }

  /// Opens the camera; the first QR code it reads, or null if cancelled.
  Future<String?> scanQR() => _channel.invokeMethod<String>('scanQR', {
        'aim': tr('对准订阅或节点的二维码', 'Point at a subscription or node QR code'),
        'settings': tr('打开系统设置', 'Open Settings'),
        'cancel': tr('取消', 'Cancel'),
        'denied': tr('相机权限已关闭，请在系统设置中允许 bowel 使用相机', 'Camera access is off. Allow bowel to use the camera in Settings.'),
        'noCamera': tr('这台设备没有可用的相机', 'This device has no usable camera'),
      });

  /// A document the user picks, copied somewhere the app can read; null if cancelled.
  Future<String?> pickFile() => _channel.invokeMethod<String>('pickFile');

  /// A picture from the photo library, copied likewise.
  Future<String?> pickImage() => _channel.invokeMethod<String>('pickImage');

  /// Android's VPN page, where always-on lives.
  Future<void> openVPNSettings() => _channel.invokeMethod('openVPNSettings');

  /// An https page in the browser, such as Tailscale's login. With [http],
  /// an http one too: a self-hosted control server (Headscale) may have no TLS.
  Future<void> openURL(String url, {bool http = false}) => _channel.invokeMethod('openURL', {'url': url, if (http) 'http': true});

  /// This copy's version on Android, where the app looks for updates itself.
  Future<String> appVersion() async => !kIsWeb && Platform.isAndroid ? await _channel.invokeMethod<String>('appVersion') ?? '' : '';

  /// The apps that can be left out of the VPN, on Android.
  Future<List<({String id, String name})>> listApps() async {
    final list = await _channel.invokeMethod<List>('listApps') ?? const [];
    return [for (final raw in list) (id: (raw as Map)['id'] as String, name: raw['name'] as String)];
  }

  /// Whether bowel can see package `id` on this phone, which a rule for it
  /// needs: not when it isn't installed, nor when Android keeps it from
  /// other apps. Null where that can't be asked.
  Future<bool?> canSeeApp(String id) async {
    try {
      return await _channel.invokeMethod<bool>('canSeeApp', {'id': id});
    } catch (_) {
      return null;
    }
  }

  void setPref(String key, String value) {
    if (!kIsWeb) _channel.invokeMethod('setPref', {'key': key, 'value': value});
  }
}
