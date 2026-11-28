import 'package:flutter/gestures.dart' show GestureBinding, PointerDeviceKind, PointerDownEvent;
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

/// Focuses [node] the way a D-pad move does, scrolling it into view.
void focusTo(FocusNode node) => FocusTraversalPolicy.defaultTraversalRequestFocusCallback(node);

/// The ring a focused target wears, drawn inside it, so on its [fill] where
/// it has one and on the paper where it hasn't: the theme color where it
/// stands out there, else whichever of the ink, the paper and the color
/// drawn on the theme color stands out most (a lime ring on a lime button
/// would be no ring at all). On a fill that stands apart from the paper the
/// ring keeps in from the edge, with a band of the fill outside it: a ring
/// at the edge in the paper's color would only look like a smaller button.
({Color color, double inset}) focusRing(Look l, {Color? fill}) {
  final under = fill == null ? l.paper : Color.alphaBlend(fill, l.paper);
  var c = l.accent;
  if (contrast(c, under) < 3) {
    for (final o in [l.ink, l.onAccent, l.paper]) {
      if (contrast(o, under) > contrast(c, under)) c = o;
    }
  }
  return (color: c, inset: contrast(under, l.paper) >= 1.4 ? 3.5 : 0);
}

const _ringWidth = 2.5;

class _RingPainter extends CustomPainter {
  _RingPainter(this.color, this.inset, this.radius);
  final Color color;
  final double inset;
  final BorderRadius radius;

  @override
  void paint(Canvas canvas, Size size) {
    final ring = radius.toRRect(Offset.zero & size).deflate(inset + _ringWidth / 2);
    canvas.drawRRect(
      ring,
      Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = _ringWidth
        ..color = color,
    );
  }

  @override
  bool shouldRepaint(_RingPainter old) => old.color != color || old.inset != inset || old.radius != radius;
}

/// Something OK presses. A finger still goes through the [child]'s own
/// gesture detector; this only adds the focus, the ring and the key.
class KeyTarget extends StatefulWidget {
  const KeyTarget({super.key, required this.onActivate, required this.child, this.radius = 10, this.focusNode, this.fill});

  /// Null leaves it out of the focus order, as a disabled button is.
  final VoidCallback? onActivate;
  final Widget child;
  final double radius;
  final FocusNode? focusNode;

  /// What the target is filled with, which its ring must stand out on.
  final Color? fill;

  @override
  State<KeyTarget> createState() => _KeyTargetState();
}

class _KeyTargetState extends State<KeyTarget> {
  bool _lit = false;

  /// The same tree lit or not, so what's inside keeps its state.
  Widget _ringed(BuildContext context) {
    if (!_lit) return CustomPaint(child: widget.child);
    final l = Skin.of(context);
    final ring = focusRing(l, fill: widget.fill);
    return CustomPaint(foregroundPainter: _RingPainter(ring.color, ring.inset, l.br(widget.radius)), child: widget.child);
  }

  @override
  Widget build(BuildContext context) {
    final on = widget.onActivate;
    return FocusableActionDetector(
      enabled: on != null,
      focusNode: widget.focusNode,
      onShowFocusHighlight: (v) {
        if (v != _lit) setState(() => _lit = v);
      },
      actions: {
        ActivateIntent: CallbackAction<ActivateIntent>(
          onInvoke: (_) {
            on?.call();
            return null;
          },
        ),
      },
      child: _ringed(context),
    );
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
    return FocusTraversalGroup(
      child: Focus(focusNode: _node, canRequestFocus: false, skipTraversal: true, descendantsAreFocusable: open, descendantsAreTraversable: open, child: widget.child),
    );
  }
}
