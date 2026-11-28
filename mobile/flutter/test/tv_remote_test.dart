import 'package:bowel/app/state.dart';
import 'package:bowel/main.dart';
import 'package:bowel/pages/frame.dart';
import 'package:bowel/pages/settings.dart';
import 'package:bowel/pages/sheet.dart';
import 'package:bowel/ui/atoms.dart';
import 'package:bowel/ui/focus.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

import 'real_font.dart';

// A TV remote, as Android hands it to Flutter: arrows, OK (DPAD_CENTER is
// "select"), and Back, which Flutter sees as a key it leaves alone and then
// as the system's back.

/// Lets a card open or close, a page slide: the board never settles, since
/// a measuring shimmer runs on.
Future<void> _settle(WidgetTester tester) async {
  for (var i = 0; i < 8; i++) {
    await tester.pump(const Duration(milliseconds: 100));
  }
}

FocusNode get _now => FocusManager.instance.primaryFocus!;

/// The mark nearest the focus: a card, a row, the option in use.
String? _mark([FocusNode? n]) => (n ?? _now).context?.findAncestorWidgetOfExactType<FocusMark>()?.id;

/// The exit in use, when the list has been built down to it; the first row
/// when it hasn't, so the D-pad lands at once.
final _exitRow = anyOf('picked', startsWith('node:'));

Future<void> _press(WidgetTester tester, LogicalKeyboardKey key, {int times = 1}) async {
  for (var i = 0; i < times; i++) {
    await tester.sendKeyEvent(key);
    await _settle(tester);
  }
}

/// Android has no physical key for BACK in Flutter's tables; the logical key,
/// from its key code, is what anything would act on.
const _backScan = PhysicalKeyboardKey.escape;

/// Back on a remote. The key is Android's to turn into a back, not Flutter's:
/// were Flutter to take it, Android would go back a second time.
Future<void> _back(WidgetTester tester) async {
  expect(await tester.sendKeyDownEvent(LogicalKeyboardKey.goBack, physicalKey: _backScan), isFalse);
  expect(await tester.sendKeyUpEvent(LogicalKeyboardKey.goBack, physicalKey: _backScan), isFalse);
  await tester.binding.handlePopRoute();
  await _settle(tester);
}

Future<(AppState, List<MethodCall>)> _open(WidgetTester tester, Size size, {FakeViewPadding? bars}) async {
  tester.view.physicalSize = size;
  tester.view.devicePixelRatio = 1;
  if (bars != null) tester.view.padding = bars;
  addTearDown(tester.view.reset);
  final calls = <MethodCall>[];
  tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform, (call) async {
    calls.add(call);
    return null;
  });
  addTearDown(() => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform, null));
  final app = AppState();
  await tester.pumpWidget(BowelApp(app: app, ready: Future.value()));
  // the launch line draws and fades before the app takes a touch
  await _settle(tester);
  await _settle(tester);
  return (app, calls);
}

bool _exited(List<MethodCall> calls) => calls.any((c) => c.method == 'SystemNavigator.pop');

/// Focus where the D-pad would have it, without walking there.
Future<void> _focusOn(WidgetTester tester, Finder f) async {
  focusTo(Focus.of(tester.element(f)));
  await _settle(tester);
}

void main() {
  setUpAll(loadSystemFont);
  setUp(() => FocusManager.instance.highlightStrategy = FocusHighlightStrategy.automatic);

  testWidgets('a 1080p TV gets the wide board inside a margin, and every card fits', (tester) async {
    final (app, _) = await _open(tester, const Size(960, 540));
    expect(find.byType(TopTabs), findsOneWidget);
    expect(find.byType(AndroidBar), findsNothing);
    // the board keeps off the TV's edges
    final board = tester.getRect(find.byType(IndexedStack));
    expect(board.left, 48);
    expect(board.right, 960 - 48);
    expect(board.bottom, 540 - 27);
    for (final id in ['hero', 'map', 'mode', 'routes', 'speed', 'apps']) {
      app.open(id);
      await _settle(tester);
      expect(tester.takeException(), isNull, reason: id);
      app.collapse();
      await _settle(tester);
    }
  });

  // A phone's system image made TV-shaped, as the emulator is: its status bar
  // and gesture strip are margin enough, and the speed card's lines all show.
  testWidgets('a 960x540 screen with system bars still holds every card', (tester) async {
    await _open(tester, const Size(960, 540), bars: const FakeViewPadding(top: 29.5, bottom: 29));
    final board = tester.getRect(find.byType(IndexedStack));
    expect(board.top, 29.5 + 50); // the bar, then the tabs: no margin on top of the bar
    expect(board.bottom, 540 - 29);
    expect(board.left, 48);
    final speed = tester.getRect(find.byWidgetPredicate((w) => w is FocusMark && w.id == 'cell:speed'));
    expect(speed.bottom, lessThanOrEqualTo(board.bottom + .01));
    expect(tester.getRect(find.textContaining('今日').first).bottom, lessThanOrEqualTo(speed.bottom));
    expect(tester.takeException(), isNull);
  });

  testWidgets('the D-pad goes card to card, OK opens one, Back closes it onto the card, then the tabs, then out', (tester) async {
    final (app, calls) = await _open(tester, const Size(960, 540));

    // Nothing is focused yet; the first press finds the tabs along the top.
    await _press(tester, LogicalKeyboardKey.arrowDown);
    expect(_now.debugLabel, 'tab 0');
    expect(FocusManager.instance.highlightMode, FocusHighlightMode.traditional);

    await _press(tester, LogicalKeyboardKey.arrowDown);
    expect(_mark(), 'cell:hero');
    await _press(tester, LogicalKeyboardKey.arrowRight);
    expect(_mark(), 'cell:map');
    await _press(tester, LogicalKeyboardKey.arrowDown);
    expect(_mark(), 'cell:routes');
    await _press(tester, LogicalKeyboardKey.arrowLeft);
    expect(_mark(), 'cell:mode');
    await _press(tester, LogicalKeyboardKey.arrowUp);
    expect(_mark(), 'cell:hero');

    // OK opens the card, onto the exit in use.
    await _press(tester, LogicalKeyboardKey.select);
    expect(app.x, 'hero');
    expect(_mark(), _exitRow);
    final first = _now;
    await _press(tester, LogicalKeyboardKey.arrowDown);
    expect(_now, isNot(first));
    expect(_mark(), startsWith('node:'));

    // In the open card the D-pad stays in it: neither the tabs nor the cards beside it.
    await _press(tester, LogicalKeyboardKey.arrowUp, times: 8);
    expect(_now.debugLabel, isNot(startsWith('tab')));
    expect(_now.debugLabel, isNot('cell'));
    expect(_now.context!.findAncestorWidgetOfExactType<FocusMark>()?.id, isNot(anyOf('cell:map', 'cell:mode', 'cell:routes', 'cell:speed', 'cell:apps')));

    await _back(tester);
    expect(app.x, isNull);
    expect(_mark(), 'cell:hero');

    await _back(tester);
    expect(_now.debugLabel, 'tab 0');
    expect(_exited(calls), isFalse);

    await _back(tester);
    expect(_exited(calls), isTrue);
  });

  testWidgets("a route's exits are a level under the routes card, and Back returns to that route", (tester) async {
    final (app, _) = await _open(tester, const Size(960, 540));
    await _press(tester, LogicalKeyboardKey.arrowDown, times: 2);
    await _press(tester, LogicalKeyboardKey.arrowRight);
    await _press(tester, LogicalKeyboardKey.arrowDown);
    expect(_mark(), 'cell:routes');

    await _press(tester, LogicalKeyboardKey.select);
    expect(app.x, 'routes');
    final route = _mark()!;
    expect(route, startsWith('route:'));

    await _press(tester, LogicalKeyboardKey.select);
    expect(app.sub, route.substring(6));
    expect(_mark(), _exitRow);

    await _back(tester);
    expect(app.x, 'routes');
    expect(app.sub, isNull);
    expect(_mark(), route);

    await _back(tester);
    expect(app.x, isNull);
    expect(_mark(), 'cell:routes');
  });

  testWidgets('a sheet holds the D-pad, starts on the option in use, and Back returns to what opened it', (tester) async {
    final (app, _) = await _open(tester, const Size(960, 540));
    await _press(tester, LogicalKeyboardKey.arrowDown, times: 2);
    await _press(tester, LogicalKeyboardKey.select);
    expect(app.x, 'hero');

    // the node strategy, in the card's footer
    final strategy = find.text('自动').last;
    await _focusOn(tester, strategy);
    final opener = _now;
    await _press(tester, LogicalKeyboardKey.select);
    expect(app.sheet, isNotNull);
    expect(_mark(), 'picked');
    expect(_now.context!.findAncestorWidgetOfExactType<SheetHost>(), isNotNull);

    for (final k in [LogicalKeyboardKey.arrowUp, LogicalKeyboardKey.arrowLeft, LogicalKeyboardKey.arrowRight, LogicalKeyboardKey.arrowDown]) {
      await _press(tester, k, times: 4);
      expect(_now.context!.findAncestorWidgetOfExactType<SheetHost>(), isNotNull, reason: '$k');
    }

    await _back(tester);
    expect(app.sheet, isNull);
    expect(app.x, 'hero');
    expect(_now, opener);
  });

  testWidgets('Back leaves a text field before it closes the card', (tester) async {
    final (app, _) = await _open(tester, const Size(960, 540));
    await _press(tester, LogicalKeyboardKey.arrowDown, times: 2);
    await _press(tester, LogicalKeyboardKey.select);
    await _focusOn(tester, find.descendant(of: find.bySemanticsLabel('搜索节点').last, matching: find.byType(GestureDetector)).first);
    await _press(tester, LogicalKeyboardKey.select);
    expect(app.searching, isTrue);
    expect(_now.context!.findAncestorStateOfType<EditableTextState>(), isNotNull);

    await _back(tester);
    expect(app.x, 'hero');
    expect(_now.context!.findAncestorStateOfType<EditableTextState>(), isNull);
    // on the field's box, still in the card
    expect(_now.context!.findAncestorWidgetOfExactType<BareField>(), isNotNull);

    await _back(tester);
    expect(app.x, isNull);
    expect(_mark(), 'cell:hero');
  });

  testWidgets('a 设置 page goes back to the row it was opened from, and the list scrolls to the focus', (tester) async {
    final (app, calls) = await _open(tester, const Size(960, 540));
    await _press(tester, LogicalKeyboardKey.arrowDown);
    await _press(tester, LogicalKeyboardKey.arrowRight, times: 3);
    expect(_now.debugLabel, 'tab 3');
    await _press(tester, LogicalKeyboardKey.select);
    expect(app.tab, 3);
    // a tab's page leaves the focus on the tab
    expect(_now.debugLabel, 'tab 3');

    await tester.scrollUntilVisible(find.text('路由组'), 200, scrollable: find.descendant(of: find.byType(SettingsPage), matching: find.byType(Scrollable)).first);
    await _focusOn(tester, find.text('路由组'));
    final row = _now;
    await _press(tester, LogicalKeyboardKey.select);
    expect(app.settingsNav.currentState!.canPop(), isTrue);
    expect(_now, isNot(row));
    expect(_now.debugLabel, isNot(startsWith('tab')));

    await _back(tester);
    expect(app.settingsNav.currentState!.canPop(), isFalse);
    expect(_now, row);

    // down the list: the focused row is always on screen
    final page = tester.getRect(find.byType(IndexedStack));
    for (var i = 0; i < 12; i++) {
      await _press(tester, LogicalKeyboardKey.arrowDown);
      final r = _now.rect;
      expect(r.top, greaterThanOrEqualTo(page.top - 1), reason: '$i');
      expect(r.bottom, lessThanOrEqualTo(page.bottom + 1), reason: '$i');
    }

    await _back(tester);
    expect(_now.debugLabel, 'tab 3');
    await _back(tester);
    expect(app.tab, 0);
    expect(_now.debugLabel, 'tab 0');
    await _back(tester);
    expect(_exited(calls), isTrue);
  });

  testWidgets('under touch a phone is as it was: no ring, and Back closes the card and leaves', (tester) async {
    final (app, calls) = await _open(tester, const Size(390, 844));
    expect(find.byType(AndroidBar), findsOneWidget);
    await tester.tapAt(tester.getRect(find.byWidgetPredicate((w) => w is FocusMark && w.id == 'cell:hero')).topLeft + const Offset(40, 30));
    await _settle(tester);
    expect(app.x, 'hero');
    expect(FocusManager.instance.highlightMode, FocusHighlightMode.touch);
    expect(find.byWidgetPredicate((w) => w is CustomPaint && w.foregroundPainter?.runtimeType.toString() == '_RingPainter'), findsNothing);

    await tester.binding.handlePopRoute();
    await _settle(tester);
    expect(app.x, isNull);
    await tester.binding.handlePopRoute();
    await _settle(tester);
    expect(_exited(calls), isTrue);
  });
}
