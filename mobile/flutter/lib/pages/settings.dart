import 'dart:io' show Platform;

import 'package:flutter/widgets.dart';

import '../app/i18n.dart';
import '../app/shell.dart';
import '../app/state.dart';
import '../ui/atoms.dart';
import '../ui/skin.dart';
import '../ui/skin_data.dart';
import 'frame.dart';
import 'options.dart';
import 'routing.dart';
import 'rows.dart';
import 'sheet.dart';

/// 设置, with its own stack of pages: swiping back stays in this tab, and the
/// system tab bar stays over them.
class SettingsPage extends StatefulWidget {
  const SettingsPage({super.key, required this.app});

  final AppState app;

  @override
  State<SettingsPage> createState() => _SettingsPageState();
}

class _SettingsPageState extends State<SettingsPage> {
  @override
  void initState() {
    super.initState();
    final app = widget.app;
    app.openGroupPage = (id, {service}) {
      final nav = app.settingsNav.currentState;
      if (nav == null) return;
      nav.popUntil((r) => r.isFirst);
      nav.push(routeOf(RoutesPage(app: app)));
      nav.push(routeOf(GroupPage(app: app, id: id, service: service)));
    };
    app.openRoutesPage = () {
      final nav = app.settingsNav.currentState;
      if (nav == null) return;
      nav.popUntil((r) => r.isFirst);
      nav.push(routeOf(RoutesPage(app: app)));
    };
  }

  @override
  Widget build(BuildContext context) {
    return Navigator(
      key: widget.app.settingsNav,
      onGenerateRoute: (_) => routeOf(_SettingsHome(app: widget.app)),
    );
  }
}

class _SettingsHome extends StatefulWidget {
  const _SettingsHome({required this.app});
  final AppState app;

  @override
  State<_SettingsHome> createState() => _SettingsHomeState();
}

class _SettingsHomeState extends State<_SettingsHome> {
  Widget _skins(BuildContext context, Look l) {
    final dark = MediaQuery.platformBrightnessOf(context) == Brightness.dark;
    final ids = skinDefs.keys.toList();
    return Padding(
      padding: const EdgeInsets.only(top: 10, bottom: 6),
      child: LayoutBuilder(builder: (context, c) {
        final w = (c.maxWidth - 16) / 3;
        return Wrap(spacing: 8, runSpacing: 8, children: [
          for (final id in ids) SizedBox(width: w, child: _skin(l, id, Look.of(id, dark))),
        ]);
      }),
    );
  }

  Widget _skin(Look l, String id, Look p) {
    final on = widget.app.skin == id;
    return Tap(
      onTap: () => widget.app.setSkin(id),
      label: skinName(id),
      child: Container(
        constraints: const BoxConstraints(minHeight: 66),
        padding: const EdgeInsets.all(9),
        decoration: BoxDecoration(
          color: p.paper,
          borderRadius: BorderRadius.circular(10),
          border: Border.all(color: on ? l.ink : l.hair, width: on ? 2 : 1),
        ),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Row(children: [
            for (final c in [p.accent, p.route('default'), p.route('media')]) ...[Square(c, size: 11), const SizedBox(width: 3)],
          ]),
          const SizedBox(height: 7),
          Text(skinName(id), style: p.text(13, weight: 600, display: true, lh: 18)),
        ]),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final app = widget.app;
    return ListenableBuilder(
      listenable: app,
      builder: (context, _) {
        final l = Skin.of(context);
        final android = Platform.isAndroid;
        final out = app.vpnExcluded.split(',').where((s) => s.isNotEmpty).length;
        return ColoredBox(
          color: l.paper,
          child: ListPage(
            title: tr('设置', 'Settings'),
            marginTop: 0,
            list: (bottom) => ListView(padding: EdgeInsets.only(bottom: bottom), children: [
              section(l, tr('接管', 'Connection'), top: 8, [
                toggleRow(l, 'VPN', app.on, app.toggleTake, sub: app.on ? tr('生效中', 'Active') : tr('已关闭', 'Off')),
                if (android) ...[
                  navRow(l, tr('分应用代理', 'Per-app proxy'), out == 0 ? tr('全部 App', 'All apps') : tr('{0} 个不走', '{0} excluded', [out]), sub: tr('选择哪些 App 不走 bowel', 'Choose which apps skip bowel'), onTap: () => pushPage(context, AppsPage(app: app))),
                  navRow(l, tr('始终开启 VPN', 'Always-on VPN'), tr('系统设置', 'System Settings'), sub: tr('系统级，开机即接管', 'System-wide, on from startup'), onTap: Shell.instance.openVPNSettings),
                ] else
                  toggleRow(l, tr('断线后自动接管', 'Reconnect automatically'), app.vpnOnDemand, () => app.setOnDemand(!app.vpnOnDemand), sub: tr('网络恢复或切换后自动重连，下次开启 VPN 时生效', 'Reconnects when the network returns or changes. Applies the next time VPN turns on.')),
              ]),
              section(l, tr('外观', 'Appearance'), [_skins(context, l)]),
              section(l, tr('路由与规则', 'Routes and rules'), [
                navRow(l, tr('路由组', 'Route groups'), tr('{0} 个', '{0}', [app.ruleTargets.length]), onTap: () => pushPage(context, RoutesPage(app: app))),
                navRow(l, tr('我的规则', 'My rules'), tr('{0} 条', '{0}', [app.userRules]), sub: tr('让某个域名、关键词或 IP 段走指定路由', 'Send a domain, keyword or IP range through a chosen route'), onTap: () => pushPage(context, MyRulesPage(app: app))),
              ]),
              section(l, tr('通用', 'General'), [
                navRow(l, tr('语言', 'Language'), languageNames[language]!, onTap: () => pickSheet<String>(app, tr('语言', 'Language'), [for (final c in languages) PickOption(c, languageNames[c]!)], app.setLanguage, value: language)),
                navRow(l, tr('内核选项', 'Core options'), app.coreText, onTap: () => pushPage(context, OptionsPage(app: app))),
                navRow(l, tr('关于与开源许可', 'About and licenses'), '0.1.0', onTap: () => pushPage(context, AboutPage(app: app))),
              ]),
              const SizedBox(height: 18),
            ]),
          ),
        );
      },
    );
  }
}
