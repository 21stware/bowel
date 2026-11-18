import 'package:flutter/scheduler.dart';
import 'package:flutter/widgets.dart';

import 'state.dart';

/// Marks what its child shows as on screen: a node's delay on a route, or
/// with no node the route's other sites. The core keeps that tested for as
/// long as this is mounted on the tab in front, and in view where its page
/// scrolls.
class Shown extends StatefulWidget {
  const Shown({super.key, required this.app, this.node = '', required this.route, required this.child});

  final AppState app;
  final String node, route;
  final Widget child;

  @override
  State<Shown> createState() => _ShownState();
}

class _ShownState extends State<Shown> {
  (String, String)? _told;
  bool _front = false; // the other tabs keep their pages, with their tickers off
  ScrollableState? _scroll; // what scrolls this, if anything does
  ScrollPosition? _pos;
  bool _waiting = false;

  void _tell((String, String)? row) {
    if (row == _told) return;
    if (_told != null) widget.app.unshow(_told!);
    if (row != null) widget.app.show(row);
    _told = row;
  }

  /// Whether what scrolls this shows any of it: a page lays out rows it has
  /// no room for, and a list keeps some ready past its edges.
  bool _inView() {
    final box = context.findRenderObject();
    final view = _scroll?.context.findRenderObject();
    if (box is! RenderBox || view is! RenderBox || !box.hasSize || !view.hasSize) return true;
    return (box.localToGlobal(Offset.zero, ancestor: view) & box.size).overlaps(Offset.zero & view.size);
  }

  void _sync() => _tell(_front && _inView() ? (widget.node, widget.route) : null);

  /// Looks again once the frame has placed everything.
  void _later() {
    if (_scroll == null) return _sync();
    if (_waiting) return;
    _waiting = true;
    SchedulerBinding.instance.addPostFrameCallback((_) {
      _waiting = false;
      if (mounted) _sync();
    });
    SchedulerBinding.instance.ensureVisualUpdate();
  }

  /// Rows passing by under a finger are looked at where they come to rest.
  void _moved() {
    if (!(_pos?.isScrollingNotifier.value ?? false)) _later();
  }

  void _listen(ScrollPosition? pos) {
    if (pos == _pos) return;
    _pos?.removeListener(_moved);
    _pos?.isScrollingNotifier.removeListener(_moved);
    _pos = pos;
    pos?.addListener(_moved);
    pos?.isScrollingNotifier.addListener(_moved);
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _front = TickerMode.valuesOf(context).enabled;
    _scroll = Scrollable.maybeOf(context);
    _listen(_scroll?.position);
    _later();
  }

  @override
  void didUpdateWidget(Shown old) {
    super.didUpdateWidget(old);
    _later();
  }

  @override
  void dispose() {
    _listen(null);
    _tell(null);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => widget.child;
}
