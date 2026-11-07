// The exit map. Redrawn every frame at the cell's current size, so it
// follows the morph. The view fits every known point (你 and the exit of
// each route), equirectangular, with each longitude wrapped to the side
// nearest to 你 (US ↔ Asia goes across the Pacific). Arcs, their width and
// opacity, and the projection itself ease toward their targets.

import { S, active, exitNode, nodeByName, routes, takeoverOn } from "./store";
import type { Place } from "./mygo";

type Pt = [number, number];
interface Arc { x: number; y: number; wd: number; a: number; share: number }

let raf = 0;
let canvas: HTMLCanvasElement | null = null;
const arcs: Record<string, Arc> = {};
let particles: { id: string; t: number; v: number; col: string }[] = [];
let gridA = 1;
const proj = { cx: 0, cy: 0, k: 0, ox: 0, oy: 0, cos: 1, ready: false };

const lerp = (a: number, b: number, k: number) => a + (b - a) * k;
const arcOf = (a: Pt, b: Pt): [Pt, Pt, Pt] => {
  const mx = (a[0] + b[0]) / 2, my = (a[1] + b[1]) / 2, dx = b[0] - a[0], dy = b[1] - a[1];
  const d = Math.hypot(dx, dy), side = dx >= 0 ? 1 : -1;
  return [a, [mx - dy * 0.22 * side, my - d * 0.28], b];
};
const qp = ([a, c, b]: [Pt, Pt, Pt], t: number): Pt => [
  (1 - t) ** 2 * a[0] + 2 * (1 - t) * t * c[0] + t * t * b[0],
  (1 - t) ** 2 * a[1] + 2 * (1 - t) * t * c[1] + t * t * b[1],
];

export function startMap(c: HTMLCanvasElement): void {
  cancelAnimationFrame(raf);
  canvas = c;
  proj.ready = false;
  const frame = () => {
    if (!canvas || !canvas.isConnected) return;
    draw(canvas);
    raf = requestAnimationFrame(frame);
  };
  frame();
}
export function stopMap(): void { cancelAnimationFrame(raf); canvas = null; }

function colors() {
  const cs = getComputedStyle(document.documentElement), v = (n: string) => cs.getPropertyValue(n).trim();
  return { ink: v("--ink"), faint: v("--faint"), hair: v("--hair"), paper: v("--paper"), mute: v("--mute"), warn: v("--warn"), serif: v("--serif"), sans: v("--sans"), v };
}

function draw(c: HTMLCanvasElement): void {
  const g = c.getContext("2d");
  if (!g) return;
  const dpr = devicePixelRatio || 1, w = c.clientWidth, h = c.clientHeight;
  if (!w || !h) return;
  if (c.width !== Math.round(w * dpr) || c.height !== Math.round(h * dpr)) { c.width = Math.round(w * dpr); c.height = Math.round(h * dpr); }
  g.setTransform(dpr, 0, 0, dpr, 0, 0);
  g.clearRect(0, 0, w, h);
  const col = colors();
  const on = takeoverOn();
  const small = w < 260 || h < 160;
  const expanded = S.x === "map";

  // ---- what is on the map ----
  const you = S.snap.you?.place;
  const rs = routes().filter((r) => !r.direct);
  const exits = rs.map((r) => ({ r, n: nodeByName(r.node) }));
  const known: Place[] = on ? exits.map((e) => e.n?.where.place).filter((p): p is Place => !!p) : [];
  const refLon = you?.lon ?? (known.length ? known.reduce((a, p) => a + p.lon, 0) / known.length : 110);
  const wrap = (lon: number) => lon + 360 * Math.round((refLon - lon) / 360);

  // ---- projection: fit every known point, with room for the captions ----
  const pts = [...(you ? [you] : []), ...known];
  const lats = pts.map((p) => p.lat), lons = pts.map((p) => wrap(p.lon));
  const midLat = pts.length ? (Math.min(...lats) + Math.max(...lats)) / 2 : 30;
  const cos = Math.cos((Math.max(-60, Math.min(60, midLat)) * Math.PI) / 180);
  let x0 = pts.length ? Math.min(...lons) * cos : 100 * cos, x1 = pts.length ? Math.max(...lons) * cos : 130 * cos;
  let y0 = pts.length ? -Math.max(...lats) : -45, y1 = pts.length ? -Math.min(...lats) : -15;
  const minX = 26 * cos + 10, minY = 16;
  if (x1 - x0 < minX) { const m = (x0 + x1) / 2; x0 = m - minX / 2; x1 = m + minX / 2; }
  if (y1 - y0 < minY) { const m = (y0 + y1) / 2; y0 = m - minY / 2; y1 = m + minY / 2; }
  const pad = { l: small ? 18 : 40, r: (small ? 18 : 90) + (expanded ? 300 : 0), t: small ? 18 : 64, b: small ? 18 : 48 };
  const aw = Math.max(40, w - pad.l - pad.r), ah = Math.max(40, h - pad.t - pad.b);
  const tk = Math.min(aw / (x1 - x0), ah / (y1 - y0));
  const t = { cx: (x0 + x1) / 2, cy: (y0 + y1) / 2, k: tk, ox: pad.l + aw / 2, oy: pad.t + ah / 2, cos };
  if (!proj.ready) { Object.assign(proj, t); proj.ready = true; }
  else for (const key of ["cx", "cy", "k", "ox", "oy", "cos"] as const) proj[key] = lerp(proj[key], t[key], 0.08);
  const P = (p: Place): Pt => [proj.ox + (wrap(p.lon) * proj.cos - proj.cx) * proj.k, proj.oy + (-p.lat - proj.cy) * proj.k];

  // ---- graticule, faded out when not taking over ----
  gridA = lerp(gridA, on ? 0.6 : 0.25, 0.08);
  const degPx = proj.k * proj.cos;
  const step = [5, 10, 15, 20, 30, 45, 60, 90].find((s) => s * degPx >= 70) ?? 90;
  const lonAt = (sx: number) => (proj.cx + (sx - proj.ox) / proj.k) / proj.cos;
  const latAt = (sy: number) => -(proj.cy + (sy - proj.oy) / proj.k);
  g.strokeStyle = col.hair; g.lineWidth = 1; g.globalAlpha = gridA;
  for (let lon = Math.ceil(lonAt(0) / step) * step; lon <= lonAt(w); lon += step) {
    const x = proj.ox + (lon * proj.cos - proj.cx) * proj.k;
    g.beginPath(); g.moveTo(Math.round(x) + 0.5, 0); g.lineTo(Math.round(x) + 0.5, h); g.stroke();
  }
  const latStep = [5, 10, 15, 20, 30, 45].find((s) => s * proj.k >= 54) ?? 45;
  const latLines: number[] = [];
  for (let lat = Math.ceil(latAt(h) / latStep) * latStep; lat <= latAt(0); lat += latStep) {
    if (Math.abs(lat) > 85) continue;
    latLines.push(lat);
    const y = proj.oy + (-lat - proj.cy) * proj.k;
    g.beginPath(); g.moveTo(0, Math.round(y) + 0.5); g.lineTo(w, Math.round(y) + 0.5); g.stroke();
  }
  g.globalAlpha = 1;
  if (!small && h > 200) {
    g.fillStyle = col.faint; g.font = `10px ${col.sans}`;
    for (const lat of latLines) {
      const y = proj.oy + (-lat - proj.cy) * proj.k;
      if (y < pad.t - 10 || y > h - 20) continue;
      g.fillText(lat === 0 ? "赤道" : `${lat > 0 ? "北纬" : "南纬"} ${Math.abs(lat)}°`, 6, y - 4);
    }
  }

  // ---- arcs ease toward their targets ----
  const youAt: Pt = you ? P(you) : [w * 0.3, h * 0.62];
  const unknownAt: Pt = [Math.min(w - pad.r + 30, w * 0.64), Math.min(h - pad.b, h * 0.8)];
  const rates = S.tick.byRoute, total = rs.reduce((a, r) => a + (rates[r.id] ?? 0), 0);
  const lines = exits.map(({ r, n }) => {
    const place = n?.where.place;
    const to: Pt = place ? P(place) : unknownAt;
    const st = (arcs[r.id] ??= { x: youAt[0], y: youAt[1], wd: 0, a: 0, share: r.share });
    const live = active(r) && !!n;
    st.x = lerp(st.x, to[0], 0.1); st.y = lerp(st.y, to[1], 0.1);
    st.share = lerp(st.share, total > 0 ? (rates[r.id] ?? 0) / total : r.share, 0.05);
    st.wd = lerp(st.wd, live ? 1 + st.share * 9 : on && n ? 1 : 0, 0.1);
    st.a = lerp(st.a, live ? 0.9 : on && n ? 0.3 : 0, 0.1);
    return { r, n, place, live, path: arcOf(youAt, [st.x, st.y]), to, st };
  });
  for (const { r, live, path, place, st } of lines) {
    if (st.a < 0.01) continue;
    const rc = col.v(r.color) || col.ink;
    g.beginPath(); g.moveTo(...path[0]); g.quadraticCurveTo(...path[1], ...path[2]);
    g.setLineDash(place ? [] : [3, 5]);
    g.strokeStyle = live ? rc : col.faint; g.globalAlpha = st.a;
    g.lineWidth = st.wd; g.lineCap = "round"; g.stroke(); g.globalAlpha = 1; g.setLineDash([]);
    if (live && place && Math.random() < st.share * 0.5 + 0.03) particles.push({ id: r.id, t: 0, v: 0.006 + Math.random() * 0.004, col: rc });
  }
  particles = particles.filter((p) => (p.t += p.v) < 1 && lines.some((l) => l.r.id === p.id && l.live));
  for (const p of particles) {
    const l = lines.find((x) => x.r.id === p.id);
    if (!l) continue;
    const [x, y] = qp(l.path, p.t);
    g.beginPath(); g.arc(x, y, 2.1, 0, 7); g.fillStyle = col.paper; g.fill();
    g.beginPath(); g.arc(x, y, 1.4, 0, 7); g.fillStyle = p.col; g.fill();
  }

  // ---- places ----
  const ex = exitNode(), exPlace = ex?.where.place;
  const seen = new Set<string>();
  for (const { n, place, to, st } of lines) {
    if (st.a < 0.05 || !n) continue;
    const key = place ? `${place.city}|${place.lat}|${place.lon}` : "?";
    if (seen.has(key)) continue;
    seen.add(key);
    if (!place) {
      const tt = Date.now() / 1000;
      g.beginPath(); g.arc(to[0], to[1], 9 + Math.sin(tt * 4) * 2, 0, 7); g.setLineDash([2, 3]); g.strokeStyle = col.warn; g.lineWidth = 1.5; g.stroke(); g.setLineDash([]);
      if (!small) { g.fillStyle = col.warn; g.font = `700 12px ${col.serif}`; g.fillText(n.where.source === "locating" ? "定位中…" : "位置未知", to[0] + 14, to[1] + 4); }
      continue;
    }
    const head = !!exPlace && exPlace.city === place.city && exPlace.lat === place.lat && exPlace.lon === place.lon;
    g.globalAlpha = Math.min(1, st.a * 1.4);
    g.beginPath(); g.arc(to[0], to[1], head ? 6 : 4, 0, 7); g.fillStyle = col.paper; g.fill();
    g.lineWidth = head ? 2.5 : 1.5; g.strokeStyle = col.ink; g.stroke();
    if (!small || head) {
      g.font = `${head ? "700 13px" : "500 11.5px"} ${col.serif}`;
      const tw = g.measureText(place.city).width, right = to[0] + 10 + tw > w - 8;
      g.fillStyle = col.ink; g.textAlign = right ? "right" : "left";
      g.fillText(place.city, to[0] + (right ? -10 : 10), to[1] + 4);
      g.textAlign = "left";
    }
    g.globalAlpha = 1;
  }

  // ---- you ----
  g.beginPath(); g.arc(youAt[0], youAt[1], 5, 0, 7); g.fillStyle = on ? col.ink : col.faint; g.fill();
  const ph = (Date.now() / 60) % 14;
  g.beginPath(); g.arc(youAt[0], youAt[1], 8 + ph, 0, 7); g.strokeStyle = on ? col.ink : col.faint;
  g.globalAlpha = Math.max(0, 0.3 - ph / 46); g.stroke(); g.globalAlpha = 1;
  g.font = `700 13px ${col.serif}`; g.fillStyle = on ? col.ink : col.mute;
  const youLabel = "你";
  g.fillText(youLabel, youAt[0] + 9, youAt[1] - 8);
}
