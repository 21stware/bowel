import 'dart:math' as math;

import 'package:flutter/widgets.dart';

import '../ui/skin.dart';

/// Each skin's texture, laid over the whole screen and under the system bar.
class Deco extends StatelessWidget {
  const Deco({super.key});

  @override
  Widget build(BuildContext context) {
    final l = Skin.of(context);
    if (l.id == 'paper') return const SizedBox.shrink();
    return RepaintBoundary(child: CustomPaint(size: Size.infinite, painter: _DecoPainter(l)));
  }
}

class _DecoPainter extends CustomPainter {
  _DecoPainter(this.look);
  final Look look;

  void _vignette(Canvas c, Rect r, double start, Color edge) {
    final radius = math.sqrt(r.width * r.width + r.height * r.height) / 2;
    final shader = RadialGradient(
      radius: 1,
      colors: [edge.withValues(alpha: 0), edge],
      stops: [start, 1],
    ).createShader(Rect.fromCircle(center: r.center, radius: radius));
    c.save();
    c.translate(r.center.dx, r.center.dy);
    c.scale(r.width / (2 * radius) * math.sqrt2, r.height / (2 * radius) * math.sqrt2);
    c.translate(-r.center.dx, -r.center.dy);
    c.drawRect(Rect.fromCircle(center: r.center, radius: radius * 1.5), Paint()..shader = shader);
    c.restore();
  }

  void _glow(Canvas c, Rect r, Offset center, Color color, double reach) {
    final radius = math.sqrt(r.width * r.width + r.height * r.height) * reach;
    c.drawRect(
      r,
      Paint()
        ..shader = RadialGradient(colors: [color, color.withValues(alpha: 0)]).createShader(Rect.fromCircle(center: center, radius: radius)),
    );
  }

  void _grid(Canvas c, Size s, double step, Color color) {
    final p = Paint()..color = color;
    for (var x = 0.0; x < s.width; x += step) {
      c.drawRect(Rect.fromLTWH(x, 0, 1, s.height), p);
    }
    for (var y = 0.0; y < s.height; y += step) {
      c.drawRect(Rect.fromLTWH(0, y, s.width, 1), p);
    }
  }

  @override
  void paint(Canvas canvas, Size size) {
    final r = Offset.zero & size;
    final w = size.width, h = size.height;
    switch (look.id) {
      case 'phosphor':
        final line = Paint()..color = const Color(0x47000000);
        for (var y = 0.0; y < h; y += 3) {
          canvas.drawRect(Rect.fromLTWH(0, y, w, 1), line);
        }
        _vignette(canvas, r, .55, const Color(0x8C000000));
      case 'broadsheet':
        _vignette(canvas, r, .58, const Color(0x295A3C14));
      case 'neon':
        final media = look.route('media'), def = look.route('default');
        canvas.drawRect(Rect.fromLTWH(0, 0, w, 2), Paint()..shader = LinearGradient(colors: [media, def]).createShader(Rect.fromLTWH(0, 0, w, 2)));
        _glow(canvas, r, Offset(w * .08, h * .05), media.withValues(alpha: .17), .55);
        _glow(canvas, r, Offset(w * .95, h * .98), def.withValues(alpha: .14), .55);
      case 'blueprint':
        _grid(canvas, size, 16, const Color(0x0BF0F7FF));
        _grid(canvas, size, 80, const Color(0x1AF0F7FF));
      case 'sakura':
        final media = look.route('media');
        _glow(canvas, r, Offset(w * .92, h * .02), media.withValues(alpha: .15), .52);
        const petal = Offset.zero;
        final path = Path()
          ..moveTo(petal.dx, petal.dy)
          ..cubicTo(-7, -5, -4, -11, -1, -10)
          ..lineTo(0, -9)
          ..lineTo(1, -10)
          ..cubicTo(4, -11, 7, -5, 0, 0)
          ..close();
        for (var i = 0; i < 9; i++) {
          final px = 1 - math.pow((i * .13 + .07) % 1, 2);
          final py = math.pow((i * .17 + .05) % 1, 2).toDouble();
          canvas.save();
          canvas.translate(w * px, h * py);
          canvas.rotate(((i * 73) % 360) * math.pi / 180);
          final z = .8 + (i % 3) * .3;
          canvas.scale(z);
          canvas.drawPath(path, Paint()..color = media.withValues(alpha: .14 + .2 * (1 - py)));
          canvas.restore();
        }
    }
  }

  @override
  bool shouldRepaint(_DecoPainter old) => !identical(old.look, look);
}
