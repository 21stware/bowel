import 'dart:convert';

import 'package:bowel/app/i18n.dart';
import 'package:bowel/app/state.dart';
import 'package:bowel/pages/frame.dart';
import 'package:bowel/pages/settings.dart';
import 'package:bowel/pages/sheet.dart';
import 'package:bowel/ui/atoms.dart';
import 'package:bowel/ui/skin.dart';
import 'package:flutter/material.dart' show DefaultMaterialLocalizations, Material, MaterialType;
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

/// The newest release the bridge finds; null when the app is already on it.
String? _latest;

/// The bridge can't reach GitHub.
bool _down = false;
final _checks = <Map>[];
final _shell = <MethodCall>[];

String _apk(String v) => 'https://github.com/21stware/bowel/releases/download/v$v/bowel-$v-android-arm64.apk';

void _fakes() {
  final messenger = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  messenger.setMockMethodCallHandler(const MethodChannel('bowel/core'), (call) async {
    if (call.method != 'call') return null;
    final method = call.arguments['method'] as String;
    if (method == 'view') return jsonEncode({'vpn': 'off', 'mode': 'rule', 'nodes': <Map>[], 'providers': <Map>[], 'routes': <Map>[]});
    if (method != 'checkUpdate') return 'null';
    _checks.add(jsonDecode(call.arguments['payload'] as String) as Map);
    if (_down) throw PlatformException(code: 'call', message: 'no route to host');
    return _latest == null ? 'null' : jsonEncode({'version': _latest, 'url': _apk(_latest!)});
  });
  messenger.setMockMethodCallHandler(const MethodChannel('bowel/shell'), (call) async {
    _shell.add(call);
    return null;
  });
}

/// An app on Android, at `version`, with the core attached.
Future<AppState> _app(WidgetTester tester, String version) async {
  final app = AppState();
  await app.attachCore();
  app.appVersion = version;
  return app;
}

/// A phone's screen around `child`, which is rebuilt as the app changes.
Widget _host(AppState app, Widget Function() child, {double width = 360}) => Localizations(
      locale: const Locale('en'),
      delegates: const [DefaultMaterialLocalizations.delegate, DefaultWidgetsLocalizations.delegate],
      child: MediaQuery(
        data: MediaQueryData(size: Size(width, 1600)),
        child: Skin(
          look: Look.of('paper', false),
          child: Frame(
            data: const FrameData(top: 0, barInset: 0, nativeBar: false, homeIndicator: 0, form: WindowForm.phone),
            child: Material(
              type: MaterialType.transparency,
              child: AnimatedBuilder(animation: app, builder: (context, _) => Stack(fit: StackFit.expand, children: [child(), SheetHost(app: app)])),
            ),
          ),
        ),
      ),
    );

Future<void> _show(WidgetTester tester, AppState app, Widget Function() child, {double width = 360}) async {
  tester.view.physicalSize = Size(width, 1600);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(_host(app, child, width: width));
  await tester.pump();
}

final _close = find.byWidgetPredicate((w) => w is Ic && w.name == 'x');

/// Lets a toast's timer run out.
Future<void> _quiet(WidgetTester tester) => tester.pump(const Duration(seconds: 5));

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUp(() {
    _latest = '0.4.5';
    _down = false;
    _checks.clear();
    _shell.clear();
    language = 'zh';
    _fakes();
  });

  testWidgets('a check finds the newer version, and one that fails says nothing', (tester) async {
    final app = await _app(tester, '0.4.4');
    expect(await app.checkUpdate(), isTrue);
    expect(_checks.single, {'version': '0.4.4'});
    expect(app.update, (version: '0.4.5', url: _apk('0.4.5')));
    expect(app.updateNotice, isTrue);
    expect(app.toast, isNull);

    _down = true;
    expect(await app.checkUpdate(), isFalse);
    expect(app.toast, isNull);
    expect(app.update?.version, '0.4.5', reason: 'what was found stays through a failed check');

    _down = false;
    _latest = null;
    expect(await app.checkUpdate(), isTrue);
    expect(app.update, isNull);
    expect(app.updateNotice, isFalse);
    expect(app.toast, isNull);
  });

  testWidgets('a check the user asked for says how it went', (tester) async {
    final app = await _app(tester, '0.4.5');
    _latest = null;
    await app.checkUpdate(manual: true);
    expect(app.toast, '已是最新版本 0.4.5');

    _latest = '0.4.6';
    await app.checkUpdate(manual: true);
    expect(app.toast, '新版本 0.4.6');

    _down = true;
    await app.checkUpdate(manual: true);
    expect(app.toast, '检查更新失败：no route to host');
    expect(app.updateChecking, isFalse);
    await _quiet(tester);
  });

  testWidgets('checks 20 s after opening, every 6 h, and 30 min after a failure', (tester) async {
    final app = await _app(tester, '0.4.4');
    app.scheduleUpdateChecks();
    await tester.pump(const Duration(seconds: 19));
    expect(_checks, isEmpty);
    await tester.pump(const Duration(seconds: 1));
    await tester.pump();
    expect(_checks, hasLength(1));
    expect(app.update?.version, '0.4.5');

    await tester.pump(const Duration(hours: 5, minutes: 59));
    expect(_checks, hasLength(1));
    _down = true;
    await tester.pump(const Duration(minutes: 1));
    await tester.pump();
    expect(_checks, hasLength(2));

    _down = false;
    await tester.pump(const Duration(minutes: 29));
    expect(_checks, hasLength(2));
    await tester.pump(const Duration(minutes: 1));
    await tester.pump();
    expect(_checks, hasLength(3));
    expect(app.toast, isNull);
    app.dispose(); // the next check is 6 h away
  });

  testWidgets('nothing is checked where the app has no version to compare', (tester) async {
    final app = await _app(tester, ''); // iOS: the store updates the app
    app.scheduleUpdateChecks();
    await tester.pump(const Duration(hours: 7));
    expect(_checks, isEmpty);
  });

  testWidgets('a closed notice stays closed for that version only', (tester) async {
    final app = await _app(tester, '0.4.4');
    await app.checkUpdate();
    app.dismissUpdate();
    expect(app.updateNotice, isFalse);
    expect(app.update?.version, '0.4.5', reason: 'Settings still offers it');
    final saved = _shell.singleWhere((c) => c.method == 'setPref');
    expect(saved.arguments, {'key': 'updateDismissed', 'value': '0.4.5'});

    _latest = '0.4.6';
    await app.checkUpdate();
    expect(app.updateNotice, isTrue);
  });

  testWidgets('去下载 opens the link the check found', (tester) async {
    final app = await _app(tester, '0.4.4');
    await app.checkUpdate();
    app.openUpdate();
    await tester.pump();
    expect(_shell.singleWhere((c) => c.method == 'openURL').arguments, {'url': _apk('0.4.5')});
  });

  testWidgets('the notice names the version, opens its link, and closes', (tester) async {
    final app = await _app(tester, '0.4.4');
    await _show(tester, app, () => Align(alignment: Alignment.topCenter, child: UpdateNotice(app: app)));
    expect(find.text('新版本 0.4.5'), findsNothing, reason: 'nothing found yet');
    expect(tester.getSize(find.byType(UpdateNotice)).height, 0);

    await app.checkUpdate();
    await tester.pump();
    expect(find.text('新版本 0.4.5'), findsOneWidget);
    await tester.tap(find.text('去下载'));
    await tester.pump();
    expect(_shell.singleWhere((c) => c.method == 'openURL').arguments, {'url': _apk('0.4.5')});

    await tester.tap(_close);
    await tester.pump();
    expect(find.text('新版本 0.4.5'), findsNothing);
    expect(app.update?.version, '0.4.5');
  });

  for (final code in languages) {
    testWidgets('$code: the notice fits a narrow phone', (tester) async {
      final app = await _app(tester, '0.4.4');
      _latest = '10.20.30';
      language = code;
      addTearDown(() => language = 'zh');
      await app.checkUpdate();
      await _show(tester, app, () => Align(alignment: Alignment.topCenter, child: UpdateNotice(app: app)), width: 320);
      expect(tester.takeException(), isNull);
      expect(find.textContaining('10.20.30'), findsOneWidget);
    });
  }

  testWidgets('Settings offers the new version and a check by hand', (tester) async {
    final app = await _app(tester, '0.4.4');
    _latest = null;
    await _show(tester, app, () => SettingsPage(app: app));
    expect(find.text('0.4.4'), findsOneWidget, reason: 'the version this copy really is');
    expect(find.text('新版本 0.4.5'), findsNothing);

    _latest = '0.4.5';
    await tester.tap(find.text('检查更新'));
    await tester.pump();
    expect(_checks, hasLength(1));
    expect(find.text('新版本 0.4.5'), findsOneWidget);

    app.dismissUpdate(); // the notice on 代理 was closed: this row stays
    await tester.pump();
    await tester.tap(find.text('新版本 0.4.5'));
    await tester.pump();
    expect(_shell.singleWhere((c) => c.method == 'openURL').arguments, {'url': _apk('0.4.5')});
    await _quiet(tester);
  });

  testWidgets('Settings has no check where the store updates the app', (tester) async {
    final app = await _app(tester, '');
    await _show(tester, app, () => SettingsPage(app: app));
    expect(find.text('检查更新'), findsNothing);
  });

  testWidgets('the 设置 tab carries a dot while a new version waits', (tester) async {
    final app = await _app(tester, '0.4.4');
    await _show(tester, app, () => Align(alignment: Alignment.bottomCenter, child: AndroidBar(app: app)));
    final before = tester.widgetList(find.byType(Dot)).length;
    await app.checkUpdate();
    await tester.pump();
    expect(tester.widgetList(find.byType(Dot)).length, before + 1);
  });
}
