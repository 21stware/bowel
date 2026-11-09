import 'package:bowel/app/state.dart';
import 'package:bowel/board/cards.dart';
import 'package:bowel/board/fly.dart';
import 'package:bowel/ui/skin.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

Widget _hero(AppState app, double height) => Directionality(
      textDirection: TextDirection.ltr,
      child: Skin(
        look: Look.of('paper', false),
        child: Align(
          alignment: Alignment.topLeft,
          child: SizedBox(
            width: 390,
            height: height,
            child: FlyHost(
              child: CellMark(
                child: CellScope(
                  cell: 'hero',
                  shown: true,
                  child: AnimatedBuilder(animation: app, builder: (context, _) => cardView(context, app, 'hero', 'expanded', 'wide')),
                ),
              ),
            ),
          ),
        ),
      ),
    );

void main() {
  test('tapping the tab you are on closes the open card', () {
    final app = AppState()
      ..x = 'routes'
      ..sub = 'ai';
    app.selectTab(0);
    expect(app.x, isNull);
    expect(app.sub, isNull);
    expect(app.tab, 0);
  });

  test('tapping the tab you are on with nothing open changes nothing', () {
    final app = AppState();
    var told = 0;
    app.addListener(() => told++);
    app.selectTab(0);
    expect(told, 0);
  });

  testWidgets('the big switch turns the tunnel off and on and the card stays open', (tester) async {
    final app = AppState()..x = 'hero';
    await tester.pumpWidget(_hero(app, 473));

    await tester.tap(find.text('关闭 VPN'));
    await tester.pump();
    expect(find.text('正在断开'), findsOneWidget);
    await tester.pump(const Duration(seconds: 1));
    expect(app.on, isFalse);
    expect(app.x, 'hero');

    await tester.tap(find.text('开启 VPN'));
    await tester.pump(const Duration(seconds: 1));
    expect(app.on, isTrue);
    expect(app.x, 'hero');
  });

  testWidgets('a problem offers the reconnect instead', (tester) async {
    final app = AppState()
      ..x = 'hero'
      ..on = false
      ..issue = true;
    await tester.pumpWidget(_hero(app, 473));
    expect(find.text('重新接管'), findsOneWidget);
    expect(find.text('开启 VPN'), findsNothing);
    await tester.tap(find.text('重新接管'));
    await tester.pump(const Duration(seconds: 1));
    expect(app.on, isTrue);
    expect(app.issue, isFalse);
  });

  testWidgets('a short card keeps its room for the node list', (tester) async {
    final app = AppState()..x = 'hero';
    await tester.pumpWidget(_hero(app, 200));
    expect(find.text('关闭 VPN'), findsNothing);
  });

  testWidgets('the switch sits at the very bottom of the card', (tester) async {
    final app = AppState()..x = 'hero';
    await tester.pumpWidget(_hero(app, 473));
    final button = tester.getRect(find.text('关闭 VPN'));
    final collapse = tester.getRect(find.bySemanticsLabel('收起'));
    expect(button.top, greaterThan(collapse.bottom));
  });
}
