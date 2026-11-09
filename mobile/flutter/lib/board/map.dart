import 'dart:convert';
import 'dart:math' as math;
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/scheduler.dart';
import 'package:flutter/widgets.dart';

import '../app/i18n.dart';
import '../app/sample.dart';
import '../app/state.dart';
import '../ui/land.dart';
import '../ui/skin.dart';

final Uint8List _land = base64.decode(landBase64);

bool _isLand(double lat, double lon) {
  final r = (90 - lat).floor();
  final c = (((lon + 180) % 360 + 360) % 360).floor();
  if (r < 0 || r >= 180) return false;
  final i = r * 360 + c;
  return (_land[i >> 3] >> (i & 7)) & 1 == 1;
}

class _Arc {
  _Arc(this.x, this.y);
  double x, y, width = 0, alpha = 0;
}

class _Particle {
  _Particle(this.route, this.v, this.color);
  final String route;
  final double v;
  final Color color;
  double t = 0;
}

/// Everything the map keeps between frames. It eases toward its targets, so a
/// route that changes exit swings across rather than jumping.
class _MapState {
  final arcs = <String, _Arc>{};
  var particles = <_Particle>[];
  double pad = 0;
  String? dotsKey;
  Float32List dots = Float32List(0);
  final labels = <String, TextPainter>{};
}

class ExitMap extends StatefulWidget {
  const ExitMap({super.key, required this.state, required this.expanded});

  final AppState state;
  final bool expanded;

  @override
  State<ExitMap> createState() => _ExitMapState();
}

class _ExitMapState extends State<ExitMap> with SingleTickerProviderStateMixin {
  late final Ticker _ticker;
  final _s = _MapState();
  final _tick = ValueNotifier<int>(0);
  Duration _last = Duration.zero;
  double _dt = 1 / 60;
  final _random = math.Random(7);

  @override
  void initState() {
    super.initState();
    _ticker = createTicker((elapsed) {
      _dt = ((elapsed - _last).inMicroseconds / 1e6).clamp(1 / 240, 1 / 20);
      _last = elapsed;
      _tick.value++;
    })
      ..start();
  }

  @override
  void dispose() {
    _ticker.dispose();
    _tick.dispose();
    for (final l in _s.labels.values) {
      l.dispose();
    }
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return RepaintBoundary(
      child: CustomPaint(
        size: Size.infinite,
        painter: _MapPainter(
          repaint: _tick,
          app: widget.state,
          expanded: widget.expanded,
          look: Skin.of(context),
          s: _s,
          dt: () => _dt,
          random: _random,
          reduced: MediaQuery.disableAnimationsOf(context),
        ),
      ),
    );
  }
}

class _Line {
  _Line(this.route, this.node, this.place, this.live, this.arc, this.path);
  final RouteCfg route;
  final Node node;
  final Place? place;
  final bool live;
  final _Arc arc;
  final List<Offset> path;
}

class _MapPainter extends CustomPainter {
  _MapPainter({required Listenable repaint, required this.app, required this.expanded, required this.look, required this.s, required this.dt, required this.random, required this.reduced}) : super(repaint: repaint);

  final AppState app;
  final bool expanded, reduced;
  final Look look;
  final _MapState s;
  final double Function() dt;
  final math.Random random;

  double _ease(double rate) => 1 - math.pow(1 - rate, dt() * 60).toDouble();
  double _lerp(double a, double b, double k) => a + (b - a) * k;

  List<Offset> _arc(Offset a, Offset b) {
    final m = (a + b) / 2;
    final d = b - a;
    return [a, Offset(m.dx - d.dy * .22, m.dy - d.distance * .28), b];
  }

  Offset _qp(List<Offset> p, double t) => p[0] * ((1 - t) * (1 - t)) + p[1] * (2 * (1 - t) * t) + p[2] * (t * t);

  void _label(Canvas c, String text, double x, double y, bool bold, TextAlign align) {
    final key = '$text|$bold|${look.id}|${look.isDark}';
    TextPainter make(bool stroke) {
      final base = look.text(bold ? 13 : 11.5, weight: bold ? 700 : 500, lh: bold ? 16 : 14);
      final style = stroke
          ? base.copyWith(
              color: null,
              foreground: Paint()
                ..style = PaintingStyle.stroke
                ..strokeWidth = 3
                ..color = look.paper,
              shadows: const [])
          : base.copyWith(shadows: const []);
      return TextPainter(text: TextSpan(text: text, style: style), textDirection: TextDirection.ltr)..layout();
    }

    final stroke = s.labels['$key|s'] ??= make(true);
    final fill = s.labels[key] ??= make(false);
    final dx = align == TextAlign.right ? x - fill.width : x;
    final o = Offset(dx, y - fill.height * .75);
    stroke.paint(c, o);
    fill.paint(c, o);
  }

  @override
  void paint(Canvas canvas, Size size) {
    final w = size.width, h = size.height;
    if (w <= 0 || h <= 0) return;
    final ease14 = _ease(.14), ease10 = _ease(.1);
    s.pad = _lerp(s.pad, expanded ? 260 : 0, ease14);
    final hh = math.max(40.0, h - s.pad);
    final k = math.max(2.4, math.min(hh / 46, w / 70));
    final latTop = 21 + hh / (2 * k);
    final lonLeft = 121.5 - w * .556 / k;
    Offset at(double lat, double lon) => Offset((lon - lonLeft) * k, (latTop - lat) * k);

    final key = '${w.round()}|${h.round()}|${hh.round()}';
    if (s.dotsKey != key) {
      s.dotsKey = key;
      final pts = <double>[];
      for (var y = 3.0; y < h; y += 7) {
        for (var x = 3.0; x < w; x += 7) {
          if (_isLand(latTop - y / k, x / k + lonLeft)) pts.addAll([x, y]);
        }
      }
      s.dots = Float32List.fromList(pts);
    }
    canvas.drawRawPoints(
      ui.PointMode.points,
      s.dots,
      Paint()
        ..color = look.faint
        ..strokeWidth = 1.9
        ..strokeCap = StrokeCap.round,
    );

    final you = at(31.2, 121.5);
    final on = app.on;
    bool isLive(RouteCfg r) => on && (app.mode == 'rule' || (app.mode == 'global' && r.id == 'default'));
    final small = h < 100;
    final lines = <_Line>[];
    for (final r in app.proxied) {
      final n = node(r.node!);
      final pl = n.flag.isEmpty ? null : places[n.flag];
      double pin() => at(pl!.lat, 0).dy.clamp(14.0, math.max(14.0, hh - 12));
      final Offset to;
      if (pl == null) {
        to = Offset(w * .68, hh * .72);
      } else if (pl.edge) {
        to = Offset(w - 16, pin());
      } else if (pl.west) {
        to = Offset(16, pin());
      } else {
        to = at(pl.lat, pl.lon);
      }
      final a = s.arcs[r.id] ??= _Arc(you.dx, you.dy);
      final lv = isLive(r);
      a.x = _lerp(a.x, to.dx, ease14);
      a.y = _lerp(a.y, to.dy, ease14);
      a.width = _lerp(a.width, lv ? 1 + share[r.id]! * 8 : (on ? 1 : 0), ease10);
      a.alpha = _lerp(a.alpha, lv ? .8 : (on ? .25 : 0), ease10);
      lines.add(_Line(r, n, pl, lv, a, _arc(you, Offset(a.x, a.y))));
    }

    for (final l in lines) {
      if (l.arc.alpha < .01) continue;
      final path = Path()
        ..moveTo(l.path[0].dx, l.path[0].dy)
        ..quadraticBezierTo(l.path[1].dx, l.path[1].dy, l.path[2].dx, l.path[2].dy);
      final paint = Paint()
        ..style = PaintingStyle.stroke
        ..strokeCap = StrokeCap.round
        ..strokeWidth = l.arc.width
        ..color = (l.live ? look.route(l.route.id) : look.faint).withValues(alpha: l.arc.alpha);
      canvas.drawPath(l.place != null ? path : _dashed(path, 3, 5), paint);
      if (!reduced && l.live && l.place != null && random.nextDouble() < (share[l.route.id]! * .4 + .03) * dt() * 60) {
        s.particles.add(_Particle(l.route.id, .007 + random.nextDouble() * .004, look.route(l.route.id)));
      }
    }

    final step = dt() * 60;
    s.particles = s.particles.where((p) => (p.t += p.v * step) < 1).toList();
    for (final p in s.particles) {
      final l = lines.where((x) => x.route.id == p.route).firstOrNull;
      if (l == null || !l.live) continue;
      final o = _qp(l.path, p.t);
      canvas.drawCircle(o, 2.1, Paint()..color = look.paper);
      canvas.drawCircle(o, 1.4, Paint()..color = p.color);
    }

    final seen = <String>{};
    final head = lines.first;
    for (final l in lines) {
      if (l.arc.alpha < .05 || l.place == null) continue;
      if (!seen.add(l.node.flag)) continue;
      final main = head.node.flag == l.node.flag;
      final alpha = math.min(1.0, l.arc.alpha * 1.4);
      final o = Offset(l.arc.x, l.arc.y);
      canvas.drawCircle(o, main ? 5.5 : 4, Paint()..color = look.paper.withValues(alpha: alpha));
      canvas.drawCircle(
        o,
        main ? 5.5 : 4,
        Paint()
          ..style = PaintingStyle.stroke
          ..strokeWidth = main ? 2.5 : 1.6
          ..color = look.ink.withValues(alpha: alpha),
      );
      if (!small || main) {
        final pl = l.place!;
        final city = l.node.city.isEmpty ? regionName(l.node.flag) : l.node.city;
        if (pl.edge) {
          _label(canvas, '$city →', o.dx - 10, o.dy + 4, main, TextAlign.right);
        } else if (pl.west) {
          _label(canvas, '← $city', o.dx + 10, o.dy + 4, main, TextAlign.left);
        } else {
          _label(canvas, city, o.dx + 10, o.dy + 4, main, TextAlign.left);
        }
      }
    }

    final ink = on ? look.ink : look.mute;
    canvas.drawCircle(you, 4.8, Paint()..color = ink);
    if (!reduced) {
      final ph = (DateTime.now().millisecondsSinceEpoch / 60) % 14;
      canvas.drawCircle(
        you,
        8 + ph,
        Paint()
          ..style = PaintingStyle.stroke
          ..strokeWidth = 1
          ..color = ink.withValues(alpha: math.max(0, .3 - ph / 46)),
      );
    }
    if (!small) _label(canvas, tr('你', 'You'), you.dx - 9, you.dy - 7, true, TextAlign.right);
  }

  Path _dashed(Path source, double on, double off) {
    final out = Path();
    for (final m in source.computeMetrics()) {
      var d = 0.0;
      while (d < m.length) {
        out.addPath(m.extractPath(d, math.min(d + on, m.length)), Offset.zero);
        d += on + off;
      }
    }
    return out;
  }

  @override
  bool shouldRepaint(_MapPainter old) => !identical(old.look, look) || old.expanded != expanded;
}
