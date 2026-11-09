import 'package:bowel/board/board.dart';
import 'package:bowel/pages/frame.dart';
import 'package:flutter/painting.dart';
import 'package:flutter_test/flutter_test.dart';

const _ids = ['map', 'speed', 'apps', 'routes', 'mode', 'hero'];

void main() {
  test('window form follows the room there is, not the device', () {
    expect(windowFormOf(const Size(390, 844)), WindowForm.phone);
    expect(windowFormOf(const Size(844, 390)), WindowForm.phone); // a phone on its side
    expect(windowFormOf(const Size(507, 1194)), WindowForm.phone); // half of a split iPad
    expect(windowFormOf(const Size(744, 1133)), WindowForm.medium);
    expect(windowFormOf(const Size(899, 1200)), WindowForm.medium);
    expect(windowFormOf(const Size(900, 600)), WindowForm.wide);
    expect(windowFormOf(const Size(1376, 1032)), WindowForm.wide);
    expect(windowFormOf(const Size(1000, 599)), WindowForm.phone); // too short for the grids
  });

  for (final (form, size) in [
    (WindowForm.medium, const Size(790, 490)), // a 600pt-tall window, less its bars
    (WindowForm.medium, const Size(790, 640)),
    (WindowForm.wide, const Size(1000, 490)),
  ]) {
    test('$form ${size.width}x${size.height}: short rows still hold their cards', () {
      final cells = boardLayout(form, null, size.height, size.width);
      expect(cells['hero']!.height, greaterThanOrEqualTo(146));
      expect(cells['routes']!.height, greaterThanOrEqualTo(166));
      expect(cells['speed']!.height, greaterThanOrEqualTo(104));
      final bottom = cells.values.map((r) => r.bottom).reduce((p, n) => p > n ? p : n);
      expect(bottom, closeTo(size.height, .01));
    });
  }

  for (final (form, size) in [
    (WindowForm.medium, const Size(744, 1000)),
    (WindowForm.wide, const Size(1194, 784)),
    (WindowForm.wide, const Size(1032, 1250)),
  ]) {
    for (final x in [null, ..._ids]) {
      test('$form ${size.width}x${size.height} with ${x ?? 'nothing'} open: every card fits, none overlap', () {
        final box = Offset.zero & size;
        final cells = boardLayout(form, x, size.height, size.width);
        expect(cells.keys.toSet(), _ids.toSet());
        for (final a in cells.entries) {
          expect(a.value.width, greaterThan(0), reason: a.key);
          expect(a.value.height, greaterThan(0), reason: a.key);
          expect(box.deflate(-.01).contains(a.value.topLeft), isTrue, reason: a.key);
          expect(a.value.right, lessThanOrEqualTo(size.width + .01), reason: a.key);
          expect(a.value.bottom, lessThanOrEqualTo(size.height + .01), reason: a.key);
          for (final b in cells.entries) {
            if (a.key == b.key) continue;
            final both = a.value.intersect(b.value);
            expect(both.width > .01 && both.height > .01, isFalse, reason: '${a.key} / ${b.key}');
          }
        }
        // The rows reach the bottom edge, with the 1pt gaps showing the board.
        final bottom = cells.values.map((r) => r.bottom).reduce((p, n) => p > n ? p : n);
        expect(bottom, closeTo(size.height, .01));
        if (x != null) {
          for (final other in cells.entries) {
            expect(cells[x]!.height, greaterThanOrEqualTo(other.value.height), reason: other.key);
          }
        }
      });
    }
  }
}
