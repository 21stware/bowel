import 'dart:math' as math;

import 'package:bowel/app/state.dart';
import 'package:bowel/main.dart';
import 'package:bowel/ui/atoms.dart';
import 'package:bowel/ui/focus.dart';
import 'package:bowel/ui/skin.dart';
import 'package:bowel/ui/skin_data.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

import 'real_font.dart';

// The focus as a TV's own interface shows it, with no outline anywhere: a
// card or a row lifts, a button inverts. Only with keys driving.

double _lum(Color c) {
  double ch(double v) => v <= .04045 ? v / 12.92 : math.pow((v + .055) / 1.055, 2.4).toDouble();
  return .2126 * ch(c.r) + .7152 * ch(c.g) + .0722 * ch(c.b);
}

Widget _host(Look l, Widget child) => Directionality(
      textDirection: TextDirection.ltr,
      child: Skin(
        look: l,
        child: ColoredBox(color: l.paper, child: Center(child: SizedBox(width: 360, child: child))),
      ),
    );

/// The nearest of [T] round [f]'s widget.
T _around<T extends Widget>(WidgetTester tester, Finder f) => tester.widget<T>(find.ancestor(of: f, matching: find.byType(T)).first);

Color? _fillAround(WidgetTester tester, Finder f) => (_around<AnimatedContainer>(tester, f).decoration as BoxDecoration?)?.color;

Future<void> _settle(WidgetTester tester) async {
  for (var i = 0; i < 4; i++) {
    await tester.pump(const Duration(milliseconds: 100));
  }
}

void main() {
  setUpAll(loadSystemFont);
  tearDown(() => FocusManager.instance.highlightStrategy = FocusHighlightStrategy.automatic);

  test('in every skin, light and dark, a lifted surface is lighter than any it lifts from, and an inverted button reads', () {
    for (final id in skinDefs.keys) {
      for (final dark in [false, true]) {
        final l = Look.of(id, dark);
        final lifted = liftedSurface(l);
        for (final under in [l.paper, l.card]) {
          expect(_lum(lifted), greaterThan(_lum(under)), reason: '$id dark=${l.isDark}');
        }
        expect(_lum(lifted), greaterThanOrEqualTo(_lum(l.sheet)), reason: '$id dark=${l.isDark}');
        expect(contrast(l.paper, l.ink), greaterThanOrEqualTo(4.5), reason: '$id dark=${l.isDark}');
        // a focused lime button stands out from the lime ones beside it, and
        // what's on it reads
        final inv = invertedOf(l, fill: l.accent);
        expect(inv.fill, l.ink, reason: '$id dark=${l.isDark}');
        expect(contrast(inv.fill, inv.on), greaterThanOrEqualTo(4.5), reason: '$id dark=${l.isDark}');
      }
    }
  });

  for (final dark in [false, true]) {
    testWidgets('${dark ? 'dark' : 'light'}: keys on a button invert and grow it; on a row, lift and grow it', (tester) async {
      final l = Look.of('paper', dark);
      await tester.pumpWidget(_host(
        l,
        Column(mainAxisSize: MainAxisSize.min, children: [
          Btn('继续', kind: BtnKind.accent, expand: true, onTap: () {}),
          Tap(onTap: () {}, child: const SizedBox(height: 48, child: Text('一行'))),
          Chip(text: '全部', on: true, onTap: () {}),
          Seg<int>(options: const [(0, '今天'), (1, '7 天')], value: 0, onChanged: (_) {}),
        ]),
      ));
      FocusManager.instance.highlightStrategy = FocusHighlightStrategy.alwaysTraditional;

      Focus.of(tester.element(find.text('继续'))).requestFocus();
      await _settle(tester);
      final inv = invertedOf(l, fill: l.accent);
      expect(inv.fill, l.ink); // the paper skin's ink is no lime
      expect(_fillAround(tester, find.text('继续')), inv.fill);
      final filter = _around<ColorFiltered>(tester, find.text('继续')).colorFilter;
      expect(filter, ColorFilter.mode(inv.on, BlendMode.srcIn));
      expect(_around<AnimatedScale>(tester, find.text('继续')).scale, closeTo(1.07, .001));
      // the row beside it is as it was
      expect(_around<AnimatedScale>(tester, find.text('一行')).scale, 1);

      Focus.of(tester.element(find.text('一行'))).requestFocus();
      await _settle(tester);
      expect(_around<AnimatedScale>(tester, find.text('一行')).scale, closeTo(1.035, .001));
      final surface = tester.widget<AnimatedContainer>(find.descendant(of: find.ancestor(of: find.text('一行'), matching: find.byType(KeyTarget)).first, matching: find.byType(AnimatedContainer)).first).decoration! as BoxDecoration;
      expect(surface.color, liftedSurface(l));
      expect(surface.boxShadow, isNotEmpty);
      expect(_around<AnimatedScale>(tester, find.text('继续')).scale, 1);
      expect(_fillAround(tester, find.text('继续')), l.accent);

      // A chip or segment that's chosen and focused keeps a mark of being
      // chosen; a chosen chip, ink already, inverts to the paper.
      Focus.of(tester.element(find.text('全部'))).requestFocus();
      await _settle(tester);
      expect(_fillAround(tester, find.text('全部')), l.paper);
      expect(find.descendant(of: find.ancestor(of: find.text('全部'), matching: find.byType(AnimatedContainer)).first, matching: find.byType(Positioned)), findsOneWidget);
      Focus.of(tester.element(find.text('7 天'))).requestFocus();
      await _settle(tester);
      expect(_fillAround(tester, find.text('7 天')), l.ink);
      expect(find.descendant(of: find.ancestor(of: find.text('7 天'), matching: find.byType(AnimatedContainer)).first, matching: find.byType(Positioned)), findsNothing);
    });
  }

  testWidgets('under touch, focus changes nothing to see', (tester) async {
    final l = Look.of('paper', false);
    await tester.pumpWidget(_host(
      l,
      Column(mainAxisSize: MainAxisSize.min, children: [
        Btn('继续', kind: BtnKind.accent, onTap: () {}),
        Tap(onTap: () {}, child: const SizedBox(height: 48, child: Text('一行'))),
      ]),
    ));
    FocusManager.instance.highlightStrategy = FocusHighlightStrategy.alwaysTouch;
    for (final t in ['继续', '一行']) {
      Focus.of(tester.element(find.text(t))).requestFocus();
      await _settle(tester);
      expect(find.byWidgetPredicate((w) => w is AnimatedScale && w.scale != 1), findsNothing);
      expect(_fillAround(tester, find.text('继续')), l.accent);
      expect(_around<ColorFiltered>(tester, find.text('继续')).colorFilter, const ColorFilter.mode(Color(0x00000000), BlendMode.dst));
    }
  });

  for (final appearance in ['light', 'dark']) {
    testWidgets('$appearance: on the board a focused card lifts, rounds and grows over its neighbours', (tester) async {
      tester.view.physicalSize = const Size(960, 540);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);
      final app = AppState()..appearance = appearance;
      await tester.pumpWidget(BowelApp(app: app, ready: Future.value()));
      for (var i = 0; i < 16; i++) {
        await tester.pump(const Duration(milliseconds: 100));
      }
      await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
      await _settle(tester);
      await tester.sendKeyEvent(LogicalKeyboardKey.arrowDown);
      await _settle(tester);

      final mark = find.byWidgetPredicate((w) => w is FocusMark && w.id == 'cell:hero');
      final scale = tester.widget<AnimatedScale>(find.descendant(of: mark, matching: find.byType(AnimatedScale)).first);
      expect(scale.scale, closeTo(1.035, .001));
      final l = Look.of('paper', appearance == 'dark');
      final card = tester.widgetList<AnimatedContainer>(find.descendant(of: mark, matching: find.byType(AnimatedContainer))).map((c) => c.decoration).whereType<BoxDecoration>().where((d) => d.color == liftedSurface(l));
      expect(card, isNotEmpty);
      expect(card.any((d) => d.borderRadius != BorderRadius.zero), isTrue);
      // drawn last, over its neighbours
      final cells = tester.widgetList<FocusMark>(find.byWidgetPredicate((w) => w is FocusMark && w.id.startsWith('cell:'))).map((m) => m.id).toList();
      expect(cells.last, 'cell:hero');
      expect(tester.takeException(), isNull);
    });
  }
}
