import 'dart:convert';
import 'dart:io' show Platform;

import '../app/i18n.dart';
import '../app/receive.dart';
import '../app/sample.dart';
import '../app/state.dart';
import '../ui/skin.dart';
import 'options.dart';

const _e = HtmlEscape();
String _h(Object? s) => _e.convert('${s ?? ''}');

List<Map> _list(Object? m, String key) => m is Map ? ((m[key] as List?) ?? const []).cast<Map>() : const [];

/// The settings a phone's browser edits for this device, served by a
/// [LinkReceiver]: what takes typing, which a remote makes slow. Each form
/// does what the same setting does on the device, through the same calls.
class PhoneSettings implements ReceiverPanel {
  PhoneSettings(this.app, {bool? android}) : android = android ?? Platform.isAndroid;

  final AppState app;

  /// Tailscale and the apps left out of the VPN are Android's.
  final bool android;

  /// A whole config sent as a rule set, waiting for its target to be picked.
  ({String link, String route, List<String> targets})? _config;

  /// The part the last form came from, shown open.
  String? _open;

  @override
  String get title => tr('在手机上设置 bowel', 'Set up bowel from a phone');

  @override
  Future<String> body({String? open}) async {
    _open = open;
    await app.refreshRouting();
    final s = await app.ask('settings');
    final settings = s is Map ? s : const {};
    return [
      '<p class="sub">${_h(tr('改动会马上存到这台设备上。这个页面在设备上关掉设置窗口、或一阵子不用后失效。', 'Changes are saved on the device at once. This page stops working once its window on the device closes, or after a while unused.'))}</p>',
      _providers(),
      _rules(),
      _ruleSets(),
      if (android) _tailnet(settings),
      if (android) _apps(),
      _accent(),
      _core(settings),
    ].join();
  }

  // ---- the page ----

  String _section(String id, String title, String inner, {String? sub, bool open = false}) =>
      '<details id="$id"${open || id == _open ? ' open' : ''}><summary>${_h(title)}</summary>${sub == null ? '' : '<p class="sub">${_h(sub)}</p>'}$inner</details>';

  String _form(String what, String at, String inner, String button) =>
      '<form method="post"><input type="hidden" name="do" value="$what"><input type="hidden" name="at" value="$at">$inner<button>${_h(button)}</button></form>';

  String _field(String name, String label, {String value = '', String? hint, String type = 'text', String? note}) =>
      '<label for="$name">${_h(label)}</label><input id="$name" name="$name" type="$type" value="${_h(value)}"${hint == null ? '' : ' placeholder="${_h(hint)}"'} autocapitalize="off" autocomplete="off" spellcheck="false">${note == null ? '' : '<small>${_h(note)}</small>'}';

  String _area(String name, String label, {String value = '', String? hint, int rows = 5, String? note}) =>
      '<label for="$name">${_h(label)}</label><textarea id="$name" name="$name" rows="$rows"${hint == null ? '' : ' placeholder="${_h(hint)}"'} autocapitalize="off" autocomplete="off" spellcheck="false">${_h(value)}</textarea>${note == null ? '' : '<small>${_h(note)}</small>'}';

  String _select(String name, String label, List<(String, String)> options, {String? value, String groups = ''}) {
    final opts = options.map((o) => '<option value="${_h(o.$1)}"${o.$1 == value ? ' selected' : ''}>${_h(o.$2)}</option>').join();
    return '<label for="$name">${_h(label)}</label><select id="$name" name="$name">$opts$groups</select>';
  }

  String _providers() {
    final list = providers.where((p) => p.id.isNotEmpty).toList();
    final add = _form('link', 'prov', _area('link', tr('订阅链接或节点链接', 'Subscription or node link'), hint: 'https://…  vmess://…', note: tr('一行一个，可以一次发多个节点链接', 'One a line: several node links go at once')), tr('添加', 'Add'));
    final rename = list.isEmpty
        ? ''
        : '<h2>${_h(tr('重命名', 'Rename'))}</h2>${list.map((p) => _form('rename', 'prov', '<div class="row"><div>${_field('name', p.name, value: p.name)}</div></div><input type="hidden" name="id" value="${_h(p.id)}">', tr('改名', 'Rename'))).join()}';
    return _section('prov', 'Provider', add + rename, open: list.isEmpty);
  }

  /// The services a rule can go into, by route group.
  String _serviceOptions() {
    final b = StringBuffer();
    for (final g in _list(app.routing, 'groups')) {
      final services = _list(g, 'services').where((s) => s['set'] == null).toList();
      if (services.isEmpty) continue;
      b.write('<optgroup label="${_h(g['name'] ?? g['id'])}">');
      for (final s in services) {
        b.write('<option value="${_h('${g['id']}\t${s['id']}')}">${_h(s['name'] ?? s['id'])}</option>');
      }
      b.write('</optgroup>');
    }
    return '$b';
  }

  String _rules() {
    final kinds = [('', tr('自动识别', 'Detect')), for (final k in _list(app.routing, 'kinds')) (k['id'] as String, k['name'] as String? ?? k['id'] as String)];
    final inner = '${_select('target', tr('加到', 'Add to'), const [], groups: _serviceOptions())}'
        '${_select('kind', tr('类型', 'Kind'), kinds)}'
        '${_area('rules', tr('规则', 'Rules'), hint: 'example.com\n1.2.3.0/24', note: tr('一行一条：域名、IP、链接，或按类型写的值', 'One a line: a domain, an IP, a link, or a value of the kind picked'))}';
    return _section('rules', tr('添加规则', 'Add rules'), _form('rules', 'rules', inner, tr('添加', 'Add')));
  }

  String _ruleSets() {
    final groups = [for (final g in _list(app.routing, 'groups')) (g['id'] as String, g['name'] as String? ?? g['id'] as String)];
    final c = _config;
    if (c != null) {
      final name = groups.where((g) => g.$1 == c.route).firstOrNull?.$2 ?? c.route;
      final pick = '<p class="sub">${_h(tr('这是一份完整配置，不是规则集。可以把其中发往一个目标的规则导入到「{0}」；它的节点、策略组和 DNS 不会带进来。', 'This is a whole config, not a rule set. The rules it sends to one target can be imported into {0}. Its nodes, groups and DNS are left out.', [name]))}</p>'
          '<input type="hidden" name="link" value="${_h(c.link)}"><input type="hidden" name="route" value="${_h(c.route)}">'
          '${_select('target', tr('目标', 'Target'), [for (final t in c.targets) (t, t)])}';
      return _section('sets', tr('导入规则集', 'Import a rule set'), _form('set', 'sets', pick, tr('导入', 'Import')) + _form('setCancel', 'sets', '', tr('换一个链接', 'Use another link')), open: true);
    }
    final inner = '${_select('route', tr('导入到', 'Into'), groups)}${_field('link', tr('规则集链接', 'Rule set link'), hint: 'https://…', type: 'url')}';
    return _section('sets', tr('导入规则集', 'Import a rule set'), _form('set', 'sets', inner, tr('导入', 'Import')));
  }

  String _tailnet(Map settings) {
    final t = (settings['tailnet'] as Map?) ?? const {};
    final key = t['authKey'] as String? ?? '';
    final control = t['controlUrl'] as String? ?? '';
    final inner = '${_field('hostname', tr('设备名', 'Device name'), value: t['hostname'] as String? ?? '', hint: 'bowel-android', note: tr('留空用 bowel-android。已登录时改名会同步到 tailnet。', 'Empty is bowel-android. Renaming while logged in renames it on the tailnet.'))}'
        '${_field('controlUrl', tr('控制服务器', 'Control server'), value: control, hint: 'https://headscale.example.com', type: 'url', note: tr('留空用 Tailscale 官方的服务器。改了会退出登录。', "Empty is Tailscale's own server. Changing it logs out."))}'
        '${_field('domain', tr('MagicDNS 域名', 'MagicDNS domain'), value: t['domain'] as String? ?? '', hint: 'tail.example.com', note: tr('Headscale 配置里的 base_domain', "Headscale's base_domain"))}'
        '${_field('authKey', 'Auth key', type: 'password', hint: key.isEmpty ? (control.isEmpty ? 'tskey-auth-…' : 'hskey-auth-…') : tr('已设置，留空不改', 'Set. Leave empty to keep it'), note: tr('只在第一次登录时用。建议用一次性的 key。', 'Used only for the first login. A one-off key is best.'))}'
        '${key.isEmpty ? '' : '<label class="check"><input type="checkbox" name="clearKey" value="1">${_h(tr('清除已设置的 auth key', 'Clear the auth key set'))}</label>'}';
    return _section('tail', 'Tailscale', _form('tailnet', 'tail', inner, tr('保存', 'Save')));
  }

  String _apps() {
    final ids = app.excludedApps.toList()..sort();
    final inner = _area('apps', tr('包名', 'Package names'), value: ids.join('\n'), hint: 'com.example.app', rows: 6, note: tr('一行一个。这些 App 直接联网，不经过 VPN。', 'One a line. These apps go online directly, not through the VPN.'));
    return _section('apps', tr('不走 bowel 的 App', 'Apps that skip bowel'), _form('apps', 'apps', inner, tr('保存', 'Save')));
  }

  String _accent() {
    final v = app.accent.startsWith('#') ? app.accent : '';
    final inner = _field('accent', tr('主题色', 'Theme color'), value: v, hint: '#RRGGBB', note: tr('留空回到默认颜色', 'Empty goes back to the default color'));
    return _section('look', tr('主题色', 'Theme color'), _form('accent', 'look', inner, tr('保存', 'Save')));
  }

  String _core(Map settings) {
    final sections = coreFieldSections(geo: settings['canUpdateGeo'] == true);
    final b = StringBuffer();
    for (final (i, (title, fields)) in sections.indexed) {
      final inner = StringBuffer('<input type="hidden" name="sec" value="$i">');
      for (final f in fields) {
        final v = settings[f.key];
        final name = 'f.${f.key}';
        final label = f.unit.isEmpty ? f.title : '${f.title}（${f.unit}）';
        switch (f.kind) {
          case 'bool':
            inner.write('<label class="check"><input type="checkbox" name="$name" value="1"${v == true ? ' checked' : ''}>${_h(f.title)}</label>${f.sub == null ? '' : '<small>${_h(f.sub)}</small>'}');
          case 'int':
            inner.write(_field(name, label, value: '${v ?? ''}', type: 'number', note: f.sub));
          case 'pick':
            inner.write(_select(name, f.title, f.choices, value: '${v ?? ''}'));
          case 'list':
            inner.write(_area(name, f.title, value: ((v as List?) ?? const []).join('\n'), note: f.sub ?? tr('每行一条', 'One per line')));
          default:
            inner.write(_field(name, f.title, value: '${v ?? ''}', note: f.sub));
        }
      }
      b.write('<h2>${_h(title)}</h2>${_form('core', 'core', '$inner', tr('保存「{0}」', 'Save {0}', [title]))}');
    }
    return _section('core', tr('内核选项', 'Core options'), '$b', sub: tr('只有改过的项会存', 'Only what was changed is saved'));
  }

  // ---- the forms ----

  static ({String message, bool bad}) _ok(String m) => (message: m, bad: false);
  static ({String message, bool bad}) _no(String m) => (message: m, bad: true);

  @override
  Future<({String message, bool bad})> submit(Map<String, String> form) async {
    switch (form['do']) {
      case 'link':
        final link = cleanLink(form['link'] ?? '');
        if (link == null) return _no(tr('这不是订阅链接或节点链接', "That isn't a subscription or node link"));
        final err = await app.receiveLink(link);
        return err == null ? _ok(tr('已添加', 'Added')) : _no(tr('没有添加：{0}', 'Not added: {0}', [err]));
      case 'rename':
        final p = providers.where((p) => p.id == form['id']).firstOrNull;
        final name = (form['name'] ?? '').trim();
        if (p == null) return _no(tr('这个 Provider 已经不在了', 'That Provider is gone'));
        if (name.isEmpty) return _no(tr('名字不能为空', 'The name cannot be empty'));
        return await app.renameProvider(p, name) ? _ok(tr('已改名为「{0}」', 'Renamed to {0}', [name])) : _no(tr('没有改名', 'Not renamed'));
      case 'rules':
        return _addRules(form);
      case 'set':
        return _importSet(form);
      case 'setCancel':
        _config = null;
        return _ok(tr('可以换一个链接了', 'Send another link'));
      case 'tailnet':
        return _saveTailnet(form);
      case 'apps':
        final ids = {for (final x in (form['apps'] ?? '').split(RegExp(r'[\s,，、]+'))) if (x.trim().isNotEmpty) x.trim()};
        app.setExcluded(ids);
        return _ok(ids.isEmpty ? tr('所有 App 都走 bowel', 'Every app goes through bowel') : tr('已保存，{0} 个 App 不走 bowel', 'Saved: {0} apps skip bowel', [ids.length]));
      case 'accent':
        var hex = (form['accent'] ?? '').trim().toUpperCase();
        if (hex.isEmpty) {
          app.setAccent('');
          return _ok(tr('已回到默认颜色', 'Back to the default color'));
        }
        if (!hex.startsWith('#')) hex = '#$hex';
        if (!validAccent(hex)) return _no(tr('主题色要写成 #RRGGBB', 'The theme color must be #RRGGBB'));
        app.setAccent(hex);
        return _ok(tr('主题色已改为 {0}', 'Theme color is now {0}', [hex]));
      case 'core':
        return _saveCore(form);
    }
    return _no(tr('不认识这个表单，请刷新页面', "That form isn't known. Reload the page."));
  }

  Future<({String message, bool bad})> _addRules(Map<String, String> form) async {
    final target = (form['target'] ?? '').split('\t');
    if (target.length != 2) return _no(tr('先选要加到哪里', 'Pick where to add them first'));
    final lines = [for (final x in (form['rules'] ?? '').split('\n')) if (x.trim().isNotEmpty) x.trim()];
    if (lines.isEmpty) return _no(tr('没有写规则', 'No rules were written'));
    final failed = <String>[];
    var added = 0;
    for (final text in lines) {
      final r = await app.addServiceRule({'kind': form['kind'] ?? '', 'text': text, 'route': target[0], 'service': target[1]});
      if (r.error == null && r.conflict == null) {
        added++;
      } else {
        failed.add(r.conflict != null ? tr('{0}：已经在「{1}」里', '{0}: already in {1}', [text, r.conflict]) : '$text：${r.error}');
      }
    }
    if (failed.isEmpty) return _ok(tr('已添加 {0} 条规则', 'Added {0} rules', [added]));
    return _no('${tr('添加了 {0} 条，{1} 条没有添加', 'Added {0}, {1} not added', [added, failed.length])}\n${failed.join('\n')}');
  }

  Future<({String message, bool bad})> _importSet(Map<String, String> form) async {
    final link = (form['link'] ?? '').trim();
    final route = form['route'] ?? '';
    if (link.isEmpty || route.isEmpty) return _no(tr('写上规则集链接', 'Write the rule set link'));
    final args = {'input': link, 'path': '', 'text': form['target'] ?? '', 'route': route};
    final check = await app.checkRuleSet(args);
    if (check.error != null) return _no(check.error!);
    final targets = _list(check.out, 'targets');
    if (targets.isNotEmpty) {
      _config = (link: link, route: route, targets: [for (final t in targets) t['name'] as String]);
      return _ok(tr('这是一份完整配置，选一个目标再导入', 'This is a whole config: pick a target, then import'));
    }
    final out = check.out;
    if (out?['dup'] != null) return _no(tr('已经导入过：{0}', 'Already imported: {0}', [out!['dup']]));
    if ((out?['count'] as int? ?? 0) == 0) return _no(tr('里面没有能用的规则', 'It holds no usable rules'));
    final r = await app.importRuleSet(args);
    if (r.error != null) return _no(r.error!);
    _config = null;
    return _ok(tr('已导入 {0} 条规则', 'Imported {0} rules', [out!['count']]));
  }

  Future<({String message, bool bad})> _saveTailnet(Map<String, String> form) async {
    final s = await app.ask('settings');
    final had = s is Map ? ((s['tailnet'] as Map?) ?? const {}) : const {};
    final key = (form['authKey'] ?? '').trim();
    final patch = {
      ...had,
      'hostname': (form['hostname'] ?? '').trim(),
      'controlUrl': (form['controlUrl'] ?? '').trim(),
      'domain': (form['domain'] ?? '').trim(),
      if (key.isNotEmpty) 'authKey': key else if (form['clearKey'] == '1') 'authKey': '',
    };
    return await app.send('setSettings', {'patch': {'tailnet': patch}}) ? _ok(tr('Tailscale 设置已保存', 'Tailscale settings saved')) : _no(tr('没有保存', 'Not saved'));
  }

  Future<({String message, bool bad})> _saveCore(Map<String, String> form) async {
    final s = await app.ask('settings');
    if (s is! Map) return _no(tr('读不到内核选项', "Couldn't read the core options"));
    final sections = coreFieldSections(geo: s['canUpdateGeo'] == true);
    final i = int.tryParse(form['sec'] ?? '');
    if (i == null || i < 0 || i >= sections.length) return _no(tr('不认识这个表单，请刷新页面', "That form isn't known. Reload the page."));
    final patch = <String, Object?>{};
    for (final f in sections[i].$2) {
      final raw = form['f.${f.key}'];
      final had = s[f.key];
      // each compared as the page showed it, so what wasn't touched stays
      final Object? v, was;
      switch (f.kind) {
        case 'bool':
          (v, was) = (raw == '1', had == true);
        case 'int':
          final t = (raw ?? '').trim();
          if (t.isEmpty && had == null) continue;
          final n = int.tryParse(t);
          if (n == null) return _no(tr('「{0}」要写数字', '{0} must be a number', [f.title]));
          (v, was) = (n, had);
        case 'pick':
          if (!f.choices.any((c) => c.$1 == raw)) continue;
          (v, was) = (raw, '${had ?? ''}');
        case 'list':
          (v, was) = ([for (final x in (raw ?? '').split('\n')) if (x.trim().isNotEmpty) x.trim()], (had as List?) ?? const []);
        default:
          (v, was) = ((raw ?? '').trim(), '${had ?? ''}');
      }
      if (jsonEncode(v) != jsonEncode(was)) patch[f.key] = v;
    }
    if (patch.isEmpty) return _ok(tr('没有改动', 'Nothing changed'));
    return await app.send('setSettings', {'patch': patch}) ? _ok(tr('已保存 {0} 项', 'Saved {0}', [patch.length])) : _no(tr('没有保存', 'Not saved'));
  }
}
