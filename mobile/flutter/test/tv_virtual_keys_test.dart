import 'package:bowel/app/state.dart';
import 'package:bowel/main.dart';
import 'package:bowel/ui/focus.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

import 'real_font.dart';

// Keys exactly as Android hands them over from `adb shell input keyevent`, or
// from a remote some TV boxes wire in the same way: from its virtual keyboard,
// device -1. Flutter alone takes that for the soft keyboard and stays in touch
// mode: focus moved, but no ring, and no level took focus. Its own file, as
// these come in raw, not as the simulator's key data.

const _dpadDown = 20, _dpadCenter = 23, _back = 4;

/// One key from Android's virtual keyboard: down, then up.
Future<bool> _androidKey(WidgetTester tester, int keyCode, {int flags = 0x8}) async {
  var handled = false;
  for (final type in ['keydown', 'keyup']) {
    final message = {'type': type, 'keymap': 'android', 'keyCode': keyCode, 'scanCode': 0, 'deviceId': -1, 'source': 0x101, 'flags': flags};
    await tester.binding.defaultBinaryMessenger.handlePlatformMessage(SystemChannels.keyEvent.name, SystemChannels.keyEvent.codec.encodeMessage(message), (data) {
      final reply = SystemChannels.keyEvent.codec.decodeMessage(data) as Map?;
      if (type == 'keydown') handled = reply?['handled'] == true;
    });
  }
  for (var i = 0; i < 8; i++) {
    await tester.pump(const Duration(milliseconds: 100));
  }
  return handled;
}

String? _mark() => FocusManager.instance.primaryFocus?.context?.findAncestorWidgetOfExactType<FocusMark>()?.id;

void main() {
  setUpAll(loadSystemFont);

  testWidgets("a D-pad from Android's virtual keyboard drives the app as a remote's does", (tester) async {
    tester.view.physicalSize = const Size(960, 540);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    watchRemoteKeys();
    final app = AppState();
    await tester.pumpWidget(BowelApp(app: app, ready: Future.value()));
    for (var i = 0; i < 16; i++) {
      await tester.pump(const Duration(milliseconds: 100));
    }
    expect(FocusManager.instance.highlightMode, FocusHighlightMode.touch);

    // A soft keyboard's cursor key leaves touch mode alone (it still moves
    // the focus, onto the tabs, as Flutter does).
    await _androidKey(tester, _dpadDown, flags: 0x2 | 0x8);
    expect(FocusManager.instance.highlightMode, FocusHighlightMode.touch);

    expect(await _androidKey(tester, _dpadDown), isTrue);
    expect(FocusManager.instance.highlightMode, FocusHighlightMode.traditional);
    expect(_mark(), 'cell:hero');

    expect(await _androidKey(tester, _dpadCenter), isTrue);
    expect(app.x, 'hero');
    expect(_mark(), anyOf('picked', startsWith('node:')));

    // Back is Android's: Flutter leaves the key, then hears the system's back.
    expect(await _androidKey(tester, _back), isFalse);
    await tester.binding.handlePopRoute();
    for (var i = 0; i < 8; i++) {
      await tester.pump(const Duration(milliseconds: 100));
    }
    expect(app.x, isNull);
    expect(_mark(), 'cell:hero');

    // A touch hands the app back to the finger.
    await tester.tapAt(const Offset(480, 300));
    await tester.pump(const Duration(seconds: 1));
    expect(FocusManager.instance.highlightMode, FocusHighlightMode.touch);
    FocusManager.instance.highlightStrategy = FocusHighlightStrategy.automatic;
  });
}
