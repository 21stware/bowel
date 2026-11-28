import 'dart:math' as math;

import 'package:flutter/gestures.dart' show GestureBinding, PointerDeviceKind, PointerDownEvent;
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

import 'skin.dart';

// A TV remote, or a keyboard, moves a focus around the screen: the D-pad
// moves it within the level that is up (the board, an opened card, a sheet,
// a page), OK presses what has it, and Back goes up a level. Under touch
// none of this shows: the ring is drawn only while keys are driving, and
// levels take focus as they open only then.

/// Keys, not a finger, are driving: Flutter's own switch, flipped by the
/// last key or touch.
bool get keyDriven => FocusManager.instance.highlightMode == FocusHighlightMode.traditional;

/// The keys a remote moves and presses with. None of them types.
final _remoteKeys = {
  LogicalKeyboardKey.arrowUp,
  LogicalKeyboardKey.arrowDown,
  LogicalKeyboardKey.arrowLeft,
  LogicalKeyboardKey.arrowRight,
  LogicalKeyboardKey.select,
  LogicalKeyboardKey.enter,
  LogicalKeyboardKey.numpadEnter,
  LogicalKeyboardKey.gameButtonA,
  LogicalKeyboardKey.goBack,
  LogicalKeyboardKey.tab,
};

bool _watching = false;

/// Puts keys in charge on a remote's key, and a finger back on a touch.
///
/// Flutter leaves touch mode on a key unless Android says it came from its
/// virtual keyboard (device -1), which it takes for the soft keyboard. But
/// `adb shell input keyevent`, and the remotes some TV boxes wire in the
/// same way, send their D-pad as that device: focus then moves, but in touch
/// mode, so no ring shows and no level takes focus. A D-pad key is no soft
/// keyboard's typing, unless Android flags it so (a soft keyboard's cursor
/// keys, or a key that asks to keep touch mode).
void watchRemoteKeys() {
  if (_watching) return;
  _watching = true;
  // In front of FocusManager's own handler, by the same deprecated way it is
  // set: only Android's raw message says whether a soft keyboard sent the key.
  // ignore: deprecated_member_use
  final keys = ServicesBinding.instance.keyEventManager;
  // ignore: deprecated_member_use
  final inner = keys.keyMessageHandler;
  // ignore: deprecated_member_use
  keys.keyMessageHandler = (message) {
    if (_fromRemote(message)) FocusManager.instance.highlightStrategy = FocusHighlightStrategy.alwaysTraditional;
    return inner?.call(message) ?? false;
  };
  GestureBinding.instance.pointerRouter.addGlobalRoute((event) {
    final touch = event.kind == PointerDeviceKind.touch || event.kind == PointerDeviceKind.stylus || event.kind == PointerDeviceKind.invertedStylus;
    if (event is PointerDownEvent && touch && FocusManager.instance.highlightStrategy == FocusHighlightStrategy.alwaysTraditional) {
      FocusManager.instance.highlightStrategy = FocusHighlightStrategy.automatic;
    }
  });
}

// ignore: deprecated_member_use
bool _fromRemote(KeyMessage message) {
  // ignore: deprecated_member_use
  final raw = message.rawEvent?.data;
  // ignore: deprecated_member_use
  if (raw is RawKeyEventDataAndroid && raw.flags & 0x6 != 0) return false; // FLAG_SOFT_KEYBOARD, FLAG_KEEP_TOUCH_MODE
  return message.events.any((e) => e is KeyDownEvent && _remoteKeys.contains(e.logicalKey));
}

/// How long a focus takes to come and go, or nothing where the system asks
/// for no motion.
Duration focusMotion(BuildContext context) => (MediaQuery.maybeDisableAnimationsOf(context) ?? false) ? Duration.zero : const Duration(milliseconds: 150);

const focusCurve = Curves.easeOut;

/// How far past a focused target a list keeps scrolled, so its growth and
/// shadow show too.
const _revealMargin = 16.0;

final _levelPolicy = ReadingOrderTraversalPolicy(requestFocusCallback: focusTo);

/// Focuses [node] the way a D-pad move does, scrolling it into view with
/// room round it for the focus to grow into.
void focusTo(FocusNode node, {ScrollPositionAlignmentPolicy? alignmentPolicy, double? alignment, Duration? duration, Curve? curve}) {
  node.requestFocus();
  final box = node.context?.findRenderObject();
  if (box is! RenderBox || !box.attached || !box.hasSize) return;
  box.showOnScreen(rect: (Offset.zero & box.size).inflate(_revealMargin), duration: duration ?? const Duration(milliseconds: 150), curve: curve ?? focusCurve);
}

/// How a focused target shows it, as a TV's own interface does, with no
/// outline anywhere. A card or a row lifts: it grows a little and its
/// surface comes up lighter than its neighbours'. A button, chip, segment,
/// icon box or tab inverts: ink fill, the paper's colour on it, and it grows
/// more.
enum FocusLook { lift, invert }

/// A lifted surface: a step lighter than anything a dark skin draws, and
/// white on a light one's off-white.
Color liftedSurface(Look l) => l.isDark ? Color.lerp(l.sheet, l.ink, .14)! : const Color(0xFFFFFFFF);

List<BoxShadow> liftShadow(Look l) => [BoxShadow(color: Color(l.isDark ? 0x73000000 : 0x29000000), blurRadius: 18, offset: const Offset(0, 6))];

/// Whether the target round [context] has the focus, keys driving. Targets
/// that draw their own fill read it and lift or invert that.
bool focusLit(BuildContext context) => context.dependOnInheritedWidgetOfExactType<_Lit>()?.lit ?? false;

class _Lit extends InheritedWidget {
  const _Lit({required this.lit, required super.child});
  final bool lit;

  @override
  bool updateShouldNotify(_Lit old) => old.lit != lit;
}

/// Something OK presses. A finger still goes through the [child]'s own
/// gesture detector; this only adds the focus, its look and the key.
class KeyTarget extends StatefulWidget {
  const KeyTarget({super.key, required this.onActivate, required this.child, this.radius = 10, this.focusNode, this.look = FocusLook.lift, this.scale, this.surface = true, this.bleed = 0, this.onLit});

  /// Null leaves it out of the focus order, as a disabled button is.
  final VoidCallback? onActivate;
  final Widget child;

  /// The corners of the lifted surface, to match the child's.
  final double radius;
  final FocusNode? focusNode;
  final FocusLook look;

  /// How much it grows; by default 3.5% lifted, 7% inverted.
  final double? scale;

  /// A lifted target draws its surface and shadow behind the child. Off for
  /// one that lifts something of its own, as a text field lifts its box.
  final bool surface;

  /// Told when the focus comes and goes, keys driving.
  final ValueChanged<bool>? onLit;

  /// How far a lifted surface reaches past the sides, for a row whose text
  /// runs to its edges. Drawn past them, so nothing moves.
  final double bleed;

  @override
  State<KeyTarget> createState() => _KeyTargetState();
}

class _KeyTargetState extends State<KeyTarget> {
  bool _lit = false;

  @override
  Widget build(BuildContext context) {
    final on = widget.onActivate;
    final l = Skin.of(context);
    final motion = focusMotion(context);
    final grow = widget.scale ?? (widget.look == FocusLook.lift ? 1.035 : 1.07);
    // The same tree lit or not, so what's inside keeps its state.
    Widget child = widget.child;
    if (widget.look == FocusLook.lift && widget.surface) {
      final lifted = liftedSurface(l);
      final b = widget.bleed;
      child = Stack(clipBehavior: Clip.none, fit: StackFit.passthrough, children: [
        Positioned(
          left: -b,
          right: -b,
          top: 0,
          bottom: 0,
          child: AnimatedContainer(
            duration: motion,
            curve: focusCurve,
            decoration: BoxDecoration(color: _lit ? lifted : lifted.withValues(alpha: 0), borderRadius: l.br(widget.radius), boxShadow: _lit ? liftShadow(l) : const []),
          ),
        ),
        child,
      ]);
    }
    return FocusableActionDetector(
      enabled: on != null,
      focusNode: widget.focusNode,
      onShowFocusHighlight: (v) {
        if (v == _lit) return;
        setState(() => _lit = v);
        widget.onLit?.call(v);
      },
      actions: {
        ActivateIntent: CallbackAction<ActivateIntent>(
          onInvoke: (_) {
            on?.call();
            return null;
          },
        ),
      },
      child: _Lit(
        lit: _lit,
        child: AnimatedScale(scale: _lit ? grow : 1, duration: motion, curve: focusCurve, child: child),
      ),
    );
  }
}

/// An inverted target's fill and the colour of what's on it: the ink and the
/// paper. A target already filled with the ink, as a chosen chip is, would
/// look the same focused: it takes the paper then, with the ink on it.
({Color fill, Color on}) invertedOf(Look l, {Color? fill}) {
  if (fill == null) return (fill: l.ink, on: l.paper);
  final f = Color.alphaBlend(fill, l.paper);
  final d = math.sqrt(math.pow(f.r - l.ink.r, 2) + math.pow(f.g - l.ink.g, 2) + math.pow(f.b - l.ink.b, 2));
  return d < .12 ? (fill: l.paper, on: l.ink) : (fill: l.ink, on: l.paper);
}

/// Turns everything drawn in [child] one colour, [on], on an inverted target:
/// its text and icons, whatever colours they were given. Always in the tree,
/// so what's inside keeps its state.
Widget invertContent(Color? on, Widget child) => ColorFiltered(colorFilter: on != null ? ColorFilter.mode(on, BlendMode.srcIn) : const ColorFilter.mode(Color(0x00000000), BlendMode.dst), child: child);

/// Marks a selected tab, segment or chip that also has the focus, so it
/// stands apart from one that only has it.
Widget selectedMark(Look l, {double width = 16}) {
  final c = contrast(l.accent, l.ink) >= 2 ? l.accent : l.paper;
  return Container(width: width, height: 3, decoration: BoxDecoration(color: c, borderRadius: BorderRadius.circular(1.5)));
}

/// Lifts the nearest filled box round a text field, which isn't the field's
/// own: setup, sheets and pages each draw it their way. [t] runs from 0, not
/// lifted, to 1.
class LiftBox extends SingleChildRenderObjectWidget {
  const LiftBox({super.key, required this.t, required this.color, required this.shadow, super.child});
  final double t;
  final Color color;
  final List<BoxShadow> shadow;

  @override
  RenderObject createRenderObject(BuildContext context) => RenderLiftBox(t, color, shadow);

  @override
  void updateRenderObject(BuildContext context, RenderLiftBox renderObject) => renderObject
    ..t = t
    ..color = color
    ..shadow = shadow;
}

class RenderLiftBox extends RenderProxyBox {
  RenderLiftBox(this._t, this._color, this._shadow);
  double _t;
  Color _color;
  List<BoxShadow> _shadow;

  set t(double v) {
    if (v == _t) return;
    _t = v;
    markNeedsPaint();
  }

  set color(Color v) {
    if (v == _color) return;
    _color = v;
    markNeedsPaint();
  }

  set shadow(List<BoxShadow> v) {
    _shadow = v;
    markNeedsPaint();
  }

  /// The filled box round the field, a few boxes up at most.
  RenderDecoratedBox? _box() {
    RenderObject? o = parent;
    for (var i = 0; i < 8 && o != null; i++) {
      if (o is RenderDecoratedBox && o.decoration is BoxDecoration && (o.decoration as BoxDecoration).color != null) return o;
      o = o.parent;
    }
    return null;
  }

  @override
  void paint(PaintingContext context, Offset offset) {
    final box = _t > 0 ? _box() : null;
    if (box != null) {
      final rect = MatrixUtils.transformRect(box.getTransformTo(this), Offset.zero & box.size).shift(offset);
      final radius = (box.decoration as BoxDecoration).borderRadius?.resolve(TextDirection.ltr) ?? BorderRadius.zero;
      final rrect = radius.toRRect(rect);
      for (final s in _shadow) {
        context.canvas.drawRRect(rrect.shift(s.offset), s.toPaint()..color = s.color.withValues(alpha: s.color.a * _t));
      }
      context.canvas.drawRRect(rrect, Paint()..color = _color.withValues(alpha: _color.a * _t));
    }
    super.paint(context, offset);
  }
}

/// Marks the target under it, so a level can pick it to focus first or
/// return to: the chosen option, the row a page was opened from.
class FocusMark extends StatelessWidget {
  const FocusMark(this.id, {super.key, required this.child});
  final String id;
  final Widget child;

  @override
  Widget build(BuildContext context) => child;
}

/// One level the D-pad moves in. Only an [active] level that isn't [covered]
/// lets anything in it take focus. When it opens, or [entry] changes, with
/// keys driving: what had focus there last, else the target marked
/// [initial], else the first in reading order. With [restore], focus goes
/// back to where it came from when the level closes.
class FocusLevel extends StatefulWidget {
  const FocusLevel({super.key, required this.child, this.active = true, this.covered = false, this.restore = false, this.take = true, this.initial, this.entry});

  final Widget child;
  final bool active, covered, restore;

  /// Takes focus from wherever it is. Without, only focus that was lost: a
  /// tab's page leaves the D-pad on the tab that opened it.
  final bool take;
  final String? initial;

  /// A change opens the level afresh: an opened card that shows another page.
  final Object? entry;

  @override
  State<FocusLevel> createState() => _FocusLevelState();
}

class _FocusLevelState extends State<FocusLevel> {
  final _node = FocusNode(debugLabel: 'level', skipTraversal: true, canRequestFocus: false);
  FocusNode? _from, _last;

  @override
  void initState() {
    super.initState();
    if (widget.active) _enter();
  }

  @override
  void didUpdateWidget(FocusLevel old) {
    super.didUpdateWidget(old);
    if (widget.active && !old.active) {
      _enter();
    } else if (!widget.active && old.active) {
      _leave();
    } else if (widget.active && widget.entry != old.entry) {
      _last = null;
      _enter(force: true);
    }
  }

  @override
  void deactivate() {
    if (widget.active) _leave();
    super.deactivate();
  }

  @override
  void dispose() {
    _node.dispose();
    super.dispose();
  }

  void _enter({bool force = false}) {
    final now = FocusManager.instance.primaryFocus;
    if (!force) _from = now == null || now is FocusScopeNode || _inside(now) ? null : now;
    WidgetsBinding.instance.addPostFrameCallback((_) => _land(force: force));
  }

  /// Puts the focus in the level. An opened card is still growing into place,
  /// with its list not yet built down to the [FocusLevel.initial] row: for a
  /// few frames, until the D-pad moves, focus goes on to that row when it is.
  void _land({required bool force, FocusNode? landed, int frames = 0}) {
    if (!mounted || !widget.active || widget.covered || !keyDriven) return;
    if (landed == null) {
      if (!force && _node.hasFocus) return;
      final now = FocusManager.instance.primaryFocus;
      if (!force && !widget.take && now != null && now is! FocusScopeNode && now.canRequestFocus) return;
      landed = _pick();
      if (landed == null) return;
      focusTo(landed);
    } else {
      if (FocusManager.instance.primaryFocus != landed) return;
      final marked = _marked();
      if (marked != null) {
        if (marked != landed) focusTo(marked);
        return;
      }
    }
    if (widget.initial == null || _markOf(landed) == widget.initial || frames >= 40) return;
    final at = landed;
    WidgetsBinding.instance.addPostFrameCallback((_) => _land(force: force, landed: at, frames: frames + 1));
  }

  void _leave() {
    final now = FocusManager.instance.primaryFocus;
    final had = now != null && _inside(now);
    if (had) _last = now;
    final from = _from;
    _from = null;
    if (!widget.restore || !had || from == null) return;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (keyDriven && from.context != null && from.canRequestFocus) focusTo(from);
    });
  }

  bool _inside(FocusNode n) => n == _node || n.ancestors.contains(_node);

  static String? _markOf(FocusNode n) => n.context?.findAncestorWidgetOfExactType<FocusMark>()?.id;

  List<FocusNode> get _targets => _node.traversalDescendants.where((n) => n is! FocusScopeNode && n.context != null).toList();

  FocusNode? _marked() {
    final want = widget.initial;
    return want == null ? null : _targets.where((n) => _markOf(n) == want).firstOrNull;
  }

  FocusNode? _pick() {
    final last = _last;
    if (last != null && last.context != null && last.canRequestFocus && _inside(last)) return last;
    final all = _targets;
    if (all.isEmpty) return null;
    return _marked() ?? ReadingOrderTraversalPolicy().sortDescendants(all, _node).first;
  }

  @override
  Widget build(BuildContext context) {
    final open = widget.active && !widget.covered;
    // a D-pad move scrolls with room round the target, for its focus to grow into
    return FocusTraversalGroup(
      policy: _levelPolicy,
      child: Focus(focusNode: _node, canRequestFocus: false, skipTraversal: true, descendantsAreFocusable: open, descendantsAreTraversable: open, child: widget.child),
    );
  }
}
