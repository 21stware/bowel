import 'dart:io';

import 'package:bowel/ui/skin.dart';
import 'package:flutter/services.dart';

/// Loads a real face as the system's, for tests that widths decide: the test
/// font draws every letter as wide as a kanji. The app draws in the phone's
/// own sans-serif; OPPO Sans, the desktop's bundled face and the system's on
/// OPPO's and OnePlus's phones, stands in for it.
Future<void> loadSystemFont() async {
  final bytes = await File('../../resources/fonts/OPPOSans4.0.ttf').readAsBytes();
  await (FontLoader(systemFamily!)..addFont(Future.value(ByteData.sublistView(bytes)))).load();
}
