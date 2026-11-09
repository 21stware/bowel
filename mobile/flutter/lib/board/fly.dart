import 'package:flutter/rendering.dart';
import 'package:flutter/widgets.dart';

import '../ui/atoms.dart';

/// Which cell a piece of content is in, and whether that view is the one showing.
class CellScope extends InheritedWidget {
  const CellScope({super.key, required this.cell, required this.shown, required super.child});

  final String cell;
  final bool shown;

  static CellScope? of(BuildContext context) => context.dependOnInheritedWidgetOfExactType<CellScope>();

  @override
  bool updateShouldNotify(CellScope oldWidget) => cell != oldWidget.cell || shown != oldWidget.shown;
}

/// The clock of a view's entrance, in fractions of one second.
class EntranceScope extends InheritedNotifier<Animation<double>> {
  const EntranceScope({super.key, required Animation<double> super.notifier, required super.child});

  static Animation<double>? of(BuildContext context) => context.getInheritedWidgetOfExactType<EntranceScope>()?.notifier;
}

class _TopReveal extends CustomClipper<Rect> {
  const _TopReveal(this.t);
  final double t;

  @override
  Rect getClip(Size size) => Rect.fromLTRB(-40, 0, size.width + 40, size.height * t);

  @override
  bool shouldReclip(_TopReveal old) => old.t != t;
}

const _revealCurve = Cubic(.2, .8, .2, 1);

/// Unrolls from the top when its view enters: `step` staggers it, 28 ms each.
class Unroll extends StatelessWidget {
  const Unroll({super.key, this.step = 0, required this.child});

  final int step;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final clock = EntranceScope.of(context);
    if (clock == null) return child;
    return AnimatedBuilder(
      animation: clock,
      child: child,
      builder: (context, child) {
        final t = ((clock.value * 1000 - 100 - step * 28) / 440).clamp(0.0, 1.0);
        final e = _revealCurve.transform(t);
        return ClipRect(
          clipBehavior: e >= 1 ? Clip.none : Clip.hardEdge,
          clipper: _TopReveal(e),
          child: _Rising(dy: -6 * (1 - e), child: child),
        );
      },
    );
  }
}

/// An entrance's lift, marked so a flight can aim past it at the resting place.
class _Rising extends Transform {
  _Rising({required this.dy, super.child}) : super.translate(offset: Offset(0, dy));

  final double dy;
}

class _Snap {
  _Snap({required this.rect, required this.tag});
  final Rect rect;
  final FlyTag tag;

  TextStyle? get style => tag.style;
  String get text => tag.text;
}

class _Flight {
  _Flight({required this.id, required this.from, required this.to, required this.controller});

  final String id;
  final _Snap from, to;
  final AnimationController controller;

  bool get same => from.text.replaceAll(' ', '') == to.text.replaceAll(' ', '');
  bool get moved {
    final o = from.rect, a = to.rect;
    return (o.left - a.left).abs() > 1 ||
        (o.top - a.top).abs() > 1 ||
        ((from.style?.fontSize ?? 0) - (to.style?.fontSize ?? 0)).abs() > .5 ||
        (to.tag.isBox && (o.width - a.width).abs() > 1);
  }
}

/// Lays the board's flights over it. Shared pieces fly as real text, laid out
/// again on every frame at the interpolated size, and are only ever drawn once.
class FlyHost extends StatefulWidget {
  const FlyHost({super.key, required this.child});

  final Widget child;

  static FlyHostState of(BuildContext context) => context.findAncestorStateOfType<FlyHostState>()!;

  @override
  State<FlyHost> createState() => FlyHostState();
}

class FlyHostState extends State<FlyHost> with TickerProviderStateMixin {
  final _tags = <_FlyTagState>{};
  final _flights = <_Flight>[];
  final hidden = ValueNotifier<Set<String>>(const {});

  void _hide(Set<String> ids) => hidden.value = ids;

  /// Measures every shared piece that's showing, as it's drawn right now.
  Map<String, _Snap> _snapNow() {
    final host = context.findRenderObject() as RenderBox;
    final m = <String, _Snap>{};
    for (final t in _tags) {
      final s = t.measure(host, null);
      if (s != null) m[t.widget.id] = s;
    }
    return m;
  }

  /// Runs `change` and flies every shared piece from where it was to where it
  /// ends up. `finalCell` gives a cell's rect once the morph has finished.
  void morph(VoidCallback change, {required Rect Function(String cell) finalCell, Map<String, String>? alias}) {
    _finishAll();
    final before = _snapNow();
    alias?.forEach((target, source) {
      before.remove(target);
      final s = before[source];
      if (s != null) before[target] = s;
    });
    _hide(before.keys.toSet());
    change();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      final host = context.findRenderObject() as RenderBox;
      final after = <String, _Snap>{};
      for (final t in _tags) {
        final s = t.measure(host, finalCell);
        if (s != null) after[t.widget.id] = s;
      }
      final flying = <String>{};
      for (final e in after.entries) {
        final o = before[e.key];
        if (o == null) continue;
        final f = _Flight(id: e.key, from: o, to: e.value, controller: AnimationController(vsync: this, duration: const Duration(milliseconds: 560)));
        if (!f.moved && f.same) {
          f.controller.dispose();
          continue;
        }
        _flights.add(f);
        flying.add(e.key);
        f.controller.forward().whenComplete(() => _land(f));
      }
      _hide(flying);
      setState(() {});
    });
  }

  void _land(_Flight f) {
    if (!_flights.remove(f)) return;
    f.controller.dispose();
    if (!mounted) return;
    final still = _flights.map((x) => x.id).toSet();
    _hide(still);
    setState(() {});
  }

  void _finishAll() {
    if (_flights.isEmpty) return;
    for (final f in _flights) {
      f.controller.dispose();
    }
    _flights.clear();
    _hide(const {});
  }

  @override
  void dispose() {
    for (final f in _flights) {
      f.controller.dispose();
    }
    hidden.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Stack(children: [
      Positioned.fill(child: widget.child),
      for (final f in _flights) _FlightView(key: ObjectKey(f), flight: f),
    ]);
  }
}

class _FlightView extends StatelessWidget {
  const _FlightView({super.key, required this.flight});

  final _Flight flight;

  @override
  Widget build(BuildContext context) {
    final f = flight;
    final c = f.controller;
    return AnimatedBuilder(
      animation: c,
      builder: (context, _) {
        final ms = c.value * 560;
        if (f.to.tag.isBox) {
          final r = Rect.lerp(f.from.rect, f.to.rect, easeCard.transform(c.value))!;
          return Positioned.fromRect(rect: r, child: IgnorePointer(child: f.to.tag.build(null)));
        }
        if (!f.moved) return _swap(f, ms);
        final t = easeCard.transform(c.value);
        final r = Offset.lerp(f.from.rect.topLeft, f.to.rect.topLeft, t)!;
        final style = _lerp(f.from.style!, f.to.style!, t);
        final landing = _floating(f.to.tag.build(style));
        if (f.same) return Positioned(left: r.dx, top: r.dy, child: landing);
        // new words on the way: the old roll up where they were, the new roll in
        final out = _revealFromBottom(_floating(f.from.tag.build(f.from.style)), ms, 220);
        final inn = _Reveal(t: easeCard.transform(((ms - 80) / 300).clamp(0.0, 1.0)), child: landing);
        return Stack(children: [
          Positioned(left: f.from.rect.left, top: f.from.rect.top, child: out),
          Positioned(left: r.dx, top: r.dy, child: inn),
        ]);
      },
    );
  }

  /// Same place, new words: the old slide up and out, the new up and in.
  Widget _swap(_Flight f, double ms) {
    final rect = f.to.rect;
    final inT = easeCard.transform(((ms - 90) / 420).clamp(0.0, 1.0));
    return Stack(children: [
      Positioned(left: f.from.rect.left, top: f.from.rect.top, child: _revealFromBottom(_floating(f.from.tag.build(f.from.style)), ms, 260, rise: .45)),
      Positioned(
        left: rect.left,
        top: rect.top,
        child: FractionalTranslation(translation: Offset(0, .45 * (1 - inT)), child: _Reveal(t: inT, child: _floating(f.to.tag.build(f.to.style)))),
      ),
    ]);
  }

  Widget _revealFromBottom(Widget child, double ms, double dur, {double rise = .3}) {
    final t = Curves.easeIn.transform((ms / dur).clamp(0.0, 1.0));
    return FractionalTranslation(translation: Offset(0, -rise * t), child: ClipRect(clipper: _BottomUp(1 - t), child: child));
  }

  Widget _floating(Widget child) => IgnorePointer(child: ConstrainedBox(constraints: const BoxConstraints(maxWidth: 800), child: child));

  TextStyle _lerp(TextStyle a, TextStyle b, double t) => b.copyWith(
        fontSize: a.fontSize! + (b.fontSize! - a.fontSize!) * t,
        color: Color.lerp(a.color, b.color, t),
        letterSpacing: (a.letterSpacing ?? 0) + ((b.letterSpacing ?? 0) - (a.letterSpacing ?? 0)) * t,
      );
}

class _BottomUp extends CustomClipper<Rect> {
  const _BottomUp(this.t);
  final double t;

  @override
  Rect getClip(Size size) => Rect.fromLTRB(-40, 0, size.width + 40, size.height * t);

  @override
  bool shouldReclip(_BottomUp old) => old.t != t;
}

class _Reveal extends StatelessWidget {
  const _Reveal({required this.t, required this.child});
  final double t;
  final Widget child;

  @override
  Widget build(BuildContext context) => ClipRect(clipper: _FromBottom(t), child: child);
}

class _FromBottom extends CustomClipper<Rect> {
  const _FromBottom(this.t);
  final double t;

  @override
  Rect getClip(Size size) => Rect.fromLTRB(-40, size.height * (1 - t), size.width + 40, size.height + 20);

  @override
  bool shouldReclip(_FromBottom old) => old.t != t;
}

/// A piece of content that flies between its places in the different views.
class FlyTag extends StatefulWidget {
  const FlyTag({super.key, required this.id, required this.text, required this.style, required this.build}) : isBox = false;

  /// A filled shape that moves and resizes, like the mode's underline.
  const FlyTag.box({super.key, required this.id, required this.build})
      : isBox = true,
        text = '',
        style = null;

  final String id, text;
  final TextStyle? style;
  final bool isBox;

  /// Draws the piece at a given style, so a flight can set its size each frame.
  final Widget Function(TextStyle? style) build;

  @override
  State<FlyTag> createState() => _FlyTagState();
}

class _FlyTagState extends State<FlyTag> {
  FlyHostState? _host;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _host ??= FlyHost.of(context).._tags.add(this);
  }

  @override
  void dispose() {
    _host?._tags.remove(this);
    super.dispose();
  }

  _Snap? measure(RenderBox host, Rect Function(String cell)? finalCell) {
    final scope = context.getInheritedWidgetOfExactType<CellScope>();
    if (scope == null || !scope.shown) return null;
    final box = context.findRenderObject() as RenderBox?;
    if (box == null || !box.hasSize || box.size.isEmpty || !box.attached) return null;
    final cellBox = _cellBox();
    if (cellBox == null) return null;
    var inCell = box.localToGlobal(Offset.zero, ancestor: cellBox);
    final Rect cell;
    if (finalCell != null) {
      cell = finalCell(scope.cell);
      inCell -= Offset(0, _lift());
    } else {
      final o = cellBox.localToGlobal(Offset.zero, ancestor: host);
      cell = o & cellBox.size;
    }
    // a piece clipped out of its cell has nowhere to fly from
    if (inCell.dy >= cell.height - 2 || inCell.dx >= cell.width - 2 || inCell.dy + box.size.height <= 2) return null;
    return _Snap(rect: (cell.topLeft + inCell) & box.size, tag: widget);
  }

  /// How far the entrances around this piece currently hold it off its resting place.
  double _lift() {
    var dy = 0.0;
    context.visitAncestorElements((e) {
      final w = e.widget;
      if (w is CellMark) return false;
      if (w is _Rising) dy += w.dy;
      return true;
    });
    return dy;
  }

  RenderBox? _cellBox() {
    RenderBox? found;
    context.visitAncestorElements((e) {
      if (e.widget is CellMark) {
        found = e.findRenderObject() as RenderBox?;
        return false;
      }
      return true;
    });
    return found;
  }

  @override
  Widget build(BuildContext context) {
    final host = FlyHost.of(context);
    return ValueListenableBuilder<Set<String>>(
      valueListenable: host.hidden,
      child: widget.build(widget.style),
      builder: (context, hidden, child) => Opacity(opacity: hidden.contains(widget.id) ? 0 : 1, child: child),
    );
  }
}

/// Marks the box that a cell's pieces are measured against.
class CellMark extends SingleChildRenderObjectWidget {
  const CellMark({super.key, required super.child});

  @override
  RenderObject createRenderObject(BuildContext context) => RenderProxyBox();
}
