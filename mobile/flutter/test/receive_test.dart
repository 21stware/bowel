import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:bowel/app/i18n.dart';
import 'package:bowel/app/receive.dart';
import 'package:bowel/app/state.dart';
import 'package:bowel/pages/frame.dart';
import 'package:bowel/pages/receive.dart';
import 'package:bowel/pages/setup.dart';
import 'package:bowel/pages/sheet.dart';
import 'package:bowel/ui/skin.dart';
import 'package:flutter/material.dart' show DefaultMaterialLocalizations, Material, MaterialType;
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

import 'real_font.dart';

class _Addr implements InterfaceAddress {
  _Addr(String ip) : _a = InternetAddress(ip);
  final InternetAddress _a;
  @override
  String get address => _a.address;
  @override
  InternetAddressType get type => _a.type;
  @override
  Uint8List get rawAddress => _a.rawAddress;
  @override
  bool get isLoopback => _a.isLoopback;
  @override
  bool get isLinkLocal => _a.isLinkLocal;
  @override
  bool get isMulticast => _a.isMulticast;
  @override
  dynamic noSuchMethod(Invocation i) => super.noSuchMethod(i);
}

class _If implements NetworkInterface {
  _If(this.name, List<String> ips) : addresses = [for (final ip in ips) _Addr(ip)];
  @override
  final String name;
  @override
  final List<InterfaceAddress> addresses;
  @override
  int get index => 0;
}

class _RealHttp extends HttpOverrides {}

const _sub = 'https://sub.example.com/api/v1/client/subscribe?token=abc';

/// One request to the receiver; the status and the page.
/// A body goes chunked, with no length to refuse it by, unless [sayLength].
Future<(int, String)> _send(Uri url, {String method = 'GET', String? path, String? form, String? body, String type = 'application/x-www-form-urlencoded', bool sayLength = false}) async {
  // flutter_test answers every request of its own HttpClient with a 400.
  final c = HttpOverrides.runWithHttpOverrides(HttpClient.new, _RealHttp());
  try {
    final req = await c.openUrl(method, path == null ? url : url.replace(path: path));
    if (form != null || body != null) {
      req.headers.contentType = ContentType.parse(type);
      final bytes = utf8.encode(form != null ? 'link=${Uri.encodeQueryComponent(form)}' : body!);
      req.contentLength = sayLength ? bytes.length : -1;
      req.add(bytes);
    }
    final res = await req.close();
    return (res.statusCode, await utf8.decodeStream(res));
  } finally {
    c.close(force: true);
  }
}

/// Waits, a while at most, for what the server does after answering.
Future<void> _soon(bool Function() ok) async {
  for (var i = 0; i < 200 && !ok(); i++) {
    await Future<void>.delayed(const Duration(milliseconds: 10));
  }
}

Future<bool> _refused(Uri url) async {
  try {
    final s = await Socket.connect(url.host, url.port, timeout: const Duration(seconds: 2));
    s.destroy();
    return false;
  } on SocketException {
    return true;
  }
}

void main() {
  group('cleanLink', () {
    test('takes a subscription and node links', () {
      expect(cleanLink('  $_sub \n'), _sub);
      expect(cleanLink('vmess://abc\r\ntrojan://pw@h:443#x\n\nss://y'), 'vmess://abc\ntrojan://pw@h:443#x\nss://y');
    });
    test('refuses anything else', () {
      for (final s in ['', '   ', 'hello', 'proxies:\n  - name: a', 'javascript:alert(1)', 'https://a.b and words', '<script>x</script>', 'data:text/html,hi']) {
        expect(cleanLink(s), isNull, reason: s);
      }
    });
  });

  group('pickLanAddress', () {
    test('Wi-Fi, never the VPN, cellular or Tailscale', () {
      final got = pickLanAddress([
        _If('lo', ['127.0.0.1']),
        _If('tun0', ['198.18.0.1']),
        _If('rmnet_data0', ['10.120.3.4']),
        _If('tailscale0', ['100.101.102.103']),
        _If('wlan0', ['169.254.3.3', '192.168.1.23']),
      ]);
      expect(got?.address, '192.168.1.23');
    });
    test('the VPN by its range whatever its name', () {
      expect(pickLanAddress([_If('weird0', ['198.18.0.1', '198.19.255.1'])]), isNull);
    });
    test('Ethernet before a hotspot before others', () {
      expect(pickLanAddress([_If('ap0', ['192.168.43.1']), _If('x0', ['10.0.0.9']), _If('eth0', ['10.0.0.5'])])?.address, '10.0.0.5');
      expect(pickLanAddress([_If('x0', ['10.0.0.9']), _If('ap0', ['192.168.43.1'])])?.address, '192.168.43.1');
    });
    test('none without a network', () {
      expect(pickLanAddress([_If('lo', ['127.0.0.1']), _If('tun0', ['198.18.0.1'])]), isNull);
    });
  });

  group('LinkReceiver', () {
    late List<String> got;
    late String? answer;
    late LinkReceiver r;
    late Uri url;

    Future<void> up({Duration timeout = const Duration(minutes: 5), int maxBody = 16 * 1024, int maxMisses = 20}) async {
      got = [];
      answer = null;
      r = LinkReceiver(onLink: (l) async {
        got.add(l);
        return answer;
      }, timeout: timeout, maxBody: maxBody, maxMisses: maxMisses);
      url = await r.start(InternetAddress.loopbackIPv4);
    }

    tearDown(() => r.dispose());

    test('listens on the address given, at a random port, behind a token', () async {
      await up();
      expect(url.host, '127.0.0.1');
      expect(url.port, isNot(0));
      expect(url.path, '/${r.token}');
      expect(r.token, matches(RegExp(r'^[a-hj-km-np-z2-9]{8}$')));
      final other = LinkReceiver(onLink: (_) async => null);
      expect(other.token, isNot(r.token));
      other.dispose();
      final (code, page) = await _send(url);
      expect(code, 200);
      expect(page, contains('<form method="post">'));
      expect(page, isNot(contains('<script')));
    });

    test('adds what is sent, answers, and stops', () async {
      await up();
      final (code, page) = await _send(url, method: 'POST', form: _sub);
      expect(code, 200);
      expect(got, [_sub]);
      expect(page, contains(tr('已添加到 bowel，可以关掉这个页面了。', 'Added to bowel. You can close this page.')));
      await _soon(() => !r.listening);
      expect(r.listening, isFalse);
      expect(r.end, ReceiveEnd.added);
      expect(await _refused(url), isTrue);
    });

    test('a wrong token is not let in', () async {
      await up();
      for (final p in ['/', '/x${r.token}', '/${r.token}x', '/${r.token.toUpperCase()}']) {
        final (code, _) = await _send(url, method: 'POST', path: p, form: _sub);
        expect(code, 404, reason: p);
      }
      expect(got, isEmpty);
      expect(r.listening, isTrue);
    });

    test('stops after too many guesses, not counting the icon', () async {
      await up(maxMisses: 3);
      for (var i = 0; i < 5; i++) {
        await _send(url, path: '/favicon.ico');
      }
      expect(r.listening, isTrue);
      for (var i = 0; i < 3; i++) {
        await _send(url, path: '/guess$i');
      }
      await _soon(() => !r.listening);
      expect(r.listening, isFalse);
      expect(r.end, ReceiveEnd.locked);
      expect(await _refused(url), isTrue);
    });

    test('refuses a body over the limit, said or not', () async {
      await up(maxBody: 1024);
      final big = 'vmess://${'a' * 2000}';
      final (said, _) = await _send(url, method: 'POST', body: 'link=$big', sayLength: true);
      expect(said, 413);
      final (chunked, _) = await _send(url, method: 'POST', form: big);
      expect(chunked, 413);
      expect(got, isEmpty);
      expect(r.listening, isTrue);
    });

    test('takes only link text', () async {
      await up();
      final (notLink, page) = await _send(url, method: 'POST', form: 'proxies:\n  - {name: a, type: ss}');
      expect(notLink, 400);
      expect(page, contains('<textarea'));
      final (json, _) = await _send(url, method: 'POST', body: '{"link":"$_sub"}', type: 'application/json');
      expect(json, 415);
      final (put, _) = await _send(url, method: 'PUT');
      expect(put, 405);
      final (plain, _) = await _send(url, method: 'POST', body: _sub, type: 'text/plain; charset=utf-8');
      expect(plain, 200);
      expect(got, [_sub]);
    });

    test('a link that is not added is said, and it keeps listening', () async {
      await up();
      answer = 'download failed <b>';
      final (code, page) = await _send(url, method: 'POST', form: _sub);
      expect(code, 422);
      expect(page, contains('download failed &lt;b&gt;'));
      expect(page, contains(const HtmlEscape().convert(_sub)));
      expect(r.listening, isTrue);
      expect(r.lastError, 'download failed <b>');
      answer = null;
      final (again, _) = await _send(url, method: 'POST', form: _sub);
      expect(again, 200);
      await _soon(() => r.end != null);
      expect(r.end, ReceiveEnd.added);
    });

    test('stops on its own after the timeout', () async {
      await up(timeout: const Duration(milliseconds: 200));
      await Future<void>.delayed(const Duration(milliseconds: 400));
      expect(r.listening, isFalse);
      expect(r.end, ReceiveEnd.timeout);
      expect(await _refused(url), isTrue);
    });

    test('stop closes it', () async {
      await up();
      await r.stop();
      expect(r.end, ReceiveEnd.closed);
      expect(await _refused(url), isTrue);
      await r.stop(); // twice is fine
    });
  });

  group('sheet', () {
    // Widths decide the sheet's layout.
    setUpAll(loadSystemFont);
    tearDown(() => language = 'zh');

    /// The setup page, which has its own sheet host; [bare] the sheet host alone.
    Widget host(AppState app, Size size, {bool bare = false}) => Localizations(
          locale: const Locale('en'),
          delegates: const [DefaultMaterialLocalizations.delegate, DefaultWidgetsLocalizations.delegate],
          child: MediaQuery(
            data: MediaQueryData(size: size),
            child: Skin(
              look: Look.of('paper', false),
              child: Frame(
                data: FrameData(top: 47, barInset: 0, nativeBar: false, homeIndicator: 0, form: windowFormOf(size)),
                child: Navigator(onGenerateRoute: (_) => PageRouteBuilder<void>(pageBuilder: (_, _, _) => Material(type: MaterialType.transparency, child: ListenableBuilder(listenable: app, builder: (_, _) => bare ? SheetHost(app: app) : SetupPage(app: app))))),
              ),
            ),
          ),
        );

    final rows = ['1' * 29, for (var i = 0; i < 27; i++) '1${'0' * 27}1', '1' * 29];

    /// Lets real I/O run, pumping between, until [f] is done.
    Future<T> io<T>(WidgetTester t, Future<T> Function() f) async {
      T? out;
      Object? err;
      var done = false;
      await t.runAsync(() async {
        f().then((v) {
          out = v;
          done = true;
        }, onError: (Object e) {
          err = e;
          done = true;
        });
      });
      for (var i = 0; i < 1000 && !done; i++) {
        await t.runAsync(() => Future<void>.delayed(const Duration(milliseconds: 5)));
        await t.pump();
      }
      if (err != null) throw err!;
      expect(done, isTrue, reason: 'timed out');
      return out as T;
    }

    Future<ReceivePanelState> open(WidgetTester t, AppState app, {Duration timeout = const Duration(minutes: 5)}) async {
      app.showSheet((_) => ReceivePanel(app: app, address: () async => InternetAddress.loopbackIPv4, qr: (_) async => rows, timeout: timeout));
      await t.pump();
      final s = t.state<ReceivePanelState>(find.byType(ReceivePanel));
      await io(t, () async {
        while (s.receiver?.url == null) {
          await Future<void>.delayed(const Duration(milliseconds: 5));
        }
      });
      await t.pump();
      return s;
    }

    for (final lang in ['zh', 'en', 'ja', 'zh-Hant']) {
      for (final size in const [Size(360, 640), Size(960, 540)]) {
        testWidgets('$lang: fits ${size.width.toInt()}x${size.height.toInt()} and shows the address', (t) async {
          language = lang;
          t.view.physicalSize = size;
          t.view.devicePixelRatio = 1;
          addTearDown(t.view.reset);
          final app = AppState()..setup = 1;
          await t.pumpWidget(host(app, size, bare: true));
          final s = await open(t, app);
          expect(t.takeException(), isNull);
          expect(find.text('${s.receiver!.url}'), findsOneWidget);
          expect(find.byType(CustomPaint), findsWidgets);
          await t.pumpWidget(const SizedBox());
        });
      }
    }

    // On a page, as a TV's empty Provider list has it.
    for (final lang in ['zh', 'en', 'ja', 'zh-Hant']) {
      for (final size in const [Size(360, 640), Size(960, 540)]) {
        testWidgets('$lang: on a page, fits ${size.width.toInt()}x${size.height.toInt()} serving and timed out', (t) async {
          language = lang;
          t.view.physicalSize = size;
          t.view.devicePixelRatio = 1;
          addTearDown(t.view.reset);
          final app = AppState();
          const timeout = Duration(seconds: 2);
          await t.pumpWidget(Localizations(
            locale: const Locale('en'),
            delegates: const [DefaultMaterialLocalizations.delegate, DefaultWidgetsLocalizations.delegate],
            child: MediaQuery(
              data: MediaQueryData(size: size),
              child: Skin(
                look: Look.of('paper', false),
                child: ListView(padding: const EdgeInsets.all(16), children: [
                  ReceivePanel(app: app, inline: true, address: () async => InternetAddress.loopbackIPv4, qr: (_) async => rows, timeout: timeout),
                ]),
              ),
            ),
          ));
          final s = t.state<ReceivePanelState>(find.byType(ReceivePanel));
          await io(t, () async {
            while (s.receiver?.url == null) {
              await Future<void>.delayed(const Duration(milliseconds: 5));
            }
          });
          await t.pump();
          expect(t.takeException(), isNull);
          final first = s.receiver!.url;
          expect(find.text('$first'), findsOneWidget);
          expect(find.text(tr('关闭', 'Close')), findsNothing, reason: 'a page has nothing to close');
          expect(find.text(tr('重新开始', 'Start again')), findsNothing);

          await t.pump(timeout + const Duration(seconds: 1));
          await t.pump();
          expect(t.takeException(), isNull);
          expect(s.receiver!.listening, isFalse);
          await t.tap(find.text(tr('重新开始', 'Start again')));
          await io(t, () async {
            while (s.receiver?.url == null || s.receiver!.url == first) {
              await Future<void>.delayed(const Duration(milliseconds: 5));
            }
          });
          await t.pump();
          expect(s.receiver!.listening, isTrue);
          await t.pumpWidget(const SizedBox());
        });
      }
    }

    testWidgets('closing the sheet stops the receiver', (t) async {
      final app = AppState()..setup = 1;
      await t.pumpWidget(host(app, const Size(360, 640)));
      final s = await open(t, app);
      final r = s.receiver!;
      final url = r.url!;
      expect(r.listening, isTrue);
      app.closeSheet();
      await t.pump();
      await t.pump(const Duration(milliseconds: 300));
      expect(find.byType(ReceivePanel), findsNothing);
      expect(r.listening, isFalse);
      expect(await io(t, () => _refused(url)), isTrue);
      await t.pumpWidget(const SizedBox());
    });

    testWidgets('a link sent from the phone fills setup and closes the sheet', (t) async {
      language = 'en';
      final app = AppState()..setup = 1;
      await t.pumpWidget(host(app, const Size(360, 640)));
      final s = await open(t, app);
      final r = s.receiver!;
      final (code, _) = await io(t, () => _send(r.url!, method: 'POST', form: _sub));
      expect(code, 200);
      await io(t, () => _soon(() => r.end != null));
      await t.pump(const Duration(milliseconds: 300));
      expect(app.setupField.text, _sub);
      expect(app.setup, 2); // no core in tests: setup goes on, as with a pasted link
      expect(app.sheet, isNull);
      expect(r.end, ReceiveEnd.added);
      await t.pumpWidget(const SizedBox());
    });

    testWidgets('a failure is shown on the device too, and the timeout offers to start again', (t) async {
      language = 'en';
      final app = AppState(); // past setup, with no core: adding fails
      await t.pumpWidget(host(app, const Size(360, 640), bare: true));
      final s = await open(t, app, timeout: const Duration(seconds: 2));
      final (code, page) = await io(t, () => _send(s.receiver!.url!, method: 'POST', form: _sub));
      expect(code, 422);
      expect(page, contains("The core isn&#39;t running"));
      expect(app.providerField.text, _sub);
      await t.pump();
      expect(find.textContaining('Not added:'), findsOneWidget);
      await t.pump(const Duration(seconds: 3));
      expect(s.receiver!.end, ReceiveEnd.timeout);
      await t.pump();
      expect(find.text('Start again'), findsOneWidget);
      await t.pump(const Duration(seconds: 7)); // the toast
      await t.pumpWidget(const SizedBox());
    });

    testWidgets('no network says so', (t) async {
      language = 'en';
      final app = AppState()..setup = 1;
      await t.pumpWidget(host(app, const Size(360, 640)));
      app.showSheet((_) => ReceivePanel(app: app, address: () async => null, qr: (_) async => rows));
      await t.pump();
      await t.pump();
      expect(find.text("This device isn't on Wi-Fi or a wired network"), findsOneWidget);
      expect(find.text('Start again'), findsOneWidget);
      await t.pumpWidget(const SizedBox());
    });

    testWidgets('setup offers it', (t) async {
      language = 'en';
      final app = AppState()..setup = 1;
      await t.pumpWidget(host(app, const Size(360, 640)));
      expect(find.text('Send a link from a phone'), findsOneWidget);
    });
  });
}
