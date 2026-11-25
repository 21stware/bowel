import 'dart:io';

import 'package:bowel/ui/atoms.dart';
import 'package:bowel/ui/skin.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

/// The system's faces split a line between two fonts, the sans-serif's Latin
/// and another's Chinese, each with its own height and depth. The cards are
/// measured in line heights, so a line that mixes them must still be one line
/// height tall. IBM Plex Mono and OPPO Sans, which the desktop bundles, stand
/// in for a phone's pair: their heights and depths differ as Roboto's and
/// Noto Sans CJK's do.
void main() {
  setUpAll(() async {
    for (final (family, file) in [('latin', 'IBMPlexMono-Regular.ttf'), ('cjk', 'OPPOSans4.0.ttf')]) {
      final bytes = await File('../../resources/fonts/$file').readAsBytes();
      await (FontLoader(family)..addFont(Future.value(ByteData.sublistView(bytes)))).load();
    }
  });

  const lines = ['↑ 212 KB/s · 今日 2.26 GB', '香港 03 IPLC', 'Shadowsocks · 来自 机场 A'];

  // The sizes of the speed and of the exit's name; at a card's smaller lines
  // this pair's difference rounds away.
  for (final (size, lh) in [(20.0, 26.0), (32.0, 40.0)]) {
    testWidgets('a line of Latin and Chinese at $size/$lh is $lh tall', (tester) async {
      final style = Look.of('paper', false).text(size, lh: lh).copyWith(fontFamily: 'latin', fontFamilyFallback: ['cjk']);
      for (final text in lines) {
        await tester.pumpWidget(Directionality(
          textDirection: TextDirection.ltr,
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Text(text, key: const Key('bare'), style: style),
            Line1(text, style, key: const Key('line')),
            Text.rich(TextSpan(style: style, children: [TextSpan(text: text)]), key: const Key('rich'), strutStyle: strutOf(style)),
          ]),
        ));
        // Without the strut the pair does make the line taller, or this test proves nothing.
        expect(tester.getSize(find.byKey(const Key('bare'))).height, greaterThan(lh + .2), reason: text);
        expect(tester.getSize(find.byKey(const Key('line'))).height, closeTo(lh, .01), reason: text);
        expect(tester.getSize(find.byKey(const Key('rich'))).height, closeTo(lh, .01), reason: text);
      }
    });
  }

  test('a style that takes its size or height from around it has no strut', () {
    expect(strutOf(const TextStyle(color: Color(0xFF000000))), isNull);
    expect(strutOf(const TextStyle(fontSize: 12)), isNull);
    expect(strutOf(const TextStyle(fontSize: 12, height: 1.5))?.forceStrutHeight, isTrue);
  });
}
