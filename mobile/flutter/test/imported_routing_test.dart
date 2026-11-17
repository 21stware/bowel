import 'dart:convert';

import 'package:bowel/app/i18n.dart';
import 'package:bowel/app/state.dart';
import 'package:bowel/pages/frame.dart';
import 'package:bowel/pages/providers.dart';
import 'package:bowel/pages/routing.dart';
import 'package:bowel/pages/sheet.dart';
import 'package:bowel/ui/skin.dart';
import 'package:flutter/material.dart' show DefaultMaterialLocalizations, Material, MaterialType;
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

// A provider's own routing (issue #184): 使用它们 on its row, and 路由组 while
// it is in use.

const _sep = '​‹';

bool _inUse = false;
final _calls = <(String, Map)>[];

Map _view() => {
      'vpn': 'off',
      'mode': 'rule',
      'nodes': <Map>[],
      'providers': [
        {
          'id': 'p1',
          'name': '机场 A',
          'kind': 'subscription',
          'nodes': 2,
          'enabled': true,
          'updated': 0,
          'routing': {'groups': 2, 'rules': 3, 'remote': 1, if (_inUse) 'inUse': true},
        },
      ],
      'routes': <Map>[],
      if (_inUse) 'imported': {'provider': 'p1', 'name': '机场 A', 'rules': 3, 'pausedRules': 1, 'pausedSets': 0, 'unused': <String>[]},
    };

Map _routing() => {
      'groups': <Map>[],
      'nodes': <Map>[],
      'regions': <Map>[],
      'providers': <Map>[],
      'kinds': <Map>[],
      'imported': {
        'provider': 'p1',
        'name': '机场 A',
        'rules': 3,
        'pausedRules': 1,
        'pausedSets': 2,
        'unused': ['dns', 'mixed-port'],
        'groups': [
          {
            'name': 'Japan',
            'kind': 'url-test',
            'route': 'default',
            'rules': 1,
            'now': 'JP-02',
            'exit': 'JP-02${_sep}p1',
            'selectable': false,
            'members': [
              {'name': 'JP-02', 'node': 'JP-02${_sep}p1', 'delay': 88, 'tested': true},
              {'name': 'JP-01', 'node': 'JP-01${_sep}p1', 'delay': 0, 'tested': true},
            ],
          },
          {
            'name': 'Streaming',
            'kind': 'select',
            'route': 'g1',
            'rules': 1,
            'now': 'Japan',
            'exit': 'JP-02${_sep}p1',
            'selectable': true,
            'members': [
              {'name': 'Japan', 'group': true, 'delay': 0, 'tested': false},
              {'name': 'JP-01', 'node': 'JP-01${_sep}p1', 'delay': 0, 'tested': true},
              {'name': 'DIRECT', 'node': 'DIRECT', 'delay': 0, 'tested': false},
            ],
          },
        ],
      },
    };

Future<void> _push(WidgetTester tester) async {
  final msg = const StandardMethodCodec().encodeMethodCall(MethodCall('emit', {'name': 'view', 'payload': jsonEncode(_view())}));
  await tester.binding.defaultBinaryMessenger.handlePlatformMessage('bowel/core', msg, (_) {});
}

void _fakeCore() {
  TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger.setMockMethodCallHandler(const MethodChannel('bowel/core'), (call) async {
    if (call.method != 'call') return null;
    final method = call.arguments['method'] as String;
    final payload = call.arguments['payload'] as String?;
    final args = payload == null || payload.isEmpty ? const {} : jsonDecode(payload) as Map;
    switch (method) {
      case 'view':
        return jsonEncode(_view());
      case 'routing':
        return jsonEncode(_routing());
      case 'useRouting':
      case 'selectMember':
        _calls.add((method, args));
        if (method == 'useRouting') _inUse = (args['id'] as String).isNotEmpty;
        return 'null';
    }
    return 'null';
  });
}

Widget _host(AppState app, Widget Function() page) => Localizations(
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
                    builder: (context, _) => Stack(fit: StackFit.expand, children: [page(), SheetHost(app: app)]),
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
    _inUse = false;
    _calls.clear();
    language = 'zh';
    _fakeCore();
  });

  tearDown(() => language = 'zh');

  testWidgets('a provider with groups offers 使用它们, and the way back once in use', (tester) async {
    tester.view.physicalSize = const Size(360, 760);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final app = AppState();
    await app.attachCore();
    await tester.pumpWidget(_host(app, () => ProvidersPage(app: app)));
    await _settle(tester);

    expect(find.text('这份配置带 2 个路由组、3 条规则'), findsOneWidget);
    await tester.tap(find.text('使用它们'));
    await _settle(tester);
    expect(find.text('按它的路由分流'), findsOneWidget);
    expect(find.textContaining('自带 1 个订阅'), findsOneWidget);
    await tester.tap(find.text('使用它们').last);
    await _settle(tester);
    expect(_calls.single.$1, 'useRouting');
    expect(_calls.single.$2, {'id': 'p1'});

    await _push(tester);
    await _settle(tester);
    expect(find.text('正在使用它的 2 个路由组'), findsOneWidget);
    await tester.tap(find.text('改回 bowel 路由'));
    await _settle(tester);
    expect(_calls.last.$2, {'id': ''});
    await tester.pump(const Duration(seconds: 3)); // let the toast's timer end
  });

  testWidgets('路由组 shows the config\'s groups, and switches only the ones chosen by hand', (tester) async {
    tester.view.physicalSize = const Size(360, 760);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final app = AppState()..routing = _routing();
    await tester.pumpWidget(_host(app, () => RoutesPage(app: app)));
    await _settle(tester);

    expect(find.text('路由组 · 来自这份配置'), findsOneWidget);
    expect(find.textContaining('dns、mixed-port 不会生效'), findsOneWidget);
    expect(find.textContaining('你的 1 条规则和 2 个规则集已暂停'), findsOneWidget);
    expect(find.text('Japan → JP-02$_sep' 'p1'), findsOneWidget);

    await tester.tap(find.text('Japan').first);
    await _settle(tester);
    expect(app.toast, '这个组是「自动最快」，由内核自己选择');
    expect(_calls, isEmpty);

    await tester.tap(find.text('Streaming'));
    await _settle(tester);
    await tester.tap(find.text('DIRECT'));
    await _settle(tester);
    expect(_calls.single.$1, 'selectMember');
    expect(_calls.single.$2, {'name': 'Streaming', 'node': 'DIRECT'});
    await tester.pump(const Duration(seconds: 3));
  });

  for (final lang in ['zh', 'en', 'ja', 'zh-Hant']) {
    testWidgets('$lang: the imported 路由组 and the provider row fit a narrow phone', (tester) async {
      tester.view.physicalSize = const Size(320, 640);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);
      final app = AppState()..routing = _routing();
      await app.attachCore();
      language = lang;
      await tester.pumpWidget(_host(app, () => RoutesPage(app: app)));
      await _settle(tester);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
      await tester.pumpWidget(_host(app, () => ProvidersPage(app: app)));
      await _settle(tester);
      expect(find.text(tr('使用它们', 'Use them')), findsOneWidget);
      expect(tester.takeException(), isNull);
      _inUse = true;
      await _push(tester);
      await _settle(tester);
      expect(find.text(tr('改回 bowel 路由', "Use bowel's routing")), findsOneWidget);
      expect(tester.takeException(), isNull);
    });
  }
}
