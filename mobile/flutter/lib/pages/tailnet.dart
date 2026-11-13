import 'dart:async';

import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

import '../app/i18n.dart';
import '../app/shell.dart';
import '../app/state.dart';
import '../ui/atoms.dart';
import '../ui/skin.dart';
import 'rows.dart';
import 'sheet.dart';

/// The core's own Tailscale, on Android: the tailnet without the Tailscale
/// app, which can't be the VPN at the same time as bowel.
class TailnetPage extends StatefulWidget {
  const TailnetPage({super.key, required this.app});
  final AppState app;

  @override
  State<TailnetPage> createState() => _TailnetPageState();
}

class _TailnetPageState extends State<TailnetPage> {
  Map? _s; // settings
  Map? _t; // the node's state
  bool _busy = false;
  Timer? _poll;

  AppState get app => widget.app;
  Map get _tailnet => (_s?['tailnet'] as Map?) ?? const {};
  bool get _on => _tailnet['on'] == true;

  @override
  void initState() {
    super.initState();
    _load();
    // The node logs in, or not, while the page is open.
    _poll = Timer.periodic(const Duration(seconds: 2), (_) => _loadState());
  }

  @override
  void dispose() {
    _poll?.cancel();
    super.dispose();
  }

  Future<void> _load() async {
    final s = await app.ask('settings');
    if (s is Map && mounted) setState(() => _s = s);
    await _loadState();
  }

  Future<void> _loadState() async {
    final t = await app.ask('tailnet');
    if (t is Map && mounted) setState(() => _t = t);
  }

  Future<void> _set(Map<String, Object?> patch) async {
    await app.send('setSettings', {'patch': {'tailnet': {..._tailnet, ...patch}}});
    await _load();
  }

  Future<void> _login() async {
    if (!app.on) {
      app.say(tr('先开启 VPN', 'Turn the VPN on first'));
      return;
    }
    setState(() => _busy = true);
    final t = await app.ask('tailnetLogin');
    if (!mounted) return;
    setState(() {
      _busy = false;
      if (t is Map) _t = t;
    });
    final url = (t is Map ? t['loginUrl'] : null) as String? ?? '';
    if (url.isNotEmpty) await _open(url);
    if (t is Map && t['state'] == 'on') app.say(tr('已登录', 'Logged in'));
  }

  Future<void> _open(String url) async {
    try {
      await Shell.instance.openURL(url);
    } catch (_) {
      await Clipboard.setData(ClipboardData(text: url));
      app.say(tr('没有可用的浏览器，链接已复制', 'No browser to open it: the link is copied'), ms: 3000);
    }
  }

  String _stateText() {
    if (!_on) return tr('未开启', 'Off');
    return switch (_t?['state']) {
      'idle' => app.on ? tr('访问 tailnet 时启动', 'Starts when the tailnet is used') : tr('VPN 开启后可用', 'Available once VPN is on'),
      'starting' => tr('连接中', 'Connecting'),
      'login' => tr('需要登录', 'Needs login'),
      'on' => tr('已登录', 'Logged in'),
      'error' => tr('出错了', 'Failed'),
      _ => '',
    };
  }

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: app,
      builder: (context, _) {
        final l = Skin.of(context);
        if (_s == null) return SubPage(title: 'Tailscale', children: const []);
        final note = l.text(13, color: l.mute, lh: 18);
        final state = _t?['state'] as String? ?? 'off';
        final url = _t?['loginUrl'] as String? ?? '';
        final err = _t?['error'] as String? ?? '';
        final host = _tailnet['hostname'] as String? ?? '';
        final key = _tailnet['authKey'] as String? ?? '';
        return SubPage(title: 'Tailscale', children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 4, 16, 6),
            child: Text(
                tr('不装官方 App 也能访问 tailnet：100.x 地址和 *.ts.net 名字在所有模式下都走 bowel 内置的 Tailscale 节点，其余流量照旧。Android 同时只能有一个 VPN，开启前请关掉官方 Tailscale App。',
                    'Reach your tailnet without the Tailscale app: 100.x addresses and *.ts.net names go by bowel\'s built-in Tailscale node in every mode, and the rest as before. Android allows one VPN at a time, so turn the Tailscale app off first.'),
                style: note),
          ),
          section(l, tr('节点', 'Node'), top: 4, [
            toggleRow(l, tr('内置 Tailscale', 'Built-in Tailscale'), _on, () => _set({'on': !_on}), sub: _stateText()),
            if (_on) ...[
              navRow(l, tr('设备名', 'Device name'), host.isEmpty ? (_t?['hostname'] as String? ?? '') : host,
                  sub: tr('在 tailnet 里显示的名字', 'The name it has on the tailnet'),
                  onTap: () => inputSheet(app, tr('设备名', 'Device name'), (v) => _set({'hostname': v.trim()}), initial: host, hint: 'bowel-android', note: tr('留空用 bowel-android。已登录时改名会同步到 tailnet。', 'Empty is bowel-android. Renaming while logged in renames it on the tailnet.'))),
              navRow(l, 'Auth key', key.isEmpty ? tr('未设置', 'Not set') : tr('已设置', 'Set'),
                  sub: tr('可选：不用浏览器登录', 'Optional: log in without a browser'),
                  onTap: () => inputSheet(app, 'Auth key', (v) => _set({'authKey': v.trim()}), initial: key, hint: 'tskey-auth-…', keyboard: TextInputType.visiblePassword, note: tr('只在第一次登录时用；之后登录状态保存在 bowel 里。建议用一次性的 key。', 'Used only for the first login; after that bowel keeps the login. A one-off key is best.'))),
            ],
          ]),
          if (_on)
            section(l, tr('登录', 'Login'), [
              if (state == 'login' && url.isNotEmpty) ...[
                const SizedBox(height: 6),
                Text(tr('在浏览器里打开下面的链接，批准这台设备加入 tailnet。', 'Open this link in a browser and approve this device on the tailnet.'), style: note),
                const SizedBox(height: 6),
                Tap(
                  onTap: () async {
                    await Clipboard.setData(ClipboardData(text: url));
                    app.say(tr('已复制', 'Copied'));
                  },
                  label: tr('复制链接', 'Copy the link'),
                  child: Text(url, style: l.text(13, color: l.ink2, lh: 18)),
                ),
              ],
              if (state == 'error' && err.isNotEmpty) Padding(padding: const EdgeInsets.only(top: 6), child: Text(err, style: l.text(13, color: l.ink2, lh: 18))),
              Padding(
                padding: const EdgeInsets.only(top: 10),
                child: Row(children: [
                  if (state != 'on')
                    Btn(state == 'login' && url.isNotEmpty ? tr('打开登录链接', 'Open the login link') : tr('登录', 'Log in'), minH: 40, kind: BtnKind.accent, onTap: _busy ? null : (state == 'login' && url.isNotEmpty ? () => _open(url) : _login)),
                  if (state != 'on') const SizedBox(width: 8),
                  Btn(tr('退出登录', 'Log out'), minH: 40, kind: BtnKind.plain, onTap: () => confirmSheet(app, tr('退出 Tailscale 登录', 'Log out of Tailscale'), tr('退出', 'Log out'), () async {
                        await app.send('tailnetLogout');
                        await _load();
                      }, body: tr('这台设备会忘掉登录状态，下次要重新登录。在 Tailscale 后台还能看到它，需要的话在那里删除。', 'This device forgets its login and will log in anew. It stays in the Tailscale admin console: remove it there if you want.'))),
                ]),
              ),
            ]),
          const SizedBox(height: 18),
        ]);
      },
    );
  }
}
