import 'dart:async';
import 'dart:io' show InternetAddress;

import 'package:flutter/widgets.dart';

import '../app/core.dart';
import '../app/i18n.dart';
import '../app/receive.dart';
import '../app/state.dart';
import '../ui/atoms.dart';
import '../ui/skin.dart';

/// Opens the sheet that takes a link from a phone: its browser opens the
/// address this device shows and sends a link, added as a pasted one is.
/// The receiver runs only while the sheet is up.
void receiveSheet(AppState app) => app.showSheet((_) => ReceivePanel(app: app));

/// A QR code's rows of 0 and 1 from the bridge, null where it can't make one.
Future<List<String>?> bridgeQR(String text) async {
  try {
    final out = await Core.instance.call('qr', {'input': text});
    final rows = out is Map ? out['rows'] : null;
    return rows is List && rows.isNotEmpty ? rows.cast<String>() : null;
  } catch (_) {
    return null;
  }
}

class ReceivePanel extends StatefulWidget {
  const ReceivePanel({super.key, required this.app, this.address = lanAddress, this.qr = bridgeQR, this.timeout = const Duration(minutes: 5), this.inline = false});

  final AppState app;

  /// On a page instead of in a sheet: a TV's empty Provider list shows it
  /// from the start. Nothing to close then, and it grows with what it holds.
  final bool inline;

  /// Where to listen; [lanAddress] but in tests.
  final Future<InternetAddress?> Function() address;
  final Future<List<String>?> Function(String text) qr;
  final Duration timeout;

  @override
  State<ReceivePanel> createState() => ReceivePanelState();
}

class ReceivePanelState extends State<ReceivePanel> {
  LinkReceiver? _r;
  List<String>? _rows;
  bool _starting = true;
  bool _noNetwork = false;
  String? _failed;
  DateTime _until = DateTime.now();
  Timer? _tick;
  late final AppLifecycleListener _life;

  /// The receiver now, for tests.
  @visibleForTesting
  LinkReceiver? get receiver => _r;

  @override
  void initState() {
    super.initState();
    // Leaving the app ends it too; the sheet offers to start again.
    _life = AppLifecycleListener(onPause: () => _r?.stop());
    _start();
  }

  Future<void> _start() async {
    _drop();
    _starting = true;
    _noNetwork = false;
    _failed = null;
    _rows = null;
    if (mounted && _built) setState(() {});
    final addr = await widget.address();
    if (!mounted) return;
    if (addr == null) {
      setState(() {
        _starting = false;
        _noNetwork = true;
      });
      return;
    }
    final r = LinkReceiver(onLink: widget.app.receiveLink, timeout: widget.timeout);
    try {
      final url = await r.start(addr);
      if (!mounted) {
        r.dispose();
        return;
      }
      _r = r..addListener(_changed);
      _until = DateTime.now().add(widget.timeout);
      _tick = Timer.periodic(const Duration(seconds: 1), (_) {
        if (mounted) setState(() {});
      });
      setState(() => _starting = false);
      final rows = await widget.qr('$url');
      if (mounted && identical(_r, r)) setState(() => _rows = rows);
    } catch (e) {
      r.dispose();
      if (mounted) {
        setState(() {
          _starting = false;
          _failed = '$e';
        });
      }
    }
  }

  void _changed() {
    final r = _r;
    if (r == null || !mounted) return;
    if (!r.listening) {
      _tick?.cancel();
      _tick = null;
    }
    if (r.end == ReceiveEnd.added) {
      widget.app.closeSheet();
      return;
    }
    setState(() {});
  }

  void _drop() {
    _tick?.cancel();
    _tick = null;
    _r?.removeListener(_changed);
    _r?.dispose();
    _r = null;
  }

  @override
  void dispose() {
    _life.dispose();
    _drop();
    super.dispose();
  }

  bool _built = false;

  @override
  Widget build(BuildContext context) {
    _built = true;
    final l = Skin.of(context);
    final r = _r;
    final url = r?.url;
    final live = r != null && r.listening;

    final String status;
    var bad = false;
    if (_starting) {
      status = tr('正在准备', 'Getting ready');
    } else if (_noNetwork) {
      status = tr('这台设备没有连上 Wi-Fi 或有线网络', "This device isn't on Wi-Fi or a wired network");
      bad = true;
    } else if (_failed != null) {
      status = _failed!;
      bad = true;
    } else if (r != null && r.adding) {
      status = tr('正在添加', 'Adding');
    } else if (live) {
      final left = _until.difference(DateTime.now());
      final s = left.isNegative ? 0 : left.inSeconds;
      final wait = tr('等待发送，{0} 后停止', 'Waiting. Stops in {0}', ['${s ~/ 60}:${(s % 60).toString().padLeft(2, '0')}']);
      status = r.lastError != null ? '${tr('没有添加：{0}', 'Not added: {0}', [r.lastError])}\n$wait' : wait;
      bad = r.lastError != null;
    } else {
      status = switch (r?.end) {
        ReceiveEnd.timeout => tr('已超时，这个地址不再可用', 'Timed out. This address no longer works.'),
        ReceiveEnd.locked => tr('错误的访问太多，已停止', 'Too many wrong requests, so it stopped'),
        _ => tr('已停止', 'Stopped'),
      };
      bad = true;
    }
    final again = !_starting && !live && !(r?.adding ?? false);

    final code = Container(
      width: 200,
      height: 200,
      padding: const EdgeInsets.all(8),
      decoration: BoxDecoration(color: const Color(0xFFFFFFFF), borderRadius: l.br(12)),
      child: live && _rows != null ? CustomPaint(painter: QRPainter(_rows!)) : null,
    );
    final words = Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Text(tr('用手机扫描二维码，或在浏览器里打开这个地址，粘贴链接后发送。手机要和这台设备在同一个网络。', 'Scan the code with a phone, or open this address in its browser, then paste the link and send it. The phone must be on the same network as this device.'),
          style: l.text(13, color: l.mute, lh: 18)),
      const SizedBox(height: 10),
      if (live && url != null) Text('$url', key: const ValueKey('receive-url'), style: l.text(16, weight: 600, lh: 22)),
      const SizedBox(height: 6),
      Text(status, key: const ValueKey('receive-status'), style: l.text(13, color: bad ? l.signalT : l.ink2, lh: 18)),
    ]);
    if (widget.inline) {
      final restart = Btn(tr('重新开始', 'Start again'), kind: BtnKind.accent, minH: 44, padH: 20, onTap: _start);
      return Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(color: l.paper2, borderRadius: l.br(14)),
        child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          Text(tr('从手机添加第一个 Provider', 'Add your first Provider from a phone'), style: l.text(17, weight: 700, lh: 23)),
          const SizedBox(height: 12),
          LayoutBuilder(
            builder: (context, c) => c.maxWidth >= 440
                ? Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
                    code,
                    const SizedBox(width: 18),
                    Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [words, if (again) ...[const SizedBox(height: 12), restart]])),
                  ])
                : Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [Center(child: code), const SizedBox(height: 12), words, if (again) ...[const SizedBox(height: 12), restart]]),
          ),
        ]),
      );
    }
    final acts = Row(children: [
      Expanded(child: Btn(tr('关闭', 'Close'), expand: true, minH: 44, onTap: widget.app.closeSheet)),
      if (again) ...[
        const SizedBox(width: 8),
        Expanded(child: Btn(tr('重新开始', 'Start again'), kind: BtnKind.accent, expand: true, minH: 44, onTap: _start)),
      ],
    ]);

    return Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Text(tr('从手机发送链接', 'Send a link from a phone'), style: l.text(15, weight: 700, lh: 21)),
      const SizedBox(height: 10),
      Flexible(
        child: SingleChildScrollView(
          child: LayoutBuilder(
            // Side by side where the sheet is wide (a TV, a tablet), so it
            // fits a short screen.
            builder: (context, c) => c.maxWidth >= 440
                ? Row(crossAxisAlignment: CrossAxisAlignment.start, children: [code, const SizedBox(width: 16), Expanded(child: words)])
                : Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [Center(child: code), const SizedBox(height: 12), words]),
          ),
        ),
      ),
      const SizedBox(height: 12),
      acts,
    ]);
  }
}

/// A QR code from rows of 0 and 1, black on the white it is laid on.
class QRPainter extends CustomPainter {
  const QRPainter(this.rows);
  final List<String> rows;

  @override
  void paint(Canvas canvas, Size size) {
    final cell = size.width / rows.length;
    final p = Paint()..color = const Color(0xFF000000);
    for (final (y, row) in rows.indexed) {
      for (var x = 0; x < row.length; x++) {
        if (row.codeUnitAt(x) == 0x31) canvas.drawRect(Rect.fromLTWH(x * cell, y * cell, cell + .5, cell + .5), p);
      }
    }
  }

  @override
  bool shouldRepaint(QRPainter old) => !identical(old.rows, rows);
}
