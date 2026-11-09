import 'package:bowel/board/fly.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

const _rest = Rect.fromLTWH(0, 100, 200, 100);

void main() {
  testWidgets('a flight ends where the text will rest, not where its entrance starts', (tester) async {
    final enter = AnimationController(vsync: tester, duration: const Duration(seconds: 1), value: 1);
    addTearDown(enter.dispose);
    final host = GlobalKey<FlyHostState>();
    var moved = false;
    late StateSetter update;

    await tester.pumpWidget(
      Directionality(
        textDirection: TextDirection.ltr,
        child: FlyHost(
          key: host,
          child: StatefulBuilder(
            builder: (context, setState) {
              update = setState;
              return Stack(children: [
                Positioned(
                  left: 0,
                  top: moved ? _rest.top : 0,
                  width: _rest.width,
                  height: _rest.height,
                  child: CellMark(
                    child: CellScope(
                      cell: 'a',
                      shown: true,
                      child: EntranceScope(
                        notifier: enter,
                        child: Unroll(
                          child: FlyTag(
                            id: 't',
                            text: 'label',
                            style: const TextStyle(fontSize: 14),
                            build: (style) => Text('label', style: style),
                          ),
                        ),
                      ),
                    ),
                  ),
                ),
              ]);
            },
          ),
        ),
      ),
    );

    host.currentState!.morph(() {
      update(() => moved = true);
      enter.forward(from: 0);
    }, finalCell: (_) => _rest);
    await tester.pump();
    await tester.pump();
    // The entrance is over by now and the flight is a few milliseconds short of landing.
    await tester.pump(const Duration(milliseconds: 555));

    final copies = find.text('label').evaluate().toList();
    expect(copies, hasLength(2), reason: 'the resting text and the flight');
    final tops = [for (final e in copies) (e.findRenderObject() as RenderBox).localToGlobal(Offset.zero).dy];
    expect(tops[0], closeTo(tops[1], .5));

    await tester.pump(const Duration(seconds: 1));
  });
}
