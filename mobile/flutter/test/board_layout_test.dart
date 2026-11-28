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
    expect(windowFormOf(const Size(1000, 519)), WindowForm.phone); // too short for the grids
    expect(windowFormOf(const Size(960, 540)), WindowForm.wide); // a 1080p TV
    expect(windowFormOf(const Size(915, 412)), WindowForm.phone); // a big phone on its side
  });

  test('only a TV-shaped window keeps off the edges', () {
    expect(overscanOf(const Size(960, 540)), const EdgeInsets.symmetric(horizontal: 24, vertical: 16));
    expect(overscanOf(const Size(1280, 720)), const EdgeInsets.symmetric(horizontal: 32, vertical: 22));
    expect(overscanOf(const Size(844, 390)), EdgeInsets.zero); // a phone on its side
    expect(overscanOf(const Size(1194, 834)), EdgeInsets.zero); // a tablet
  });

  test('only a TV-shaped window shorter than 720 is drawn at 720 and scaled down', () {
    expect(canvasScaleOf(const Size(960, 540)), .75);
    expect(canvasScaleOf(const Size(1280, 720)), 1);
    expect(canvasScaleOf(const Size(1920, 1080)), 1);
    expect(canvasScaleOf(const Size(844, 390)), 1); // a phone on its side
    expect(canvasScaleOf(const Size(1194, 834)), 1); // a tablet
  });

  // A TV's 960x540 drawn at 1280x720 (the board a desktop window has room
  // for), and the 960x540 itself, less the margins and the tabs along the top.
  for (final size in [const Size(1216, 626), const Size(912, 458)]) {
    test('a TV board of $size holds every card, opened or not', () {
      for (final x in [null, ..._ids]) {
        final cells = boardLayout(WindowForm.wide, x, size.height, size.width);
        final bottom = cells.values.map((r) => r.bottom).reduce((p, n) => p > n ? p : n);
        expect(bottom, closeTo(size.height, .01), reason: '$x');
        if (x == null) {
          expect(cells['hero']!.height, greaterThanOrEqualTo(146 - 1e-9));
          expect(cells['routes']!.height, greaterThanOrEqualTo(166 - 1e-9));
          expect(cells['speed']!.height, greaterThanOrEqualTo(104 - 1e-9));
        } else {
          // room for the opened card's switch
          expect(cells[x]!.height, greaterThanOrEqualTo(300), reason: x);
        }
      }
    });
  }

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
