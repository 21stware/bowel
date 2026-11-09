import 'package:flutter/rendering.dart' show ScrollCacheExtent;
import 'package:flutter/widgets.dart';

import '../app/i18n.dart';
import '../ui/atoms.dart';
import '../ui/skin.dart';
import 'frame.dart';

Widget rowLabel(Look l, String a, String? sub) => Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Text(a, style: l.text(15, lh: 21)),
      if (sub != null && sub.isNotEmpty) Text(sub, style: l.text(12.5, color: l.mute, lh: 17)),
    ]);

/// A row that opens or picks something. No `onTap` is a row that only says.
Widget navRow(Look l, String a, String b, {String? sub, VoidCallback? onTap}) {
  final row = hairBottom(
    l,
    Container(
      constraints: const BoxConstraints(minHeight: 48),
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Row(children: [
        Expanded(child: rowLabel(l, a, sub)),
        const SizedBox(width: 12),
        ConstrainedBox(constraints: const BoxConstraints(maxWidth: 150), child: Text(b, maxLines: 1, overflow: TextOverflow.ellipsis, style: l.text(14, color: l.mute, lh: 20))),
        if (onTap != null) ...[const SizedBox(width: 4), Ic('chev', size: 15, color: l.mute)],
      ]),
    ),
  );
  return onTap == null ? row : Tap(onTap: onTap, child: row);
}

Widget toggleRow(Look l, String a, bool on, VoidCallback onTap, {String? sub}) => hairBottom(
      l,
      Container(
        constraints: const BoxConstraints(minHeight: 48),
        padding: const EdgeInsets.symmetric(vertical: 6),
        child: Row(children: [
          Expanded(child: rowLabel(l, a, sub)),
          const SizedBox(width: 12),
          Toggle(on: on, onTap: onTap, scale: .82, label: a),
        ]),
      ),
    );

Widget section(Look l, String title, List<Widget> kids, {double top = 18}) => Padding(
      padding: EdgeInsets.fromLTRB(16, top, 16, 4),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [eyebrow(l, title), ...kids]),
    );

/// A page pushed inside 设置: title on top, back and actions by the thumb.
class SubPage extends StatelessWidget {
  const SubPage({super.key, required this.title, required this.children, this.actions = const [], this.dockChild, this.eager = false});

  final String title;
  final List<Widget> children;
  final List<Widget> actions;

  /// Takes the middle of the dock, where the actions would go.
  final Widget? dockChild;

  /// Lays out every row, so that one further down can be scrolled to.
  final bool eager;

  @override
  Widget build(BuildContext context) {
    final l = Skin.of(context);
    return ColoredBox(
      color: l.paper,
      child: ListPage(
        title: title,
        marginTop: 0,
        dockH: 50,
        dock: Row(children: [
          IconBox('chev', flip: true, label: tr('返回', 'Back'), onTap: () => Navigator.of(context).maybePop()),
          const SizedBox(width: 8),
          Expanded(child: dockChild ?? Row(mainAxisAlignment: MainAxisAlignment.end, children: actions)),
        ]),
        list: (bottom) => ListView(scrollCacheExtent: eager ? const ScrollCacheExtent.pixels(100000) : null, padding: EdgeInsets.only(bottom: bottom), keyboardDismissBehavior: ScrollViewKeyboardDismissBehavior.onDrag, children: children),
      ),
    );
  }
}
