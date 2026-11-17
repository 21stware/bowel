import 'package:flutter/widgets.dart';

import 'state.dart';

/// Marks what its child shows as on screen: a node's delay on a route, or
/// with no node the route's other sites. The core keeps that tested for as
/// long as this is mounted on the tab in front.
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

  void _tell((String, String)? row) {
    if (row == _told) return;
    if (_told != null) widget.app.unshow(_told!);
    if (row != null) widget.app.show(row);
    _told = row;
  }

  // The other tabs keep their pages, with their tickers off.
  void _sync() => _tell(TickerMode.valuesOf(context).enabled ? (widget.node, widget.route) : null);

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _sync();
  }

  @override
  void didUpdateWidget(Shown old) {
    super.didUpdateWidget(old);
    _sync();
  }

  @override
  void dispose() {
    _tell(null);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => widget.child;
}
