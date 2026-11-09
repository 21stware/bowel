import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

/// The bridge to the native host, which runs the Go side and the VPN.
///
/// [live] stays false where there is no host (or with
/// `--dart-define=BOWEL_SAMPLE=true`); the app then shows its sample values.
class Core {
  Core._();
  static final instance = Core._();

  static const _channel = MethodChannel('bowel/core');
  static const _sample = bool.fromEnvironment('BOWEL_SAMPLE');

  bool live = false;

  /// Go's pushes, already decoded: "view", "tick", "conns".
  void Function(String name, Object? value)? onEmit;

  Future<void> start() async {
    if (_sample) return;
    _channel.setMethodCallHandler((call) async {
      if (call.method != 'emit') return;
      final m = call.arguments as Map;
      onEmit?.call(m['name'] as String, jsonDecode(m['payload'] as String));
    });
    try {
      await _channel.invokeMethod('start');
      live = true;
    } on MissingPluginException {
      debugPrint('core: no native host');
    } on PlatformException catch (e) {
      debugPrint('core: ${e.message}');
    }
  }

  Future<Object?> call(String method, [Map<String, Object?> args = const {}]) async {
    final out = await _channel.invokeMethod<String>('call', {'method': method, 'payload': jsonEncode(args)});
    return out == null || out.isEmpty ? null : jsonDecode(out);
  }

  Future<void> setVPN(bool on) => _channel.invokeMethod('setVPN', {'on': on});

  /// What a failed call says, for a toast.
  static String message(Object e) => e is PlatformException ? (e.message ?? e.code) : '$e';
}
