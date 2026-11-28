import 'dart:io' show Platform;
import 'dart:math' as math;

import 'package:flutter/widgets.dart';

import '../app/i18n.dart';
import '../app/state.dart';
import '../ui/atoms.dart';
import '../ui/skin.dart';
import 'frame.dart';
import 'receive.dart';
import 'sheet.dart';

class SetupPage extends StatelessWidget {
  const SetupPage({super.key, required this.app});

  final AppState app;

  @override
  Widget build(BuildContext context) {
    final l = Skin.of(context);
    final f = Frame.of(context);
    final kb = MediaQuery.viewInsetsOf(context).bottom;
    final bottom = math.max(math.max(30.0, f.barInset + 12), kb + 12);
    final android = Platform.isAndroid;
    final h = l.text(34, weight: 700, display: true, lh: 41);
    final p = l.text(15, color: l.ink2, lh: 22);

    Widget big(String text, BtnKind kind, VoidCallback onTap) => Btn(text, kind: kind, minH: 52, fontSize: 16, expand: true, onTap: onTap);
    Widget half(String text, String icon, VoidCallback onTap) => Expanded(child: Btn(text, expand: true, leading: Ic(icon, size: 17, color: l.ink), onTap: onTap));

    final List<Widget> body;
    if (app.setup == 1) {
      body = [
        eyebrow(l, tr('第 1 步，共 2 步', 'Step 1 of 2')),
        const SizedBox(height: 8),
        Text(tr('添加节点', 'Add nodes'), style: h),
        const SizedBox(height: 8),
        Text(tr('粘贴一个订阅链接、节点链接，或者扫描二维码。', 'Paste a subscription or node link, or scan a QR code.'), style: p),
        const SizedBox(height: 22),
        Container(
          constraints: const BoxConstraints(minHeight: 128),
          padding: const EdgeInsets.all(12),
          decoration: BoxDecoration(color: l.paper2, borderRadius: l.br(12)),
          child: BareField(hint: 'https://…  vmess://…  trojan://…', label: tr('订阅或节点链接', 'Subscription or node link'), controller: app.setupField, onChanged: (v) => app.setupLink = v, maxLines: null, keyboardType: TextInputType.multiline),
        ),
        const SizedBox(height: 10),
        LayoutBuilder(builder: (context, c) {
          final two = [half(tr('扫描二维码', 'Scan QR code'), 'scan', app.scanLink), const SizedBox(width: 8), half(tr('选择文件', 'Choose file'), 'file', app.addFile)];
          final phone = Btn(tr('从手机发送链接', 'Send a link from a phone'), expand: true, onTap: () => receiveSheet(app));
          // One row where it's wide, as on a TV, whose screen is short.
          if (c.maxWidth >= 600) return Row(children: [...two, const SizedBox(width: 8), Expanded(child: phone)]);
          return Column(children: [Row(children: two), const SizedBox(height: 8), phone]);
        }),
        if (app.setupError != null) ...[
          const SizedBox(height: 10),
          Text(app.setupError!, style: l.text(13.5, color: l.signalT, lh: 19)),
        ],
        const Spacer(),
        big(app.setupBusy ? tr('正在添加', 'Adding') : tr('继续', 'Continue'), BtnKind.accent, app.submitSetupLink),
      ];
    } else {
      Widget point(String a, String b) => hairBottom(
            l,
            Container(
              width: double.infinity,
              padding: const EdgeInsets.symmetric(vertical: 12),
              child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Text(a, style: l.text(15, lh: 22)),
                Text(b, style: l.text(13, color: l.mute, lh: 18)),
              ]),
            ),
          );
      body = [
        eyebrow(l, tr('第 2 步，共 2 步', 'Step 2 of 2')),
        const SizedBox(height: 8),
        Text(tr('接管流量', 'Turn on VPN'), style: h),
        const SizedBox(height: 8),
        Text(tr('bowel 通过系统 VPN 接管这台设备的流量，并按规则分流。', 'bowel uses the system VPN to carry this device\'s traffic and split it by rules.'), style: p),
        const SizedBox(height: 22),
        point(tr('系统会询问一次', 'The system asks once'), android ? tr('确认 bowel 可以建立 VPN 连接', 'Confirm that bowel can set up a VPN connection') : tr('允许 bowel 添加 VPN 配置，需要面容或密码', 'Allow bowel to add a VPN configuration, with Face ID or your passcode')),
        point(tr('流量只经过你添加的节点', 'Traffic only goes through nodes you add'), tr('bowel 没有自己的服务器', 'bowel has no servers of its own')),
        point(tr('随时可以关闭', 'Turn it off any time'), android ? tr('App 内、快捷设置磁贴或常驻通知', 'In the app, the Quick Settings tile or the notification') : tr('App 内或系统设置的 VPN 页面', 'In the app or on the VPN page in Settings')),
        const Spacer(),
        big(tr('允许并接管', 'Allow and turn on'), BtnKind.accent, () => app.finishSetup(take: true)),
        const SizedBox(height: 4),
        big(tr('先跳过', 'Skip for now'), BtnKind.plain, () => app.finishSetup(take: false)),
      ];
    }
    // The tabs, which hold the other sheets, aren't up yet: this page has
    // its own, for the receive sheet, and Back closes it.
    return PopScope(
      canPop: app.sheet == null,
      onPopInvokedWithResult: (didPop, _) {
        if (!didPop) app.closeSheet();
      },
      child: Stack(fit: StackFit.expand, children: [
        Padding(
          padding: EdgeInsets.fromLTRB(20, f.top + 26, 20, bottom),
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            if (kb == 0) ...const [LogoTile(size: 48), SizedBox(height: 20)],
            ...body,
          ]),
        ),
        SheetHost(app: app),
      ]),
    );
  }
}
