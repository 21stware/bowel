import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math';
import 'dart:typed_data';

import 'package:flutter/foundation.dart';

import 'i18n.dart';

/// Why a [LinkReceiver] is no longer listening.
enum ReceiveEnd { added, closed, timeout, locked, failed }

/// A small web server on the local network that takes one subscription or
/// node link from another device's browser: a phone scans the address this
/// device shows, pastes a link and sends it, and [onLink] adds it.
///
/// It listens only on the LAN address it is given, on a port the system
/// picks, and answers only on a path holding a random token, so nothing
/// else on the network can send it anything without seeing the screen. It
/// takes only link text, a few KB at most, and stops after a link is added,
/// on [stop], after [timeout], and after [maxMisses] requests for any other
/// path, so the token can't be guessed by trying.
class LinkReceiver extends ChangeNotifier {
  LinkReceiver({required this.onLink, this.timeout = const Duration(minutes: 5), this.maxBody = 16 * 1024, this.maxMisses = 20});

  /// Adds a link the way a pasted one is added: null when it was added,
  /// otherwise why not, which the sending page shows.
  final Future<String?> Function(String link) onLink;
  final Duration timeout;

  /// The most a request's body may hold, in bytes.
  final int maxBody;
  final int maxMisses;

  /// The path's secret: 8 characters from 31 that read unambiguously, about
  /// 40 bits, short enough to type off a TV. With [maxMisses] tries before
  /// the receiver stops, a guess succeeds once in 50 billion.
  final String token = _newToken();

  HttpServer? _server;
  Timer? _timer;
  Uri? _url;
  bool _adding = false;
  bool _expired = false;
  int _misses = 0;
  ReceiveEnd? _end;

  /// The address to open, once started.
  Uri? get url => _url;
  bool get listening => _server != null;

  /// Whether a link that came in is being added now.
  bool get adding => _adding;
  ReceiveEnd? get end => _end;

  /// The last link that came in and wasn't added, and why.
  String? lastError;

  static const _alphabet = 'abcdefghjkmnpqrstuvwxyz23456789';

  static String _newToken() {
    final r = Random.secure();
    return String.fromCharCodes(List.generate(8, (_) => _alphabet.codeUnitAt(r.nextInt(_alphabet.length))));
  }

  /// Starts listening on [address], which [pickLanAddress] chose.
  Future<Uri> start(InternetAddress address) async {
    if (_server != null || _end != null) throw StateError('a receiver starts once');
    final server = await HttpServer.bind(address, 0);
    server.idleTimeout = const Duration(seconds: 20);
    server.autoCompress = false;
    _server = server;
    final host = address.type == InternetAddressType.IPv6 ? '[${address.address}]' : address.address;
    _url = Uri.parse('http://$host:${server.port}/$token');
    _timer = Timer(timeout, _expire);
    server.listen((req) => _handle(req).catchError((Object _) {}), onError: (Object _) {});
    _changed();
    return _url!;
  }

  /// Stops listening. Safe to call at any time and more than once.
  Future<void> stop([ReceiveEnd why = ReceiveEnd.closed]) async {
    _timer?.cancel();
    _timer = null;
    final s = _server;
    _server = null;
    if (s == null) return;
    _end ??= why;
    _changed();
    await s.close(force: true);
  }

  void _expire() {
    // A link being added finishes first; its answer stops the receiver.
    if (_adding) {
      _expired = true;
      return;
    }
    stop(ReceiveEnd.timeout);
  }

  Future<void> _handle(HttpRequest req) async {
    final res = req.response;
    res.headers
      ..set(HttpHeaders.cacheControlHeader, 'no-store')
      ..set('X-Content-Type-Options', 'nosniff')
      ..set('Referrer-Policy', 'no-referrer')
      ..set('Content-Security-Policy', "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'");
    if (req.uri.path != '/$token') {
      // Browsers ask for an icon on their own; that is no guess.
      final guess = req.uri.path != '/favicon.ico';
      await _answer(res, 404, _page(message: tr('这个地址已失效，请在设备上重新打开。', 'This address no longer works. Open it again on the device.'), bad: true, form: false));
      if (guess && ++_misses >= maxMisses) await stop(ReceiveEnd.locked);
      return;
    }
    switch (req.method) {
      case 'GET' || 'HEAD':
        return _answer(res, 200, _page());
      case 'POST':
        return _post(req);
    }
    res.headers.set(HttpHeaders.allowHeader, 'GET, POST');
    return _answer(res, 405, _page(message: tr('这里只收订阅链接或节点链接', 'This only takes a subscription or node link'), bad: true));
  }

  Future<void> _post(HttpRequest req) async {
    final res = req.response;
    final tooLong = _page(message: tr('内容太长，这里只收订阅链接或节点链接', 'Too long: this only takes a subscription or node link'), bad: true);
    if (req.contentLength > maxBody) return _answer(res, 413, tooLong);
    final mime = req.headers.contentType?.mimeType;
    if (mime != 'application/x-www-form-urlencoded' && mime != 'text/plain') {
      return _answer(res, 415, _page(message: tr('这里只收订阅链接或节点链接', 'This only takes a subscription or node link'), bad: true));
    }
    // A body that never said its length is read up to the limit, then held
    // while the refusal goes out: cancelling first would drop the answer.
    final body = BytesBuilder(copy: false);
    final over = Completer<bool>();
    late final StreamSubscription<Uint8List> sub;
    sub = req.listen(
      (chunk) {
        body.add(chunk);
        if (body.length > maxBody && !over.isCompleted) {
          sub.pause();
          over.complete(true);
        }
      },
      onDone: () => over.isCompleted ? null : over.complete(false),
      onError: (Object e) => over.isCompleted ? null : over.completeError(e),
      cancelOnError: true,
    );
    if (await over.future) {
      res.persistentConnection = false;
      await _answer(res, 413, tooLong);
      await sub.cancel();
      return;
    }
    final String raw;
    try {
      final text = utf8.decode(body.takeBytes());
      raw = mime == 'text/plain' ? text : Uri.splitQueryString(text, encoding: utf8)['link'] ?? '';
    } on FormatException {
      return _answer(res, 400, _page(message: tr('这里只收订阅链接或节点链接', 'This only takes a subscription or node link'), bad: true));
    }
    final link = cleanLink(raw);
    if (link == null) {
      return _answer(res, 400, _page(message: tr('这不是订阅链接或节点链接', "That isn't a subscription or node link"), bad: true, link: raw.length <= 2048 ? raw : ''));
    }
    if (_adding || _server == null) {
      return _answer(res, 409, _page(message: tr('正在添加上一个链接，请稍等', 'Still adding the last link. Wait a moment.'), bad: true, link: link));
    }
    _adding = true;
    lastError = null;
    _changed();
    String? err;
    try {
      err = await onLink(link);
    } catch (e) {
      err = '$e';
    }
    _adding = false;
    if (err == null) {
      await _answer(res, 200, _page(message: tr('已添加到 bowel，可以关掉这个页面了。', 'Added to bowel. You can close this page.'), form: false));
      await stop(ReceiveEnd.added);
      return;
    }
    lastError = err;
    _changed();
    await _answer(res, 422, _page(message: tr('没有添加：{0}', 'Not added: {0}', [err]), bad: true, link: link));
    if (_expired) await stop(ReceiveEnd.timeout);
  }

  Future<void> _answer(HttpResponse res, int status, String html) async {
    res.statusCode = status;
    res.headers.contentType = ContentType.html;
    res.write(html);
    await res.close();
  }

  /// The sending page: a form with one field, no scripts, nothing fetched.
  String _page({String? message, bool bad = false, bool form = true, String link = ''}) {
    const e = HtmlEscape();
    final note = message == null ? '' : '<p class="${bad ? 'bad' : 'ok'}">${e.convert(message)}</p>';
    final fields = !form
        ? ''
        : '<form method="post"><label for="l">${e.convert(tr('订阅链接或节点链接', 'Subscription or node link'))}</label>'
            '<textarea id="l" name="link" rows="5" maxlength="$maxBody" required autofocus autocapitalize="off" autocomplete="off" spellcheck="false" placeholder="https://…  vmess://…">${e.convert(link)}</textarea>'
            '<button>${e.convert(tr('发送到这台设备', 'Send to this device'))}</button></form>';
    return '<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">'
        '<title>bowel</title><style>'
        'body{font:16px/1.45 system-ui,sans-serif;margin:0;padding:24px 16px;background:#f6f4ef;color:#1d1c1a}'
        'main{max-width:480px;margin:0 auto}h1{font-size:22px;margin:0 0 6px}'
        'label{display:block;margin:16px 0 6px;color:#5d5a55}'
        'textarea{box-sizing:border-box;width:100%;font:15px/1.4 ui-monospace,monospace;padding:10px;border:1px solid #ccc8bf;border-radius:10px;background:#fff;color:inherit}'
        'button{margin-top:12px;width:100%;padding:13px;font:inherit;font-weight:600;border:0;border-radius:10px;background:#1d1c1a;color:#fff}'
        '.ok{color:#22703a}.bad{color:#b3261e}'
        '@media(prefers-color-scheme:dark){body{background:#161513;color:#eceae4}textarea{background:#22211e;border-color:#3a3833}button{background:#eceae4;color:#161513}.ok{color:#7fd197}.bad{color:#ff8a80}label{color:#aaa69d}}'
        '</style></head><body><main><h1>${e.convert(tr('发送到 bowel', 'Send to bowel'))}</h1>$note$fields</main></body></html>';
  }

  bool _disposed = false;

  void _changed() {
    if (!_disposed) notifyListeners();
  }

  /// Stops it too: the sheet that shows it going away ends it.
  @override
  void dispose() {
    _disposed = true;
    stop();
    super.dispose();
  }
}

final _linkWord = RegExp(r'^[a-z][a-z0-9+.\-]{0,15}://\S+$', caseSensitive: false);

/// The links in what was sent, one a line, or null unless it is all links:
/// a subscription's address or node links, nothing else.
@visibleForTesting
String? cleanLink(String raw) {
  final words = raw.trim().split(RegExp(r'\s+'));
  if (words.first.isEmpty || words.length > 500) return null;
  if (!words.every(_linkWord.hasMatch)) return null;
  return words.join('\n');
}

/// The address other devices on the network reach this one at: a Wi-Fi or
/// Ethernet IPv4 address, never the VPN's.
///
/// The interfaces come from Dart's NetworkInterface.list, which calls
/// bionic's getifaddrs. Go's net.Interfaces can't be used: it asks netlink
/// for every link (RTM_GETLINK), which Android 11 and later refuse to apps,
/// while bionic's getifaddrs reads the addresses without that request.
Future<InternetAddress?> lanAddress() async {
  try {
    return pickLanAddress(await NetworkInterface.list(type: InternetAddressType.IPv4));
  } catch (_) {
    return null;
  }
}

// Tunnels, VPNs and cellular data: not where a phone beside the TV reaches it.
final _notLan = RegExp(r'^(lo|tun|tap|ppp|ipsec|wg|utun|rmnet|ccmni|pdp|clat|v4-|dummy|docker|veth|p2p)', caseSensitive: false);
final _wired = RegExp(r'^(wlan|eth|en|wl|wifi)', caseSensitive: false);
final _hotspot = RegExp(r'^(ap|swlan|softap|wlan\d+ap)', caseSensitive: false);

/// The best of [interfaces]' addresses for [lanAddress]: Wi-Fi or Ethernet
/// before a hotspot before anything else, private addresses first. The
/// VPN's interface is left out by name and its addresses by range (bowel's
/// TUN is 198.18.0.1/30, inside the benchmark range its fake IPs use), as
/// are loopback, link-local and CGNAT ones, Tailscale's among them.
@visibleForTesting
InternetAddress? pickLanAddress(List<NetworkInterface> interfaces) {
  int? rank(NetworkInterface i, InternetAddress a) {
    if (a.type != InternetAddressType.IPv4 || a.isLoopback || a.isLinkLocal || a.isMulticast) return null;
    final b = a.rawAddress;
    if (b[0] == 198 && (b[1] & 0xfe) == 18) return null; // 198.18.0.0/15
    if (b[0] == 100 && (b[1] & 0xc0) == 64) return null; // 100.64.0.0/10
    if (b[0] == 0 || b[0] >= 224) return null;
    if (_notLan.hasMatch(i.name)) return null;
    final private = b[0] == 10 || b[0] == 172 && (b[1] & 0xf0) == 16 || b[0] == 192 && b[1] == 168;
    final kind = _hotspot.hasMatch(i.name) ? 1 : (_wired.hasMatch(i.name) ? 0 : 2);
    return kind * 2 + (private ? 0 : 1);
  }

  InternetAddress? best;
  var bestRank = 1 << 30;
  for (final i in interfaces) {
    for (final a in i.addresses) {
      final r = rank(i, a);
      if (r != null && r < bestRank) {
        best = a;
        bestRank = r;
      }
    }
  }
  return best;
}
