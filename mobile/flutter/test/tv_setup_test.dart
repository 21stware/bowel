import 'package:bowel/app/state.dart';
import 'package:bowel/main.dart';
import 'package:bowel/ui/focus.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

import 'real_font.dart';

// The first run on a TV: both steps fit 540 tall, the focus shows on the lime
// buttons, and Back goes from step 2 to step 1 before it leaves.

Future<void> _settle(WidgetTester tester) async {
  for (var i = 0; i < 8; i++) {
    await tester.pump(const Duration(milliseconds: 100));
  }
}

Future<void> _press(WidgetTester tester, LogicalKeyboardKey key) async {
  await tester.sendKeyEvent(key);
  await _settle(tester);
}

Future<void> _back(WidgetTester tester) async {
  await tester.binding.handlePopRoute();
  await _settle(tester);
}

Future<(AppState, List<MethodCall>)> _open(WidgetTester tester, Size size, {String appearance = 'system'}) async {
  tester.view.physicalSize = size;
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  final calls = <MethodCall>[];
  tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform, (call) async {
    calls.add(call);
    return null;
  });
  addTearDown(() => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform, null));
  final app = AppState()
    ..setup = 1
    ..appearance = appearance;
  await tester.pumpWidget(BowelApp(app: app, ready: Future.value()));
  await _settle(tester);
  await _settle(tester);
  return (app, calls);
}

String? _mark() => FocusManager.instance.primaryFocus?.context?.findAncestorWidgetOfExactType<FocusMark>()?.id;

void _onScreen(WidgetTester tester, String text, Size size) {
  final r = tester.getRect(find.text(text));
  expect(r.top, greaterThanOrEqualTo(0), reason: text);
  expect(r.bottom, lessThanOrEqualTo(size.height), reason: text);
}

void main() {
  setUpAll(loadSystemFont);
  setUp(() => FocusManager.instance.highlightStrategy = FocusHighlightStrategy.automatic);

  for (final appearance in ['light', 'dark']) {
    testWidgets('$appearance: on a TV both steps fit, the D-pad reaches every button, and Back goes step to step', (tester) async {
      const size = Size(960, 540);
      final (app, calls) = await _open(tester, size, appearance: appearance);
      _onScreen(tester, '继续', size);
      expect(tester.takeException(), isNull);

      // down through the step to 继续
      for (var i = 0; i < 6 && _mark() != 'next'; i++) {
        await _press(tester, LogicalKeyboardKey.arrowDown);
      }
      expect(_mark(), 'next');
      await _press(tester, LogicalKeyboardKey.select);
      expect(app.setup, 2);
      _onScreen(tester, '允许并接管', size);
      _onScreen(tester, '先跳过', size);
      expect(tester.takeException(), isNull);
      // the step starts on its first button, and the next one is a press away
      expect(find.ancestor(of: find.text('允许并接管'), matching: find.byType(KeyTarget)).evaluate().isNotEmpty, isTrue);
      final allow = Focus.of(tester.element(find.text('允许并接管')));
      expect(FocusManager.instance.primaryFocus, allow);
      await _press(tester, LogicalKeyboardKey.arrowDown);
      expect(FocusManager.instance.primaryFocus, Focus.of(tester.element(find.text('先跳过'))));

      await _back(tester);
      expect(app.setup, 1);
      expect(_mark(), 'next');
      expect(calls.any((c) => c.method == 'SystemNavigator.pop'), isFalse);
      await _back(tester);
      expect(calls.any((c) => c.method == 'SystemNavigator.pop'), isTrue);
    });
  }

  testWidgets('on a phone too, Back on step 2 goes to step 1, and the link added there is not added again', (tester) async {
    final (app, _) = await _open(tester, const Size(390, 844));
    app.setupField.text = 'vmess://x';
    app.setupLink = 'vmess://x';
    await tester.tap(find.text('继续'));
    await _settle(tester);
    expect(app.setup, 2);
    await _back(tester);
    expect(app.setup, 1);
    expect(app.setupField.text, isEmpty);
    expect(app.setupLink, isEmpty);
  });
}
