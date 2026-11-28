import 'package:bowel/app/i18n.dart';
import 'package:bowel/app/sample.dart';
import 'package:bowel/app/shell.dart';
import 'package:bowel/app/state.dart';
import 'package:bowel/main.dart';
import 'package:bowel/pages/frame.dart';
import 'package:bowel/pages/providers.dart';
import 'package:bowel/pages/receive.dart';
import 'package:bowel/pages/sheet.dart';
import 'package:bowel/ui/atoms.dart';
import 'package:bowel/ui/skin.dart';
import 'package:flutter/material.dart' show DefaultMaterialLocalizations, Material, MaterialType;
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

import 'real_font.dart';

// The ways to add follow what the device can do: a TV, a phone without a
// camera, one without pickers. A phone with everything keeps them all.

const _scan = 'Scan QR code', _file = 'Choose file', _phone = 'Send a link from a phone';
const _yaml = 'Choose a YAML file', _photos = 'Read a QR code from Photos', _paste = 'Paste link from clipboard', _all = 'Update all subscriptions';

void _device({bool tv = false, bool camera = true, bool files = true, bool images = true}) {
  Shell.instance
    ..tv = tv
    ..camera = camera
    ..files = files
    ..images = images;
}

const _tv = (tv: true, camera: false, files: false, images: false);

Future<void> _settle(WidgetTester t) async {
  for (var i = 0; i < 8; i++) {
    await t.pump(const Duration(milliseconds: 100));
  }
}

Future<AppState> _setup(WidgetTester t, Size size) async {
  t.view.physicalSize = size;
  t.view.devicePixelRatio = 1;
  addTearDown(t.view.reset);
  final app = AppState()..setup = 1;
  await t.pumpWidget(BowelApp(app: app, ready: Future.value()));
  await _settle(t);
  await _settle(t);
  return app;
}

Widget _providers(AppState app, Size size) => Localizations(
      locale: const Locale('en'),
      delegates: const [DefaultMaterialLocalizations.delegate, DefaultWidgetsLocalizations.delegate],
      child: MediaQuery(
        data: MediaQueryData(size: size),
        child: Skin(
          look: Look.of('paper', false),
          child: Frame(
            data: FrameData(top: 0, barInset: 0, nativeBar: false, homeIndicator: 0, form: windowFormOf(size)),
            child: Navigator(
              onGenerateRoute: (_) => PageRouteBuilder<void>(
                pageBuilder: (_, _, _) => Material(
                  type: MaterialType.transparency,
                  child: AnimatedBuilder(animation: app, builder: (context, _) => Stack(fit: StackFit.expand, children: [ProvidersPage(app: app), SheetHost(app: app)])),
                ),
              ),
            ),
          ),
        ),
      ),
    );

/// The ways the Provider tab's ⋯ offers.
Future<List<String>> _more(WidgetTester t, AppState app) async {
  await t.tap(find.bySemanticsLabel('More ways to add'));
  await _settle(t);
  final shown = [for (final s in [_phone, _yaml, _photos, _paste, _all]) if (find.descendant(of: find.byType(SheetHost), matching: find.text(s)).evaluate().isNotEmpty) s];
  app.closeSheet();
  await _settle(t);
  return shown;
}

bool _has(String text) => find.text(text).evaluate().isNotEmpty;

void main() {
  setUpAll(loadSystemFont);
  setUp(() {
    language = 'en';
    FocusManager.instance.highlightStrategy = FocusHighlightStrategy.automatic;
  });
  tearDown(() {
    language = 'zh';
    Shell.instance.resetDevice();
  });

  group('setup', () {
    testWidgets('a phone keeps every way', (t) async {
      await _setup(t, const Size(390, 844));
      expect([_scan, _file, _phone].where(_has), [_scan, _file, _phone]);
      expect(find.textContaining('or scan a QR code'), findsOneWidget);
    });

    testWidgets('no camera: no scanning, and the words don\'t ask for it', (t) async {
      _device(camera: false);
      await _setup(t, const Size(390, 844));
      expect(_has(_scan), isFalse);
      expect(_has(_file), isTrue);
      expect(_has(_phone), isTrue);
      expect(find.textContaining('or send one from a phone'), findsOneWidget);
      expect(t.takeException(), isNull);
    });

    testWidgets('no picker: no file', (t) async {
      _device(files: false, images: false);
      await _setup(t, const Size(390, 844));
      expect(_has(_file), isFalse);
      expect(_has(_scan), isTrue);
      expect(_has(_phone), isTrue);
    });

    for (final lang in ['zh', 'en', 'ja', 'zh-Hant']) {
      testWidgets('$lang: a TV offers only the phone, full width, a press below the field', (t) async {
        language = lang;
        _device(tv: _tv.tv, camera: _tv.camera, files: _tv.files, images: _tv.images);
        const size = Size(960, 540);
        final app = await _setup(t, size);
        expect(t.takeException(), isNull);
        final phone = tr('从手机发送链接', _phone);
        expect(_has(tr('扫描二维码', _scan)), isFalse);
        expect(_has(tr('选择文件', _file)), isFalse);
        expect(_has(phone), isTrue);
        final field = t.getRect(find.byType(BareField));
        final btn = t.getRect(find.ancestor(of: find.text(phone), matching: find.byType(Btn)));
        expect(btn.width, greaterThanOrEqualTo(field.width)); // the field sits padded in its box; the button spans the box
        expect(btn.top, greaterThan(field.bottom));

        // the field, then Down: the phone is next
        t.binding.focusManager.primaryFocus?.unfocus();
        Focus.of(t.element(find.byType(EditableText))).requestFocus();
        FocusManager.instance.highlightStrategy = FocusHighlightStrategy.alwaysTraditional;
        await _settle(t);
        await t.sendKeyEvent(LogicalKeyboardKey.arrowDown);
        await _settle(t);
        final focused = FocusManager.instance.primaryFocus?.context;
        expect(focused, isNotNull);
        expect(find.descendant(of: find.byElementPredicate((e) => e == focused), matching: find.text(phone)), findsOneWidget);
        await t.sendKeyEvent(LogicalKeyboardKey.select);
        await _settle(t);
        expect(find.byType(ReceivePanel), findsOneWidget);
        app.closeSheet();
        await _settle(t);
      });
    }

    testWidgets('a TV with a file picker keeps it beside the phone, which comes first', (t) async {
      _device(tv: true, camera: false);
      await _setup(t, const Size(960, 540));
      expect(t.takeException(), isNull);
      final phone = t.getRect(find.text(_phone));
      final file = t.getRect(find.text(_file));
      expect(phone.left, lessThan(file.left));
      expect(_has(_scan), isFalse);
    });

    testWidgets('step 2 on a TV points to the app, not a tile or notification', (t) async {
      _device(tv: true, camera: false, files: false, images: false);
      final app = await _setup(t, const Size(960, 540));
      app.set(() => app.setup = 2);
      await _settle(t);
      expect(find.text('In the app, whenever you like'), findsOneWidget);
      expect(find.textContaining('Quick Settings'), findsNothing);
      expect(t.takeException(), isNull);
    });
  });

  group('Provider tab', () {
    testWidgets('a phone keeps the scan icon and every way under ⋯', (t) async {
      final app = AppState();
      await t.pumpWidget(_providers(app, const Size(360, 760)));
      await _settle(t);
      expect(find.bySemanticsLabel(_scan), findsOneWidget);
      expect(_has(_phone), isFalse); // under ⋯, not in the bar
      expect(await _more(t, app), [_phone, _yaml, _photos, _paste, _all]);
    });

    testWidgets('no camera, no pickers: no scan icon, no files or photos', (t) async {
      _device(camera: false, files: false, images: false);
      final app = AppState();
      await t.pumpWidget(_providers(app, const Size(360, 760)));
      await _settle(t);
      expect(find.bySemanticsLabel(_scan), findsNothing);
      expect(await _more(t, app), [_phone, _paste, _all]);
    });

    testWidgets('a TV has the phone in the bar, and no clipboard', (t) async {
      _device(tv: _tv.tv, camera: _tv.camera, files: _tv.files, images: _tv.images);
      const size = Size(960, 540);
      t.view.physicalSize = size;
      t.view.devicePixelRatio = 1;
      addTearDown(t.view.reset);
      final app = AppState();
      await t.pumpWidget(_providers(app, size));
      await _settle(t);
      expect(t.takeException(), isNull);
      expect(find.bySemanticsLabel(_scan), findsNothing);
      expect(_has(_phone), isTrue);
      expect(await _more(t, app), [_all]);
      await t.tap(find.text(_phone));
      await _settle(t);
      expect(find.byType(ReceivePanel), findsOneWidget);
      app.closeSheet();
      await _settle(t);
    });
  });

  group('an empty Provider list', () {
    late List<ProviderInfo> had;
    setUp(() => had = providers);
    tearDown(() => providers = had);

    Future<AppState> shown(WidgetTester t, {required bool tv, int tab = 2}) async {
      if (tv) _device(tv: _tv.tv, camera: _tv.camera, files: _tv.files, images: _tv.images);
      const size = Size(960, 540);
      t.view.physicalSize = size;
      t.view.devicePixelRatio = 1;
      addTearDown(t.view.reset);
      providers = const [];
      final app = AppState()..tab = tab;
      await t.pumpWidget(_providers(app, size));
      await _settle(t);
      return app;
    }

    for (final lang in ['zh', 'en', 'ja', 'zh-Hant']) {
      testWidgets('$lang: a TV shows the address for a phone on the page, not behind a button', (t) async {
        language = lang;
        await shown(t, tv: true);
        expect(t.takeException(), isNull);
        final panel = find.byType(ReceivePanel);
        expect(panel, findsOneWidget);
        expect(t.widget<ReceivePanel>(panel).inline, isTrue);
        expect(find.descendant(of: find.byType(SheetHost), matching: panel), findsNothing);
        expect(_has(tr('从手机添加第一个 Provider', 'Add your first Provider from a phone')), isTrue);
        expect(_has(tr('从手机发送链接', 'Send a link from a phone')), isFalse, reason: 'the bar has no second way to the same thing');
        expect(_has(tr('关闭', 'Close')), isFalse);
      });
    }

    testWidgets('only while the Provider tab is in front, as the tabs stay built', (t) async {
      final app = await shown(t, tv: true, tab: 0);
      expect(find.byType(ReceivePanel), findsNothing);
      expect(_has(_phone), isTrue);
      app.set(() => app.tab = 2);
      await _settle(t);
      expect(find.byType(ReceivePanel), findsOneWidget);
      app.set(() => app.tab = 3);
      await _settle(t);
      expect(find.byType(ReceivePanel), findsNothing);
    });

    testWidgets('a phone keeps its list as it was', (t) async {
      await shown(t, tv: false);
      expect(find.byType(ReceivePanel), findsNothing);
    });

    testWidgets('a TV with a Provider has the button, not the panel', (t) async {
      _device(tv: _tv.tv, camera: _tv.camera, files: _tv.files, images: _tv.images);
      const size = Size(960, 540);
      t.view.physicalSize = size;
      t.view.devicePixelRatio = 1;
      addTearDown(t.view.reset);
      await t.pumpWidget(_providers(AppState()..tab = 2, size));
      await _settle(t);
      expect(providers, isNotEmpty);
      expect(find.byType(ReceivePanel), findsNothing);
      expect(_has(_phone), isTrue);
    });
  });

  group('picking', () {
    /// The shell answers pickFile and pickImage with [answer].
    void shell(Future<Object?> Function() answer) {
      final m = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
      m.setMockMethodCallHandler(const MethodChannel('bowel/shell'), (call) => call.method.startsWith('pick') ? answer() : Future.value());
      addTearDown(() => m.setMockMethodCallHandler(const MethodChannel('bowel/shell'), null));
    }

    testWidgets('nothing that picks (a TV\'s stub): said, and no longer offered', (t) async {
      shell(() => throw PlatformException(code: 'nopicker'));
      final app = AppState()..setup = 0;
      await app.addFile();
      expect(app.toast, 'No app on this device can choose files');
      expect(Shell.instance.files, isFalse);
      expect(Shell.instance.images, isTrue);
      await app.addFromImage();
      expect(app.toast, 'No app on this device can choose pictures');
      expect(Shell.instance.images, isFalse);
      await t.pump(const Duration(seconds: 5));
    });

    testWidgets('a picker that comes back with nothing says so', (t) async {
      shell(() async => null);
      final app = AppState()..setup = 0;
      await app.addFile();
      expect(app.toast, 'No file was chosen');
      await app.addFromImage();
      expect(app.toast, 'No picture was chosen');
      expect(await app.pickFile(), isNull);
      expect(Shell.instance.files && Shell.instance.images, isTrue); // still offered: the user may just have cancelled
      await t.pump(const Duration(seconds: 5));
    });

    testWidgets('once found out, the ways go from the Provider tab', (t) async {
      shell(() => throw PlatformException(code: 'nopicker'));
      final app = AppState();
      await t.pumpWidget(_providers(app, const Size(360, 760)));
      await _settle(t);
      await t.tap(find.bySemanticsLabel('More ways to add'));
      await _settle(t);
      await t.tap(find.text(_yaml));
      await _settle(t);
      expect(await _more(t, app), [_phone, _photos, _paste, _all]);
      await t.pump(const Duration(seconds: 5));
    });
  });
}
