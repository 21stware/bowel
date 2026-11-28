import 'dart:convert';
import 'dart:io';

import 'package:bowel/app/i18n.dart';
import 'package:bowel/app/receive.dart';
import 'package:bowel/app/state.dart';
import 'package:bowel/pages/phone_settings.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';

class _RealHttp extends HttpOverrides {}

/// One request, redirects not followed: the status, the page and where it
/// sends the browser.
Future<(int, String, String?)> _req(Uri url, {Map<String, String>? form}) async {
  // flutter_test answers every request of its own HttpClient with a 400.
  final c = HttpOverrides.runWithHttpOverrides(HttpClient.new, _RealHttp());
  try {
    final req = await c.openUrl(form == null ? 'GET' : 'POST', url);
    req.followRedirects = false;
    if (form != null) {
      req.headers.contentType = ContentType.parse('application/x-www-form-urlencoded');
      req.write(Uri(queryParameters: form).query);
    }
    final res = await req.close();
    return (res.statusCode, await utf8.decodeStream(res), res.headers.value(HttpHeaders.locationHeader));
  } finally {
    c.close(force: true);
  }
}

class _Panel implements ReceiverPanel {
  final got = <Map<String, String>>[];

  @override
  String get title => 'Panel';

  @override
  Future<String> body({String? open}) async => '<form method="post"><input name="x"></form>';

  @override
  Future<({String message, bool bad})> submit(Map<String, String> form) async {
    got.add(form);
    return form['x'] == 'bad' ? (message: 'no <good>', bad: true) : (message: 'done', bad: false);
  }
}

/// The core's side, as the bridge gives it, and what was asked of it.
Map<String, Object?> _settings = {};
final _calls = <(String, Map)>[];
final _prefs = <String, Object?>{};

Map _routing() => {
      'kinds': [
        {'id': 'domain', 'name': 'Domain'},
        {'id': 'ip', 'name': 'IP'},
      ],
      'groups': [
        {
          'id': 'ai',
          'name': 'AI',
          'services': [
            {'id': 'mine-ai', 'name': 'Mine', 'mine': true},
            {'id': 'openai', 'name': 'OpenAI'},
            {'id': 'set1', 'name': 'A set', 'set': {'count': 3}},
          ],
        },
      ],
    };

void _fake() {
  final m = TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  m.setMockMethodCallHandler(const MethodChannel('bowel/shell'), (call) async {
    if (call.method == 'setPref') {
      final a = call.arguments as Map;
      _prefs['${a['key']}'] = a['value'];
    }
    return null;
  });
  m.setMockMethodCallHandler(const MethodChannel('bowel/core'), (call) async {
    final method = call.arguments['method'] as String;
    final args = jsonDecode(call.arguments['payload'] as String) as Map;
    _calls.add((method, args));
    switch (method) {
      case 'settings':
        return jsonEncode(_settings);
      case 'routing':
        return jsonEncode(_routing());
      case 'setSettings':
        _settings = {..._settings, ...(args['patch'] as Map).cast<String, Object?>()};
        return 'null';
      case 'routeOp':
        if (args['text'] == 'taken.com') throw PlatformException(code: 'core', message: 'conflict:Netflix');
        return 'null';
    }
    return 'null';
  });
}

List<Map> _patches() => [for (final c in _calls) if (c.$1 == 'setSettings') c.$2['patch'] as Map];

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('a receiver serving a panel', () {
    late _Panel p;
    late LinkReceiver r;
    late Uri url;

    Future<void> up({Duration timeout = const Duration(minutes: 5)}) async {
      p = _Panel();
      r = LinkReceiver(panel: p, timeout: timeout);
      url = await r.start(InternetAddress.loopbackIPv4);
    }

    tearDown(() => r.dispose());

    test('shows its forms, with no scripts', () async {
      await up();
      final (code, page, _) = await _req(url);
      expect(code, 200);
      expect(page, contains('<h1>Panel</h1>'));
      expect(page, contains('<input name="x">'));
      expect(page, isNot(contains('<script')));
    });

    test('does a form, sends the browser back to its section, and says how it went once', () async {
      await up();
      final (code, _, to) = await _req(url, form: {'x': 'bad', 'at': 'rules'});
      expect(code, HttpStatus.seeOther);
      expect(to, '/${r.token}#rules');
      expect(p.got.single['x'], 'bad');
      expect(r.lastError, 'no <good>');
      final (_, page, _) = await _req(url);
      expect(page, contains('no &lt;good&gt;'));
      expect(page, contains('class="note bad"'));
      final (_, again, _) = await _req(url);
      expect(again, isNot(contains('no &lt;good&gt;')));
      // it goes on serving
      expect(r.listening, isTrue);
      await _req(url, form: {'x': 'ok', 'at': '"><b>'});
      expect(r.lastMessage, 'done');
      expect(r.lastError, isNull);
    });

    test('a wrong token gets nothing done', () async {
      await up();
      final (code, _, _) = await _req(url.replace(path: '/nottoken'), form: {'x': 'ok'});
      expect(code, 404);
      expect(p.got, isEmpty);
    });

    test('stops once unused for its timeout, not after its first form', () async {
      await up(timeout: const Duration(milliseconds: 600));
      await Future<void>.delayed(const Duration(milliseconds: 400));
      await _req(url, form: {'x': 'ok'});
      await Future<void>.delayed(const Duration(milliseconds: 400));
      expect(r.listening, isTrue);
      await Future<void>.delayed(const Duration(milliseconds: 500));
      expect(r.listening, isFalse);
      expect(r.end, ReceiveEnd.timeout);
    });
  });

  group('PhoneSettings', () {
    setUp(() {
      _fake();
      _calls.clear();
      _prefs.clear();
      _settings = {
        'logLevel': 'info',
        'testUrl': 'https://a.example/ok',
        'ipv6': false,
        'keepAliveInterval': 30,
        'nameservers': ['1.1.1.1'],
        'tailnet': {'on': true, 'hostname': 'tv', 'authKey': 'tskey-old', 'controlUrl': ''},
      };
    });

    test('shows every part on Android, its values escaped', () async {
      _settings['testUrl'] = 'https://a.example/?q="<x>';
      final page = await PhoneSettings(AppState(), android: true).body();
      for (final id in ['prov', 'rules', 'sets', 'tail', 'apps', 'look', 'core']) {
        expect(page, contains('<details id="$id"'), reason: id);
      }
      expect(page, contains('&quot;&lt;x&gt;'));
      expect(page, isNot(contains('"<x>')));
      // a rule goes into a service, never into a rule set
      expect(page, contains('value="ai\topenai"'));
      expect(page, isNot(contains('set1')));
      // the key itself is never shown
      expect(page, isNot(contains('tskey-old')));
    });

    test('opens the part a form was just sent from', () async {
      final page = await PhoneSettings(AppState(), android: true).body(open: 'core');
      expect(page, contains('<details id="core" open>'));
      expect(page, contains('<details id="rules">'));
    });

    test('leaves out what is only Android\'s elsewhere', () async {
      final page = await PhoneSettings(AppState(), android: false).body();
      expect(page, isNot(contains('<details id="tail"')));
      expect(page, isNot(contains('<details id="apps"')));
    });

    test('saves only the core options changed', () async {
      final s = PhoneSettings(AppState(), android: true);
      final out = await s.submit({'do': 'core', 'sec': '0', 'f.logLevel': 'info', 'f.testUrl': 'https://b.example/ok', 'f.ipv6': '1', 'f.keepAliveInterval': '30'});
      expect(out.bad, isFalse);
      expect(_patches().single, {'testUrl': 'https://b.example/ok', 'ipv6': true});
    });

    test('says a number that is not one, and saves nothing', () async {
      final out = await PhoneSettings(AppState(), android: true).submit({'do': 'core', 'sec': '0', 'f.keepAliveInterval': 'soon'});
      expect(out.bad, isTrue);
      expect(_patches(), isEmpty);
    });

    test('keeps the Tailscale key unless one is written or it is cleared', () async {
      final s = PhoneSettings(AppState(), android: true);
      await s.submit({'do': 'tailnet', 'hostname': 'living-room', 'controlUrl': '', 'domain': '', 'authKey': ''});
      expect((_settings['tailnet'] as Map)['authKey'], 'tskey-old');
      expect((_settings['tailnet'] as Map)['hostname'], 'living-room');
      expect((_settings['tailnet'] as Map)['on'], isTrue);
      await s.submit({'do': 'tailnet', 'hostname': 'living-room', 'authKey': '', 'clearKey': '1'});
      expect((_settings['tailnet'] as Map)['authKey'], '');
    });

    test('adds a rule a line, and says the ones not added', () async {
      final out = await PhoneSettings(AppState(), android: true).submit({'do': 'rules', 'target': 'ai\topenai', 'kind': '', 'rules': 'a.com\n\ntaken.com\n b.com '});
      final ops = [for (final c in _calls) if (c.$1 == 'routeOp') c.$2];
      expect([for (final o in ops) o['text']], ['a.com', 'taken.com', 'b.com']);
      expect(ops.first, {'op': 'addRule', 'kind': '', 'text': 'a.com', 'route': 'ai', 'service': 'openai'});
      expect(out.bad, isTrue);
      expect(out.message, contains('taken.com'));
      expect(out.message, contains('Netflix'));
    });

    test('takes a theme color only as #RRGGBB', () async {
      final app = AppState();
      final s = PhoneSettings(app, android: true);
      expect((await s.submit({'do': 'accent', 'accent': 'blue'})).bad, isTrue);
      expect((await s.submit({'do': 'accent', 'accent': '12ab3c'})).bad, isFalse);
      expect(app.accent, '#12AB3C');
    });

    test('takes the apps to leave out, one a line or by commas', () async {
      final app = AppState();
      await PhoneSettings(app, android: true).submit({'do': 'apps', 'apps': 'com.a.b\ncom.c.d, com.e.f\n'});
      expect(app.excludedApps, {'com.a.b', 'com.c.d', 'com.e.f'});
    });

    test('refuses a form it does not know', () async {
      final out = await PhoneSettings(AppState(), android: true).submit({'do': 'nope'});
      expect(out.bad, isTrue);
      expect(out.message, tr('不认识这个表单，请刷新页面', "That form isn't known. Reload the page."));
    });
  });
}
