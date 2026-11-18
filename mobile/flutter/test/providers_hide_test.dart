import 'dart:convert';

import 'package:bowel/app/i18n.dart';
import 'package:bowel/app/state.dart';
import 'package:bowel/pages/frame.dart';
import 'package:bowel/pages/providers.dart';
import 'package:bowel/pages/sheet.dart';
import 'package:bowel/ui/skin.dart';
import 'package:flutter/material.dart' show DefaultMaterialLocalizations, Material, MaterialType;
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

// The core's separator between a node's name and its Provider's id (core.NodeSep).
const _sep = '​‹';

Map _node(String name) => {
      'name': '$name${_sep}p1',
      'display': name,
      'provider': 'p1',
      'providerName': '机场 A',
      'type': 'ss',
      'delay': 120,
      'where': {'country': 'JP', 'city': ''},
      'source': '按名称',
      'h': [120],
    };

/// The nodes p1 still has and the ones removed from it, as the bridge keeps them.
List<String> _shown = [];
List<String> _hidden = [];
final _calls = <(String, Map)>[];

/// What the core was last told is on screen.
Map _viewing = {};

Map _view() => {
      'vpn': 'off',
      'mode': 'rule',
      'nodes': [for (final n in _shown) _node(n)],
      'providers': [
        {'id': 'p1', 'name': '机场 A', 'kind': 'subscription', 'nodes': _shown.length, 'enabled': true, 'updated': 0, if (_hidden.isNotEmpty) 'hidden': _hidden},
      ],
      'routes': <Map>[],
    };

Future<void> _push(WidgetTester tester) async {
  final msg = const StandardMethodCodec().encodeMethodCall(MethodCall('emit', {'name': 'view', 'payload': jsonEncode(_view())}));
  await tester.binding.defaultBinaryMessenger.handlePlatformMessage('bowel/core', msg, (_) {});
}

void _fakeCore() {
  TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger.setMockMethodCallHandler(const MethodChannel('bowel/core'), (call) async {
    if (call.method != 'call') return null;
    final method = call.arguments['method'] as String;
    final args = jsonDecode(call.arguments['payload'] as String) as Map;
    switch (method) {
      case 'view':
        return jsonEncode(_view());
      case 'viewing':
        _viewing = args;
      case 'hideNode':
        _calls.add((method, args));
        final name = (args['node'] as String).split(_sep).first;
        _shown.remove(name);
        _hidden.add(name);
        return jsonEncode(['AI 服务']);
      case 'restoreNode':
        _calls.add((method, args));
        _hidden.remove(args['name']);
        _shown.add(args['name'] as String);
        return 'null';
    }
    return 'null';
  });
}

Widget _host(AppState app) => Localizations(
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
                    builder: (context, _) => Stack(fit: StackFit.expand, children: [ProvidersPage(app: app), SheetHost(app: app)]),
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

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUp(() {
    _shown = ['东京 01', '香港 02'];
    _hidden = [];
    _calls.clear();
    _viewing = {};
    language = 'zh';
    _fakeCore();
  });

  testWidgets('a node is removed from its row and put back from the removed list', (tester) async {
    tester.view.physicalSize = const Size(360, 760);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final app = AppState();
    await app.attachCore();
    await tester.pumpWidget(_host(app));
    await _settle(tester);

    await tester.tap(find.text('机场 A').first);
    await _settle(tester);
    expect(find.text('东京 01'), findsOneWidget);
    expect(find.textContaining('已移除的节点'), findsNothing);

    await tester.tap(find.text('东京 01'));
    await _settle(tester);
    await tester.tap(find.text('移除这个节点'));
    await _settle(tester);
    expect(_calls.single.$1, 'hideNode');
    expect(_calls.single.$2, {'node': '东京 01${_sep}p1'});
    expect(app.toast, '已移除 东京 01，订阅更新后也不会回来。手动选了它的路由组：AI 服务');

    await _push(tester);
    await _settle(tester);
    expect(find.text('东京 01'), findsOneWidget, reason: 'only in the removed list now');
    expect(find.text('已移除的节点 · 1'), findsOneWidget);
    expect(find.text('香港 02'), findsOneWidget);

    await tester.tap(find.text('放回'));
    await _settle(tester);
    expect(_calls.last.$1, 'restoreNode');
    expect(_calls.last.$2, {'id': 'p1', 'name': '东京 01'});

    await _push(tester);
    await _settle(tester);
    expect(find.textContaining('已移除的节点'), findsNothing);
    expect(find.text('东京 01'), findsOneWidget);
    await tester.pump(const Duration(seconds: 6)); // let the toast's timer end
  });

  testWidgets("an open Provider's nodes are on screen, on the default route's target", (tester) async {
    tester.view.physicalSize = const Size(360, 760);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final app = AppState();
    await app.attachCore();
    await tester.pumpWidget(_host(app));
    await _settle(tester);
    expect(_viewing['nodes'] ?? const [], isEmpty, reason: 'closed, it shows no node');

    await tester.tap(find.text('机场 A').first);
    await _settle(tester);
    expect(_viewing['nodes'], unorderedEquals([
      {'node': '东京 01${_sep}p1', 'route': 'default'},
      {'node': '香港 02${_sep}p1', 'route': 'default'},
    ]));

    await tester.tap(find.text('机场 A').first);
    await _settle(tester);
    expect(_viewing['nodes'], isEmpty);
  });

  for (final lang in ['zh', 'en', 'ja', 'zh-Hant']) {
    testWidgets('$lang: the remove sheet and the removed list fit a narrow phone', (tester) async {
      tester.view.physicalSize = const Size(320, 640);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);
      addTearDown(() => language = 'zh');
      _hidden = ['一个名字特别特别长的节点 · 美国 洛杉矶 CN2 GIA 专线 01'];
      final app = AppState();
      await app.attachCore();
      language = lang;
      await tester.pumpWidget(_host(app));
      await _settle(tester);
      await tester.tap(find.text('机场 A').first);
      await _settle(tester);
      expect(find.text(tr('放回', 'Restore')), findsOneWidget);
      await tester.tap(find.text('香港 02'));
      await _settle(tester);
      expect(find.text(tr('移除这个节点', 'Remove this node')), findsOneWidget);
      expect(tester.takeException(), isNull);
    });
  }
}
