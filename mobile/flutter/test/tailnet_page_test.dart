import 'dart:convert';

import 'package:bowel/app/i18n.dart';
import 'package:bowel/app/state.dart';
import 'package:bowel/pages/frame.dart';
import 'package:bowel/pages/sheet.dart';
import 'package:bowel/pages/tailnet.dart';
import 'package:bowel/ui/skin.dart';
import 'package:flutter/material.dart' show DefaultMaterialLocalizations, Material, MaterialType;
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

/// The settings' tailnet and the node's state, as the bridge gives them.
Map _tailnet = {};
Map _state = {};

/// What the page asked of the bridge and of the shell.
final _patches = <Map>[];
final _opened = <Map>[];

void _fake() {
  final m = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  m.setMockMethodCallHandler(const MethodChannel('bowel/shell'), (call) async {
    if (call.method == 'openURL') _opened.add(call.arguments as Map);
    return null;
  });
  m.setMockMethodCallHandler(const MethodChannel('bowel/core'), (call) async {
    final args = jsonDecode(call.arguments['payload'] as String) as Map;
    switch (call.arguments['method'] as String) {
      case 'settings':
        return jsonEncode({'tailnet': _tailnet, 'canTailnet': true});
      case 'tailnet':
        return jsonEncode(_state);
      case 'setSettings':
        final patch = args['patch'] as Map;
        _patches.add(patch);
        _tailnet = patch['tailnet'] as Map;
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
                    builder: (context, _) => Stack(fit: StackFit.expand, children: [TailnetPage(app: app), SheetHost(app: app)]),
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
  await tester.pump(const Duration(milliseconds: 500));
  await tester.pump(const Duration(milliseconds: 500));
}

/// The page polls the node while it is open: closing it stops that.
Future<void> _close(WidgetTester tester) async {
  await tester.pumpWidget(const SizedBox());
  await tester.pump(const Duration(seconds: 3));
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUp(() {
    language = 'zh';
    _tailnet = {'on': true};
    _state = {'state': 'idle', 'hostname': 'bowel-android'};
    _patches.clear();
    _opened.clear();
    _fake();
  });

  tearDown(() => language = 'zh');

  testWidgets('on Tailscale\'s own server there is no domain to give', (tester) async {
    await tester.pumpWidget(_host(AppState()));
    await _settle(tester);
    expect(find.text('控制服务器'), findsOneWidget);
    expect(find.text('Tailscale'), findsWidgets);
    expect(find.text('MagicDNS 域名'), findsNothing);
    await _close(tester);
  });

  testWidgets('a control server is saved in the tailnet\'s settings', (tester) async {
    _tailnet = {'on': true, 'authKey': 'tskey-auth-a'};
    await tester.pumpWidget(_host(AppState()));
    await _settle(tester);
    await tester.tap(find.text('控制服务器'));
    await _settle(tester);
    await tester.enterText(find.byType(EditableText), ' http://hs.example.com:8080 ');
    await tester.tap(find.text('保存'));
    await _settle(tester);
    expect(_patches.single, {
      'tailnet': {'on': true, 'authKey': 'tskey-auth-a', 'controlUrl': 'http://hs.example.com:8080'},
    });
    expect(find.text('hs.example.com'), findsOneWidget);
    expect(find.text('MagicDNS 域名'), findsOneWidget);
    await _close(tester);
  });

  testWidgets('a Headscale login link may be http, and opens', (tester) async {
    _tailnet = {'on': true, 'controlUrl': 'http://hs.example.com:8080', 'domain': 'tail.example.com'};
    _state = {'state': 'login', 'loginUrl': 'http://hs.example.com:8080/register/abc', 'hostname': 'bowel-android'};
    await tester.pumpWidget(_host(AppState()));
    await _settle(tester);
    expect(find.text('tail.example.com'), findsOneWidget);
    expect(find.textContaining('在控制服务器上注册这台设备'), findsOneWidget);
    await tester.tap(find.text('打开登录链接'));
    await _settle(tester);
    expect(_opened.single, {'url': 'http://hs.example.com:8080/register/abc', 'http': true});
    await _close(tester);
  });
}
