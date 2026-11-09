import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart' show Clipboard, ClipboardData;
import 'package:flutter/widgets.dart';

import '../app/i18n.dart';
import '../app/sample.dart';
import '../app/state.dart';
import '../ui/atoms.dart';
import '../ui/skin.dart';
import 'frame.dart';
import 'sheet.dart';

const _open = Duration(milliseconds: 300);

class ProvidersPage extends StatefulWidget {
  const ProvidersPage({super.key, required this.app});

  final AppState app;

  @override
  State<ProvidersPage> createState() => _ProvidersPageState();
}

class _ProvidersPageState extends State<ProvidersPage> {
  final _scroll = ScrollController();
  final _metas = <int, GlobalKey>{};

  @override
  void dispose() {
    _scroll.dispose();
    super.dispose();
  }

  void _toggle(int i) {
    final app = widget.app;
    final opening = app.openProvider != i;
    app.set(() {
      app.openProvider = opening ? i : null;
      app.providerQuery = '';
    });
    if (!opening) return;
    // Closing the provider above this one pulls this one upward; bring its
    // first lines back so opening never lands in the middle of a list.
    Future.delayed(_open + const Duration(milliseconds: 140), () {
      if (!mounted || widget.app.openProvider != i || !_scroll.hasClients) return;
      final box = _metas[i]?.currentContext?.findRenderObject();
      if (box == null) return;
      final viewport = RenderAbstractViewport.maybeOf(box);
      if (viewport == null) return;
      final wanted = viewport.getOffsetToReveal(box, 0).offset - 45;
      if (_scroll.offset > wanted) {
        _scroll.animateTo(wanted < 0 ? 0 : wanted, duration: const Duration(milliseconds: 360), curve: easeCard);
      }
    });
  }

  Widget _dock(Look l, AppState app) {
    final i = app.openProvider;
    final p = i == null ? null : providers[i];
    if (p != null && p.long) {
      return Row(children: [
        Expanded(
          child: SearchField(
            key: ValueKey('search-$i'),
            hint: tr('在 {0} 个节点里搜索', 'Search {0} nodes', [providerNodes(p).length]),
            label: tr('搜索节点', 'Search nodes'),
            onChanged: (v) => app.set(() => app.providerQuery = v),
          ),
        ),
        const SizedBox(width: 8),
        Btn(tr('测速', 'Test'), minH: 44, padH: 13, fontSize: 13.5, onTap: app.providerTesting ? null : app.testProvider, child: app.providerTesting ? Shim(active: true, child: Text(tr('测速中', 'Testing'), maxLines: 1, style: l.text(13.5, weight: 600, lh: 19))) : null),
      ]);
    }
    Widget square(String icon, String label, VoidCallback onTap) => Tap(
          onTap: onTap,
          label: label,
          child: Container(width: 46, height: 44, alignment: Alignment.center, decoration: BoxDecoration(color: l.paper2, borderRadius: l.br(11)), child: Ic(icon, color: l.ink2)),
        );
    return Row(children: [
      Expanded(child: SearchField(key: const ValueKey('add'), controller: app.providerField, hint: tr('粘贴订阅链接或节点链接', 'Paste a subscription or node link'), label: tr('添加 Provider', 'Add Provider'), icon: false, onSubmitted: app.addProvider)),
      const SizedBox(width: 8),
      square('scan', tr('扫描二维码', 'Scan QR code'), app.scanLink),
      const SizedBox(width: 8),
      square('more', tr('更多添加方式', 'More ways to add'), () => pickSheet<String>(app, tr('添加或更新', 'Add or update'), [
            PickOption('file', tr('选择 YAML 文件', 'Choose a YAML file')),
            PickOption('image', tr('从相册识别二维码', 'Read a QR code from Photos')),
            PickOption('paste', tr('粘贴剪贴板里的链接', 'Paste link from clipboard')),
            PickOption('all', tr('更新全部订阅', 'Update all subscriptions')),
          ], (v) {
            switch (v) {
              case 'file':
                app.addFile();
              case 'image':
                app.addFromImage();
              case 'paste':
                app.addFromClipboard();
              case 'all':
                app.updateAllProviders();
            }
          })),
    ]);
  }

  @override
  Widget build(BuildContext context) {
    final app = widget.app;
    final l = Skin.of(context);
    return ListPage(
      title: 'Provider',
      dock: _dock(l, app),
      dockH: 66,
      list: (bottom) => CustomScrollView(
        controller: _scroll,
        keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.onDrag,
        slivers: [
          for (final (i, p) in providers.indexed)
            _ProviderTile(
              key: ValueKey(p.id.isEmpty ? p.name : p.id),
              app: app,
              index: i,
              p: p,
              metaKey: _metas.putIfAbsent(i, () => GlobalKey()),
              onToggle: () => _toggle(i),
            ),
          SliverPadding(padding: EdgeInsets.only(bottom: bottom)),
        ],
      ),
    );
  }
}

String _autoWord(int h) => switch (h) { 0 => tr('不自动更新', 'Never'), 1 => tr('每小时', 'Hourly'), 24 => tr('每天', 'Daily'), 168 => tr('每周', 'Weekly'), _ => tr('每 {0} 小时', 'Every {0} hours', [h]) };

void _share(AppState app, ProviderInfo p) async {
  final out = await app.ask('shareProvider', {'id': p.id});
  if (out is! Map) return;
  if (out['error'] != null) {
    app.say(out['error'] as String, ms: 4000);
    return;
  }
  final text = out['text'] as String;
  final rows = (out['rows'] as List? ?? const []).cast<String>();
  app.showSheet((context) {
    final l = Skin.of(context);
    return Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Text(tr('分享 {0}', 'Share {0}', [p.name]), style: l.text(15, weight: 700, lh: 21)),
      const SizedBox(height: 4),
      Text(tr('这个链接等同于这个 Provider 的账号，只发给信任的人。', 'This link works like the account for this Provider. Only send it to people you trust.'), style: l.text(12.5, color: l.mute, lh: 17)),
      const SizedBox(height: 12),
      if (rows.isNotEmpty)
        Center(
          child: Container(
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(color: const Color(0xFFFFFFFF), borderRadius: l.br(12)),
            child: SizedBox(width: 216, height: 216, child: CustomPaint(painter: _QR(rows))),
          ),
        )
      else
        Text(tr('内容太长，放不进二维码。', 'Too long to fit in a QR code.'), style: l.text(13.5, color: l.ink2, lh: 19)),
      const SizedBox(height: 12),
      Btn(tr('复制链接', 'Copy link'), kind: BtnKind.accent, expand: true, minH: 44, onTap: () {
        Clipboard.setData(ClipboardData(text: text));
        app.closeSheet();
        app.say(tr('已复制', 'Copied'));
      }),
    ]);
  });
}

class _QR extends CustomPainter {
  const _QR(this.rows);
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
  bool shouldRepaint(_QR old) => !identical(old.rows, rows);
}

void _more(AppState app, ProviderInfo p) {
  pickSheet<String>(app, p.name, [
    PickOption('rename', tr('重命名', 'Rename')),
    if (p.shareable) PickOption('share', tr('分享', 'Share')),
    if (p.subscription) PickOption('auto', tr('自动更新', 'Auto update'), sub: _autoWord(p.autoUpdate)),
    PickOption('enable', p.enabled ? tr('停用', 'Disable') : tr('启用', 'Enable'), sub: p.enabled ? tr('它的节点不再参与路由', 'Its nodes leave every route') : null),
    PickOption('delete', tr('删除', 'Delete'), danger: true),
  ], (v) {
    switch (v) {
      case 'rename':
        inputSheet(app, tr('重命名', 'Rename'), (t) {
          if (t.isNotEmpty) app.renameProvider(p, t);
        }, initial: p.name);
      case 'share':
        _share(app, p);
      case 'auto':
        pickSheet<int>(app, tr('自动更新', 'Auto update'), [for (final h in const [0, 1, 6, 12, 24, 168]) PickOption(h, _autoWord(h))], (h) => app.setProviderAuto(p, h), value: p.autoUpdate);
      case 'enable':
        app.setProviderEnabled(p, !p.enabled);
      case 'delete':
        confirmSheet(app, tr('删除 {0}', 'Delete {0}', [p.name]), tr('删除', 'Delete'), () => app.deleteProvider(p), body: tr('它的 {0} 个节点会一起移除。', 'Its {0} nodes are removed with it.', [p.nodes]));
    }
  });
}

Widget _tinyActs(Look l, AppState app, ProviderInfo p, {String? note}) {
  Widget tiny(String t, BtnKind k, VoidCallback onTap) => Padding(padding: const EdgeInsets.only(left: 6), child: Btn(t, kind: k, minH: 36, padH: 11, fontSize: 13, onTap: onTap));
  return Padding(
    padding: const EdgeInsets.fromLTRB(16, 2, 12, 10),
    child: Row(children: [
      if (note != null) Expanded(child: Line1(note, l.text(12.5, color: l.mute, lh: 18))) else const Spacer(),
      if (p.subscription) tiny(tr('更新', 'Update'), BtnKind.quiet, () => app.updateProvider(p)),
      tiny(tr('更多', 'More'), BtnKind.quiet, () => _more(app, p)),
    ]),
  );
}

class _NodeRow extends StatelessWidget {
  const _NodeRow({required this.n, required this.testing});
  final Node n;
  final bool testing;

  @override
  Widget build(BuildContext context) {
    final l = Skin.of(context);
    return hairTop(
      l,
      Container(
        constraints: const BoxConstraints(minHeight: 44),
        padding: const EdgeInsets.fromLTRB(45, 0, 16, 0),
        child: Row(children: [
          Expanded(child: Line1(n.name, l.text(14, lh: 20))),
          const SizedBox(width: 8),
          Text(n.proto, maxLines: 1, style: l.text(12, color: l.mute, lh: 17)),
          const SizedBox(width: 8),
          SizedBox(
            width: 56,
            child: Align(
              alignment: Alignment.centerRight,
              child: testing ? const Skeleton() : Text(latText(n.delay), maxLines: 1, style: l.text(12.5, color: latColor(l, n.delay), lh: 18)),
            ),
          ),
        ]),
      ),
    );
  }
}

class _LatDots extends StatelessWidget {
  const _LatDots(this.tones);
  final List<int> tones;

  @override
  Widget build(BuildContext context) {
    final l = Skin.of(context);
    final List<int> shown = tones.length > 48 ? List.generate(48, (i) => tones[i * tones.length ~/ 48]) : tones;
    return Row(mainAxisSize: MainAxisSize.min, children: [
      for (final v in shown)
        Padding(
          padding: const EdgeInsets.only(right: 2),
          child: Opacity(
            opacity: v == 3 ? .55 : (v < 0 ? .35 : 1),
            child: Container(width: 5, height: 5, decoration: BoxDecoration(shape: BoxShape.circle, color: v < 0 ? l.mute : (v == 1 ? l.good : (v == 0 ? l.signal : l.warn)))),
          ),
        ),
    ]);
  }
}

/// One provider. It is a set of slivers rather than a box so that an open
/// provider's name stays at the top while hundreds of its nodes go by.
class _ProviderTile extends StatefulWidget {
  const _ProviderTile({super.key, required this.app, required this.index, required this.p, required this.metaKey, required this.onToggle});

  final AppState app;
  final int index;
  final ProviderInfo p;
  final GlobalKey metaKey;
  final VoidCallback onToggle;

  @override
  State<_ProviderTile> createState() => _ProviderTileState();
}

class _ProviderTileState extends State<_ProviderTile> with SingleTickerProviderStateMixin {
  late bool _isOpen = widget.app.openProvider == widget.index;
  late final _c = AnimationController(vsync: this, duration: _open, value: _isOpen ? 1 : 0)
    ..addStatusListener((_) {
      if (mounted) setState(() {});
    });
  late final _curve = CurvedAnimation(parent: _c, curve: easeCard);
  late final _turns = Tween<double>(begin: 0, end: .25).animate(_curve);

  @override
  void didUpdateWidget(_ProviderTile old) {
    super.didUpdateWidget(old);
    final now = widget.app.openProvider == widget.index;
    if (now != _isOpen) {
      _isOpen = now;
      now ? _c.forward() : _c.reverse();
    }
  }

  @override
  void dispose() {
    _curve.dispose();
    _c.dispose();
    super.dispose();
  }

  Map<String, List<Node>> _groups(List<Node> ns) {
    final out = <String, List<Node>>{};
    for (final n in ns) {
      out.putIfAbsent(n.flag, () => []).add(n);
    }
    return out;
  }

  Widget _head(Look l, int count) {
    final p = widget.p;
    return Tap(
      onTap: widget.onToggle,
      child: Container(
        constraints: const BoxConstraints(minHeight: 44),
        padding: EdgeInsets.fromLTRB(16, 10, 16, _isOpen ? 10 : 0),
        decoration: BoxDecoration(color: l.paper, border: _isOpen ? Border(bottom: BorderSide(color: l.hair2)) : null),
        child: Row(children: [
          Expanded(child: Line1(p.name, l.text(15, weight: 600, lh: 21))),
          if (p.warn != null) ...[Dot(l.warn), const SizedBox(width: 8)],
          Text(tr('{0} 个节点', count == 1 ? '{0} node' : '{0} nodes', [count]), style: l.text(12.5, color: l.mute, lh: 18)),
          const SizedBox(width: 8),
          RotationTransition(turns: _turns, child: Ic('chev', size: 16, color: l.mute)),
        ]),
      ),
    );
  }

  Widget _meta(Look l, List<Node> ns) {
    final p = widget.p;
    final s = l.text(12.5, color: l.mute, lh: 18);
    final tones = p.long
        ? [
            for (final n in ns.toList()..sort(byLatency)) latTone(n.delay),
          ]
        : p.tones;
    return Tap(
      key: widget.metaKey,
      onTap: widget.onToggle,
      child: Container(
        width: double.infinity,
        padding: const EdgeInsets.fromLTRB(16, 0, 16, 13),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          if (p.used != null)
            Container(
              height: 5,
              margin: const EdgeInsets.only(top: 6, bottom: 3),
              clipBehavior: Clip.hardEdge,
              alignment: Alignment.centerLeft,
              decoration: BoxDecoration(color: l.paper2, borderRadius: BorderRadius.circular(3)),
              child: FractionallySizedBox(widthFactor: p.used! / 100, heightFactor: 1, child: ColoredBox(color: p.used! > 90 ? l.warn : l.ink2)),
            ),
          Text('${p.enabled ? p.kind : tr('已停用', 'Disabled')}${p.usage != null ? ' · ${p.usage}' : ''}${p.source.isNotEmpty ? ' · ${p.source}' : ''}', maxLines: 1, overflow: TextOverflow.ellipsis, style: s),
          if (p.usedBy.isNotEmpty) Text(tr('正在用：{0}', 'Used by {0}', [p.usedBy.join(tr('、', ', '))]), maxLines: 1, overflow: TextOverflow.ellipsis, style: s),
          if (p.warn != null)
            Text(p.warn!, style: s.copyWith(color: l.warnT))
          else
            Text('${p.expires != null ? '${p.expires} · ' : ''}${p.updated}', style: s),
          const SizedBox(height: 6),
          _LatDots(tones),
        ]),
      ),
    );
  }

  List<(String, List<Node>)> _regionsOf(List<Node> ns, String q) {
    final g = _groups(ns);
    final keys = g.keys.toList()..sort((a, b) => a.isEmpty ? 1 : (b.isEmpty ? -1 : g[b]!.length.compareTo(g[a]!.length)));
    return [
      for (final k in keys)
        if (q.isEmpty || g[k]!.any((n) => n.name.toLowerCase().contains(q))) (k, q.isEmpty ? g[k]! : g[k]!.where((n) => n.name.toLowerCase().contains(q)).toList()),
    ];
  }

  @override
  Widget build(BuildContext context) {
    final app = widget.app, p = widget.p;
    final l = Skin.of(context);
    final ns = providerNodes(p);
    final count = p.long ? ns.length : p.nodes;
    final head = _head(l, count);
    final q = app.providerQuery.trim().toLowerCase();

    final List<Widget> body;
    if (_c.isDismissed) {
      body = const [];
    } else if (p.long) {
      final up = ns.where((n) => n.delay > 0).length;
      final untested = ns.where((n) => n.delay < 0).length;
      final failed = ns.length - up - untested;
      final acts = _tinyActs(l, app, p, note: untested == ns.length ? tr('{0} 个未测速', '{0} untested', [untested]) : (untested > 0 ? tr('{0} 个可用，{1} 个超时，{2} 个未测速', '{0} up, {1} timed out, {2} untested', [up, failed, untested]) : tr('{0} 个可用，{1} 个超时', '{0} up, {1} timed out', [up, failed])));
      final regionsNow = _regionsOf(ns, _isOpen ? q : '');
      if (_c.isCompleted) {
        body = [
          SliverToBoxAdapter(child: acts),
          for (final (f, list) in regionsNow)
            _RegionTile(key: ValueKey(f), app: app, groupKey: '${widget.index}|$f', flag: f, nodes: list, forced: q.isNotEmpty),
        ];
      } else {
        // while the provider itself is unfolding, its contents are one box
        body = [
          SliverToBoxAdapter(
            child: SizeTransition(
              sizeFactor: _curve,
              alignment: Alignment.topCenter,
              child: Column(children: [
                acts,
                for (final (f, list) in regionsNow) _RegionHead(flag: f, nodes: list, open: app.openRegions.contains('${widget.index}|$f')),
              ]),
            ),
          ),
        ];
      }
    } else {
      final list = ns.toList()..sort(byLatency);
      body = [
        SliverToBoxAdapter(
          child: SizeTransition(
            sizeFactor: _curve,
            alignment: Alignment.topCenter,
            child: Column(children: [
              _tinyActs(l, app, p),
              for (final n in list) _NodeRow(n: n, testing: false),
              if (count > ns.length)
                hairTop(
                  l,
                  Container(
                    constraints: const BoxConstraints(minHeight: 44),
                    padding: const EdgeInsets.only(left: 45),
                    alignment: Alignment.centerLeft,
                    child: Text(tr('还有 {0} 个节点', '{0} more nodes', [count - ns.length]), style: l.text(14, color: l.mute, lh: 20)),
                  ),
                ),
            ]),
          ),
        ),
      ];
    }

    return SliverMainAxisGroup(slivers: [
      _isOpen ? PinnedHeaderSliver(child: head) : SliverToBoxAdapter(child: head),
      SliverToBoxAdapter(child: _meta(l, ns)),
      ...body,
      SliverToBoxAdapter(child: Container(height: 1, color: l.hair2)),
    ]);
  }
}

class _RegionHead extends StatelessWidget {
  const _RegionHead({required this.flag, required this.nodes, required this.open, this.turns, this.onTap});

  final String flag;
  final List<Node> nodes;
  final bool open;
  final Animation<double>? turns;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final l = Skin.of(context);
    final best = (nodes.toList()..sort(byLatency)).first;
    final chev = Ic('chev', size: 15, color: l.mute);
    return Tap(
      onTap: onTap ?? () {},
      child: Container(
        constraints: const BoxConstraints(minHeight: 48),
        padding: const EdgeInsets.fromLTRB(16, 0, 12, 0),
        decoration: BoxDecoration(color: l.paper, border: Border(top: BorderSide(color: l.hair2))),
        child: Row(children: [
          FlagDot(flag, 20),
          const SizedBox(width: 9),
          Text(regionName(flag), style: l.text(14.5, weight: 600, lh: 20)),
          const SizedBox(width: 9),
          Text('${nodes.length}', style: l.text(12.5, color: l.mute, lh: 18)),
          const Spacer(),
          Text(best.delay > 0 ? tr('最快 {0} ms', 'Fastest {0} ms', [best.delay]) : latText(best.delay), style: l.text(12.5, color: latColor(l, best.delay), lh: 18)),
          const SizedBox(width: 9),
          turns != null ? RotationTransition(turns: turns!, child: chev) : Transform.rotate(angle: open ? 1.5708 : 0, child: chev),
        ]),
      ),
    );
  }
}

class _RegionTile extends StatefulWidget {
  const _RegionTile({super.key, required this.app, required this.groupKey, required this.flag, required this.nodes, required this.forced});

  final AppState app;
  final String groupKey, flag;
  final List<Node> nodes;

  /// A search shows every region that has a match, open.
  final bool forced;

  @override
  State<_RegionTile> createState() => _RegionTileState();
}

class _RegionTileState extends State<_RegionTile> with SingleTickerProviderStateMixin {
  bool get _isOpen => widget.forced || widget.app.openRegions.contains(widget.groupKey);
  late bool _was = _isOpen;
  late final _c = AnimationController(vsync: this, duration: _open, value: _was ? 1 : 0);
  late final _curve = CurvedAnimation(parent: _c, curve: easeCard);
  late final _turns = Tween<double>(begin: 0, end: .25).animate(_curve);

  @override
  void didUpdateWidget(_RegionTile old) {
    super.didUpdateWidget(old);
    if (_isOpen != _was) {
      _was = _isOpen;
      _was ? _c.forward() : _c.reverse();
    }
  }

  @override
  void dispose() {
    _curve.dispose();
    _c.dispose();
    super.dispose();
  }

  void _toggle() {
    final app = widget.app, k = widget.groupKey;
    app.set(() => app.openRegions.contains(k) ? app.openRegions.remove(k) : app.openRegions.add(k));
  }

  @override
  Widget build(BuildContext context) {
    final head = _RegionHead(flag: widget.flag, nodes: widget.nodes, open: _was, turns: _turns, onTap: _toggle);
    final list = widget.nodes.toList()..sort(byLatency);
    return SliverMainAxisGroup(slivers: [
      _was ? PinnedHeaderSliver(child: head) : SliverToBoxAdapter(child: head),
      SliverToBoxAdapter(
        child: Accordion(
          open: _was,
          builder: (context) => Column(children: [for (final n in list) _NodeRow(n: n, testing: widget.app.providerTesting)]),
        ),
      ),
    ]);
  }
}
