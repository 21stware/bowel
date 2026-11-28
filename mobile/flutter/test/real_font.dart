import 'dart:io';

import 'package:bowel/ui/skin.dart';
import 'package:flutter/services.dart';

/// Loads a real face as the system's, for tests that widths decide: the test
/// font draws every letter as wide as a kanji. The app draws in the phone's
/// own sans-serif; Raise Sans, the desktop's bundled face on Windows and
/// Linux, whose Chinese is a stock phone's Noto Sans CJK, stands in for it.
Future<void> loadSystemFont() async {
  final bytes = await File('../../resources/windows/fonts/RaiseSans.ttf').readAsBytes();
  await (FontLoader(systemFamily!)..addFont(Future.value(ByteData.sublistView(bytes)))).load();
}
