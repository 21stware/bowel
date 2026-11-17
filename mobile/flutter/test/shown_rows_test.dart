import 'dart:convert';

import 'package:bowel/app/i18n.dart';
import 'package:bowel/app/sample.dart';
import 'package:bowel/app/shown.dart';
import 'package:bowel/app/state.dart';
import 'package:bowel/board/cards.dart';
import 'package:bowel/board/fly.dart';
import 'package:bowel/pages/frame.dart';
import 'package:bowel/pages/routing.dart';
import 'package:bowel/pages/sheet.dart';
import 'package:bowel/ui/skin.dart';
import 'package:flutter/material.dart' show DefaultMaterialLocalizations, Material, MaterialType;
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

// The core's separator between a node's name and its Provider's id (core.NodeSep).
const _sep = '‹';

/// A node as the bridge sends it: its test of the default route's target, and
/// of each other route's.
Map _node(String name, {int delay = 120, bool tested = true, Map<String, int>? routes = const {}}) => {
      'name': '$name${_sep}p1',
      'display': name,
      'provider': 'p1',
      'providerName': '机场 A',
      'type': 'ss',
      'delay': delay,
      'tested': tested,
      'routes': routes,
      'where': {'country': 'JP', 'city': ''},
      'source': '按名称',
      'h': [3],
    };

List<Map> _nodes = [];

Map _view() => {
      'vpn': 'on',
      'mode': 'rule',
      'nodes': _nodes,
      'providers': [
        {'id': 'p1', 'name': '机场 A', 'kind': 'subscription', 'nodes': _nodes.length, 'enabled': true, 'updated': 0},
      ],
      'routes': [
        {'id': 'default', 'name': '默认', 'node': '东京 01${_sep}p1', 'auto': true},
        {'id': 'ai', 'name': 'AI 服务', 'node': '东京 01${_sep}p1', 'auto': true},
      ],
    };

/// What the fake core was told is on screen, call by call, and how often it
/// was asked for the routing pages' data.
final _viewing = <Map>[];
int _routingAsked = 0;

void _fakeCore() {
  TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger.setMockMethodCallHandler(const MethodChannel('bowel/core'), (call) async {
    if (call.method != 'call') return null;
    final method = call.arguments['method'] as String;
    switch (method) {
      case 'view':
        return jsonEncode(_view());
      case 'viewing':
        _viewing.add(jsonDecode(call.arguments['payload'] as String) as Map);
      case 'routing':
        _routingAsked++;
        return jsonEncode(_routing());
    }
    return 'null';
  });
}

/// The routing pages' data: AI 服务 fixed on no node yet, with one other site.
Map _routing() => {
      'groups': [
        {
          'id': 'ai',
          'name': 'AI 服务',
          'direct': false,
          'reject': false,
          'custom': false,
          'movable': true,
          'chooses': true,
          'mode': 'fixed',
          'regions': <String>[],
          'providers': <String>[],
          'order': <String>[],
          'fixed': '',
          'now': '',
          'services': <Map>[],
          'targets': [
            {'name': 'gstatic', 'url': 'https://www.gstatic.com/generate_204'},
            {'name': 'OpenAI', 'url': 'https://chatgpt.com'},
          ],
          'sites': [
            {'name': 'OpenAI', 'url': 'https://chatgpt.com', 'delay': 0, 'tested': false},
          ],
        },
      ],
      'nodes': [
        {'name': '东京 01${_sep}p1', 'display': '东京 01', 'provider': '机场 A', 'providerId': 'p1', 'country': 'JP', 'flag': 'jp', 'delay': 120, 'routes': <String, int>{}},
        {'name': '香港 02${_sep}p1', 'display': '香港 02', 'provider': '机场 A', 'providerId': 'p1', 'country': 'HK', 'flag': 'hk', 'delay': 300, 'routes': {'ai': 90}},
      ],
      'regions': <Map>[],
      'providers': [
        {'id': 'p1', 'name': '机场 A'},
      ],
      'kinds': <Map>[],
    };

Widget _page(AppState app, Widget page) => Localizations(
      locale: const Locale('en'),
      delegates: const [DefaultMaterialLocalizations.delegate, DefaultWidgetsLocalizations.delegate],
      child: MediaQuery(
        data: const MediaQueryData(size: Size(360, 760)),
        child: Skin(
          look: Look.of('paper', false),
          child: Frame(
            data: const FrameData(top: 0, barInset: 0, nativeBar: false, homeIndicator: 0, form: WindowForm.phone),
            child: Navigator(
              onGenerateRoute: (_) => PageRouteBuilder<void>(
                pageBuilder: (_, _, _) => Material(
                  type: MaterialType.transparency,
                  child: AnimatedBuilder(
                    animation: app,
                    builder: (context, _) => Stack(fit: StackFit.expand, children: [page, SheetHost(app: app)]),
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );

Future<void> _settle(WidgetTester tester) async {
  await tester.pump();
  await tester.pump(const Duration(milliseconds: 600));
  await tester.pump(const Duration(milliseconds: 600));
}

Future<void> _push(WidgetTester tester) async {
  final msg = const StandardMethodCodec().encodeMethodCall(MethodCall('emit', {'name': 'view', 'payload': jsonEncode(_view())}));
  await tester.binding.defaultBinaryMessenger.handlePlatformMessage('bowel/core', msg, (_) {});
}

Future<AppState> _attached() async {
  final app = AppState();
  await app.attachCore();
  addTearDown(app.dispose);
  return app;
}

Widget _row(AppState app, {String node = '', required String route, bool front = true}) =>
    TickerMode(enabled: front, child: Shown(app: app, node: node, route: route, child: const SizedBox()));

Widget _card(AppState app) => Directionality(
      textDirection: TextDirection.ltr,
      child: Skin(
        look: Look.of('paper', false),
        child: Align(
          alignment: Alignment.topLeft,
          child: SizedBox(
            width: 390,
            height: 600,
            child: FlyHost(
              child: CellMark(
                child: CellScope(cell: 'routes', shown: true, child: AnimatedBuilder(animation: app, builder: (context, _) => cardView(context, app, 'routes', 'expanded', 'wide'))),
              ),
            ),
          ),
        ),
      ),
    );

const _tokyo = {'node': '东京 01${_sep}p1', 'route': 'ai'};

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUp(() {
    _nodes = [
      _node('东京 01', routes: {'ai': 700}),
      _node('香港 02', delay: 300, routes: {'ai': 90}),
    ];
    _viewing.clear();
    _routingAsked = 0;
    language = 'zh';
    _fakeCore();
  });

  test('a node reads its delay on the route it is shown for', () {
    const n = Node('n', '', '', '', '', '', 88, [], routes: {'ai': 0});
    expect(n.on('default'), 88);
    expect(n.on('ai'), 0, reason: 'it failed the target of AI 服务');
    expect(n.on('media'), -1, reason: 'never tested there');
    // Under a config's own routing no target is bowel's: one delay for every route.
    const own = Node('n', '', '', '', '', '', 88, []);
    expect(own.on('ai'), 88);
  });

  testWidgets("the core's nodes carry each route's delay, and say when they are not tested", (tester) async {
    _nodes = [
      _node('东京 01', routes: {'ai': 700}),
      _node('香港 02', delay: 0, tested: false), // a failure too old to count: bars, and no test
      _node('大阪 03', routes: null),
    ];
    await _attached();
    expect(node('东京 01').delay, 120);
    expect(node('东京 01').on('ai'), 700);
    expect(node('东京 01').on('media'), -1);
    expect(node('香港 02').delay, -1);
    expect(node('大阪 03').on('ai'), 120);
  });

  testWidgets('a row on screen is reported by its name in the core, and withdrawn when it goes', (tester) async {
    final app = await _attached();
    await tester.pumpWidget(_row(app, node: '东京 01', route: 'ai'));
    await tester.pump();
    expect(_viewing.last, {'nodes': [_tokyo], 'sites': ''});

    await tester.pumpWidget(const SizedBox());
    await tester.pump();
    expect(_viewing.last, {'nodes': <Map>[], 'sites': ''});
  });

  testWidgets('a frame of rows is one call', (tester) async {
    final app = await _attached();
    await tester.pumpWidget(Column(children: [_row(app, node: '东京 01', route: 'ai'), _row(app, node: '香港 02', route: 'ai')]));
    await tester.pump();
    expect(_viewing, hasLength(1));
    expect(_viewing.single['nodes'], hasLength(2));
    await tester.pump();
    expect(_viewing, hasLength(1), reason: 'nothing changed');
  });

  testWidgets('a page behind another tab is not on screen', (tester) async {
    final app = await _attached();
    await tester.pumpWidget(_row(app, node: '东京 01', route: 'ai', front: false));
    await tester.pump();
    expect(_viewing, isEmpty);

    await tester.pumpWidget(_row(app, node: '东京 01', route: 'ai'));
    await tester.pump();
    expect(_viewing.last['nodes'], [_tokyo]);

    await tester.pumpWidget(_row(app, node: '东京 01', route: 'ai', front: false));
    await tester.pump();
    expect(_viewing.last['nodes'], isEmpty);
  });

  testWidgets('nothing is on screen while the app is in the background', (tester) async {
    final app = await _attached();
    await tester.pumpWidget(_row(app, node: '东京 01', route: 'ai'));
    await tester.pump();

    // The states an app goes through on its way out, and back.
    const away = [AppLifecycleState.inactive, AppLifecycleState.hidden, AppLifecycleState.paused];
    away.forEach(tester.binding.handleAppLifecycleStateChanged);
    await tester.pump();
    expect(_viewing.last['nodes'], isEmpty);

    const back = [AppLifecycleState.hidden, AppLifecycleState.inactive, AppLifecycleState.resumed];
    back.forEach(tester.binding.handleAppLifecycleStateChanged);
    await tester.pump();
    expect(_viewing.last['nodes'], [_tokyo]);
  });

  testWidgets("a route's other sites are on screen with its page, and read again when the core has news", (tester) async {
    final app = await _attached();
    await tester.pumpWidget(_row(app, route: 'ai'));
    await tester.pump();
    expect(_viewing.last, {'nodes': <Map>[], 'sites': 'ai'});

    final before = _routingAsked;
    await _push(tester);
    await tester.pump();
    expect(_routingAsked, before + 1);

    await tester.pumpWidget(const SizedBox());
    await tester.pump();
    expect(_viewing.last['sites'], '');
    await _push(tester);
    await tester.pump();
    expect(_routingAsked, before + 1, reason: 'no page is waiting for them');
  });

  testWidgets("choosing a route's exit lists each node by that route's own test", (tester) async {
    final app = await _attached();
    app
      ..x = 'routes'
      ..sub = 'ai';
    await tester.pumpWidget(_card(app));
    await tester.pump(const Duration(seconds: 1));

    expect(find.text('700 ms'), findsOneWidget);
    expect(find.text('90 ms'), findsOneWidget);
    expect(find.text('120 ms'), findsNothing, reason: "the default route's delay");
    expect(tester.getTopLeft(find.text('90 ms')).dy, lessThan(tester.getTopLeft(find.text('700 ms')).dy), reason: 'fastest on this route first');
    expect(_viewing.last['nodes'], unorderedEquals([_tokyo, {'node': '香港 02${_sep}p1', 'route': 'ai'}]));

    app.subBack();
    await tester.pump(const Duration(seconds: 1));
    expect(_viewing.last['nodes'], isEmpty);
    // Both routes are on 东京 01, which is slow on the target of AI 服务.
    expect(find.text('120 ms · 0%'), findsOneWidget);
    expect(find.text('700 ms · 0%'), findsOneWidget);
  });

  testWidgets("a route group's page reads nodes on the group's own target, and has its other sites kept tested", (tester) async {
    tester.view.physicalSize = const Size(360, 760);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final app = await _attached();
    await tester.pumpWidget(_page(app, GroupPage(app: app, id: 'ai')));
    await _settle(tester);
    expect(find.text('700 ms'), findsOneWidget, reason: 'the node the group is on');
    expect(_viewing.last, {'nodes': <Map>[], 'sites': 'ai'});

    await tester.tap(find.text('未选择'));
    await _settle(tester);
    expect(find.text('机场 A · 90 ms'), findsOneWidget);
    expect(find.text('机场 A · 300 ms'), findsNothing, reason: "the default route's delay");
    expect(find.text('机场 A'), findsOneWidget, reason: '东京 01 has no test on this target');
  });
}
