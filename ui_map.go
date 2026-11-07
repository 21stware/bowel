package main

// The exit map: an arc from 你 to the exit of each route, thicker for
// more traffic, with dots travelling along it. The view fits whatever
// places are known (you may be anywhere), wrapping longitudes around you
// so that arcs take the short way. Arc ends, widths and opacities ease to
// their targets, so switching a node slides its arc to the new city.

import (
	"context"
	"encoding/base64"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/oboo/moho/internal/geo"
	"github.com/oboo/moho/internal/i18n"
)

// youTrace, if set, sees where you are drawn on the map in each frame, and
// where its land's dots are laid out from (one is there); for tests.
var youTrace func(x, y, landX, landY float32)

func bg() context.Context { return context.Background() }

type mapPoint struct {
	x, y  float32 // 0–1 within the map
	label string
	known bool
}

// projection fits places into a unit square, equirectangular, with the
// longitudes unwrapped around the user's.
type projection struct {
	lon0, lon1, lat0, lat1 float64
	aspect                 float64 // of the area it is fitted to, w/h
	span                   float32 // the share of the width it may use, from the left
}

func (pr projection) spanOr1() float32 {
	if pr.span <= 0 {
		return 1
	}
	return pr.span
}

func (pr projection) at(lat, lon float64) (float32, float32) {
	x := (lon - pr.lon0) / (pr.lon1 - pr.lon0) * float64(pr.spanOr1())
	y := (pr.lat1 - lat) / (pr.lat1 - pr.lat0)
	return float32(x), float32(y)
}

func unwrap(lon, around float64) float64 {
	for lon-around > 180 {
		lon -= 360
	}
	for lon-around < -180 {
		lon += 360
	}
	return lon
}

func fit(places []*Place, around float64, aspect float64) projection {
	lon0, lon1, lat0, lat1 := 1e9, -1e9, 1e9, -1e9
	for _, pl := range places {
		lon := unwrap(pl.Lon, around)
		lon0, lon1 = math.Min(lon0, lon), math.Max(lon1, lon)
		lat0, lat1 = math.Min(lat0, pl.Lat), math.Max(lat1, pl.Lat)
	}
	if lon0 > lon1 { // nothing known: East Asia
		lon0, lon1, lat0, lat1 = 96, 150, -4, 46
	}
	// At least 40°×24°, padded, and shaped like the area.
	cx, cy := (lon0+lon1)/2, (lat0+lat1)/2
	w, h := math.Max(lon1-lon0, 40)*1.3, math.Max(lat1-lat0, 24)*1.45
	k := math.Cos(cy * math.Pi / 180) // a degree of longitude is shorter away from the equator
	if w*k/h < aspect {
		w = h * aspect / k
	} else {
		h = w * k / aspect
	}
	// Never beyond the poles: a tall, narrow area shows more ocean, not 120°N.
	h = math.Min(h, 150)
	cy = math.Max(-75+h/2, math.Min(80-h/2, cy))
	return projection{lon0: cx - w/2, lon1: cx + w/2, lat0: cy - h/2 - h*.04, lat1: cy + h/2 - h*.04, aspect: aspect}
}

// land is landGrid decoded: whether the cell at row (from 90°N) and col
// (from 180°W) is land.
var land = func() func(row, col int) bool {
	b, err := base64.StdEncoding.DecodeString(landGrid)
	if err != nil {
		panic(err)
	}
	cols, rows := 360/landStep, 180/landStep
	return func(row, col int) bool {
		if row < 0 || row >= rows {
			return false
		}
		i := row*cols + ((col%cols)+cols)%cols
		return b[i/8]>>(i%8)&1 != 0
	}
}()

// landDots calls dot at every pitch DIPs of r where there is land: the
// same density however far the map is zoomed, a sketch of where the
// places are, not a chart of the coasts. One dot is at (ax, ay), where a
// place on the map is drawn, so that they move with it as the map pans;
// as it zooms, the dots stay put around that place rather than slide past
// it, as a lattice anchored far away (at 0°, 0°) would.
func landDots(pr projection, r ui.Rect, pitch, ax0, ay0 float32, dot func(x, y float32)) {
	pxLon := float64(r.W*pr.spanOr1()) / (pr.lon1 - pr.lon0)
	pxLat := float64(r.H) / (pr.lat1 - pr.lat0)
	p := float64(pitch)
	ax, ay := float64(ax0), float64(ay0)
	x0 := ax + math.Ceil((float64(r.X)-ax)/p)*p
	y0 := ay + math.Ceil((float64(r.Y)-ay)/p)*p
	for y := y0; y < float64(r.Y+r.H); y += p {
		lat := pr.lat1 - (y-float64(r.Y))/pxLat
		if lat <= -90 || lat >= 90 {
			continue
		}
		row := int((90 - lat) / landStep)
		for x := x0; x < float64(r.X+r.W); x += p {
			lon := pr.lon0 + (x-float64(r.X))/pxLon
			if land(row, int(math.Floor((lon+180)/landStep))) {
				dot(float32(x), float32(y))
			}
		}
	}
}

// mapView is how the expanded map is panned and zoomed from where it fits
// the places: zoom times as large (0 for 1), its center moved by dlon and
// dlat degrees.
type mapView struct {
	zoom       float64
	dlon, dlat float64
	moved      bool // by the pointer since the last frame: the map follows at once
	press      *mapPress
}

type mapPress struct {
	x, y    float32 // where it went down
	dragged bool
}

// view is pr panned and zoomed by v.
func (pr projection) view(v mapView) projection {
	z := v.zoom
	if z == 0 {
		z = 1
	}
	cx, cy := (pr.lon0+pr.lon1)/2+v.dlon, (pr.lat0+pr.lat1)/2+v.dlat
	w, h := (pr.lon1-pr.lon0)/z, (pr.lat1-pr.lat0)/z
	pr.lon0, pr.lon1, pr.lat0, pr.lat1 = cx-w/2, cx+w/2, cy-h/2, cy+h/2
	return pr
}

// pan moves the view of fit, shown in a box w×h DIPs, by dx, dy DIPs.
func (v *mapView) pan(fit projection, w, h, dx, dy float32) {
	pr := fit.view(*v)
	v.dlon -= float64(dx/(w*fit.spanOr1())) * (pr.lon1 - pr.lon0)
	v.dlat += float64(dy/h) * (pr.lat1 - pr.lat0)
	v.clamp(fit)
}

// zoomAt zooms the view by s, keeping the place at (x, y) where it is.
func (v *mapView) zoomAt(fit projection, w, h, x, y float32, s float64) {
	pr := fit.view(*v)
	fx, fy := float64(x/(w*fit.spanOr1())), float64(y/h)
	lon, lat := pr.lon0+fx*(pr.lon1-pr.lon0), pr.lat1-fy*(pr.lat1-pr.lat0)
	z := v.zoom
	if z == 0 {
		z = 1
	}
	fw, fh := fit.lon1-fit.lon0, fit.lat1-fit.lat0
	// From the whole world (in height or width) to a city or so.
	z = max(fh/170, fw/360, min(z*s, fw/8))
	v.zoom = z
	w2, h2 := fw/z, fh/z
	v.dlon = lon - fx*w2 + w2/2 - (fit.lon0+fit.lon1)/2
	v.dlat = lat + fy*h2 - h2/2 - (fit.lat0+fit.lat1)/2
	v.clamp(fit)
}

// clamp keeps the view off the poles.
func (v *mapView) clamp(fit projection) {
	pr := fit.view(*v)
	h, cy := pr.lat1-pr.lat0, (pr.lat0+pr.lat1)/2
	lo, hi := -90+h/2, 90-h/2
	if lo > hi {
		lo, hi = 0, 0
	}
	v.dlat += max(lo, min(hi, cy)) - cy
}

// mapInput pans the expanded map as the pointer drags it or two fingers
// scroll it, and zooms it as they pinch or a mouse wheel turns; a click
// that doesn't drag folds it up.
func (u *UI) mapInput(e ui.Element, fit projection, box ui.Rect) {
	v := &u.home.mapView
	e.Cursor(ui.CursorGrab)
	if v.press != nil && v.press.dragged {
		e.Cursor(ui.CursorGrabbing)
	}
	e.HandleInput(func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputPointerDown:
			if ev.Button != 0 {
				return false
			}
			v.press = &mapPress{x: ev.X, y: ev.Y}
		case ui.InputPointerMove:
			pr := v.press
			if pr == nil || ev.Button != 0 {
				return false
			}
			dx, dy := ev.X-pr.x, ev.Y-pr.y
			if !pr.dragged && dx*dx+dy*dy < 16 {
				return true
			}
			pr.dragged = true
			v.pan(fit, box.W, box.H, dx, dy)
			pr.x, pr.y, v.moved = ev.X, ev.Y, true
		case ui.InputPointerUp:
			pr := v.press
			v.press = nil
			if pr == nil {
				return false
			}
			if !pr.dragged && !ev.Canceled {
				u.home.x = ""
			}
		case ui.InputScroll:
			if ev.Precise && ev.Mods&ui.Ctrl == 0 {
				v.pan(fit, box.W, box.H, -ev.DX, -ev.DY)
			} else {
				v.zoomAt(fit, box.W, box.H, ev.X, ev.Y, math.Exp(float64(-ev.DY)*.004))
			}
			v.moved = true
		case ui.InputZoom:
			v.zoomAt(fit, box.W, box.H, ev.X, ev.Y, float64(ev.Scale))
			v.moved = true
		default:
			return false
		}
		return true
	})
}

// mapPitch is how far apart the land dots are, in DIPs.
const mapPitch = 7

func (u *UI) mapCell(c *ui.Context, p *pal, v string, box ui.Rect) {
	s := u.snap
	on := s.Takeover.On
	// What to draw.
	var you *Place
	if s.You != nil {
		you = s.You.Place
	}
	type arc struct {
		route, color string
		node         NodeView
		place        *Place
		share        float64
		live         bool
	}
	var arcs []arc
	var places []*Place
	if you != nil {
		places = append(places, you)
	}
	for _, r := range s.Routes {
		if r.Direct {
			continue
		}
		n, ok := u.node(r.Node)
		if !ok {
			continue
		}
		a := arc{route: r.ID, node: n, place: n.Where.Place, share: r.Share, live: u.active(r.ID)}
		arcs = append(arcs, a)
		if a.place != nil {
			places = append(places, a.place)
		}
	}
	around := 120.0
	if you != nil {
		around = you.Lon
	}
	// Expanded, the route cards take the right 300 DIPs: fit the places left of them.
	span := float32(1)
	if v == "expanded" && box.W > 600 {
		span = (box.W - 300) / box.W
	}
	aspect := float64(box.W * span / max(box.H, 1))
	fitted := fit(places, around, aspect)
	fitted.span = span

	h := &u.home
	e := ui.Box(c.Key("map")).Fill()
	target := fitted
	if v == "expanded" {
		target = fitted.view(h.mapView)
		u.mapInput(e, fitted, box)
	} else {
		h.mapView = mapView{}
	}
	// The view eases to where it fits the places, and follows the pointer
	// at once; so do the places on it, by their degrees, so that they
	// stay on their land.
	ease := ui.Animation{Duration: 700 * time.Millisecond, Ease: ui.CubicBezier(.32, .86, .22, 1)}
	follow := func(name string, v float64) float64 {
		if h.mapView.moved {
			return float64(e.Jump(name, float32(v)))
		}
		return float64(e.AnimateTo(name, float32(v), ease))
	}
	pr := target
	pr.lon0, pr.lon1 = follow("lon0", target.lon0), follow("lon1", target.lon1)
	pr.lat0, pr.lat1 = follow("lat0", target.lat0), follow("lat1", target.lat1)
	// A place on pr, by its degrees as they ease; one not known stays
	// at (fx, fy) of the view.
	place := func(key string, pl *Place, fx, fy float64) (float32, float32) {
		lat, lon := target.lat1-fy*(target.lat1-target.lat0), target.lon0+fx/float64(span)*(target.lon1-target.lon0)
		if pl != nil {
			lat, lon = pl.Lat, unwrap(pl.Lon, around)
		}
		return pr.at(follow("lat"+key, lat), follow("lon"+key, lon))
	}
	yx, yy := place("you", you, .5, .55)
	type drawn struct {
		x, y, w, a float32
		col        ui.Color
		label      string
		known      bool
		head       bool
		route      string
		draw       float32 // how much of the arc is drawn, 0–1
		off        float32 // where its comets are in the flow's cycle
	}
	var ds []drawn
	ex, exOK := u.exitNode()
	for i, a := range arcs {
		label, known := i18n.T("定位中…", "Locating…"), false
		if a.node.Where.Source != "locating" {
			label = i18n.T("位置未知", "Unknown location")
		}
		if a.place != nil {
			label, known = geo.Display(a.place.City), true
		}
		tx, ty := place(a.route, a.place, .64, .8)
		w, al := float32(1), float32(.3)
		if a.live {
			w, al = 1+float32(a.share)*9, .9
		}
		if !on {
			w, al = 0, 0
		}
		draw := float32(0)
		if on {
			draw = 1
		}
		ds = append(ds, drawn{
			draw: e.AnimateTo("d"+a.route, draw, ui.Animation{Duration: 1100 * time.Millisecond, Ease: ui.CubicBezier(.45, 0, .2, 1)}),
			off:  float32(i) * .37,
			x:    tx, y: ty,
			w: e.AnimateTo("w"+a.route, w, ease), a: e.AnimateTo("a"+a.route, al, ease),
			col: p.routeColor(a.route), label: label, known: known, route: a.route,
			head: exOK && a.place != nil && ex.Where.Place != nil && ex.Where.Place.City == a.place.City,
		})
	}
	// Only now: the places above follow the pointer as the view does.
	h.mapView.moved = false
	// The land's dots are laid out from you; where you are not known, you
	// stay put in the view, so they go from 0°, 0° to move with the map.
	anchorX, anchorY := pr.at(0, 0)
	if you != nil {
		anchorX, anchorY = yx, yy
	}
	phase := float32(0)
	if on {
		phase = e.Loop("flow", 3400*time.Millisecond, ui.Linear)
	}
	small := box.W < 300 || box.H < 170
	grid := e.AnimateTo("grid", map[bool]float32{true: .6, false: .25}[on], ease)

	e.Draw(func(pt *ui.Painter, r ui.Rect) {
		at := func(x, y float32) (float32, float32) { return r.X + x*r.W, r.Y + y*r.H }
		// Land, as dots: one small square each, on whole pixels (a dot
		// between them blurs to nothing; a path of thousands loses some).
		const d = 2
		q := pt.Scale()
		dot := p.ink.Alpha(.4 * grid)
		lx, ly := at(anchorX, anchorY)
		landDots(pr, r, mapPitch, lx, ly, func(x, y float32) {
			x, y = round((x-d/2)*q)/q, round((y-d/2)*q)/q
			pt.Fill(ui.Rect{X: x, Y: y, W: d, H: d}, dot, d/2)
		})
		ox, oy := at(yx, yy)
		curve := func(d drawn) (float32, float32, float32, float32, float32, float32) {
			tx, ty := at(d.x, d.y)
			mx, my := (ox+tx)/2, (oy+ty)/2
			dx, dy := tx-ox, ty-oy
			l := float32(math.Hypot(float64(dx), float64(dy)))
			return ox, oy, mx - dy*.22, my - l*.28, tx, ty
		}
		for _, d := range ds {
			if d.a < .01 || d.draw < .01 {
				continue
			}
			x0, y0, cx, cy, x1, y1 := curve(d)
			var path ui.Path
			path.MoveTo(x0, y0).QuadTo(cx, cy, x1, y1).Cap(ui.RoundCap)
			if d.draw < 1 {
				path.Trim(0, d.draw)
			}
			w := max(d.w, .5)
			if !d.known {
				path.Dash(0, 3, 5)
				pt.StrokePath(&path, w, d.col.Alpha(d.a))
				continue
			}
			// Fainter where it leaves you, full at the exit.
			ang := float32(math.Atan2(float64(x1-x0), float64(y0-y1)) * 180 / math.Pi)
			pt.StrokePathGradient(&path, w, ui.LinearGradient{From: d.col.Alpha(d.a * .3), To: d.col.Alpha(d.a * .8), Angle: ang})
			if d.a < .5 || !on || d.draw < .99 {
				continue
			}
			// Comets: data on the move, two to an arc, a little quicker in
			// the middle of the way than at its ends.
			along, length := alongQuad(x0, y0, cx, cy, x1, y1)
			cw := max(w*.9, 2)
			for k := 0; k < 2; k++ {
				t := frac(phase + d.off + float32(k)/2)
				s := t - .55*float32(math.Sin(2*math.Pi*float64(t)))/(2*math.Pi)
				fade := min(1, s/.08, (1-s)/.1)
				// The trail, in pieces that fade towards its tail.
				const pieces = 6
				trail := min(.2, 64/max(length, 1))
				for j := 0; j < pieces; j++ {
					f0 := s - trail*float32(pieces-j)/pieces
					f1 := s - trail*float32(pieces-j-1)/pieces
					if f1 <= 0 {
						continue
					}
					var seg ui.Path
					seg.MoveTo(x0, y0).QuadTo(cx, cy, x1, y1).Trim(max(f0, 0), f1)
					q := float32(j+1) / pieces
					pt.StrokePath(&seg, cw*(.45+.55*q), d.col.Alpha(fade*q*q))
				}
				hx, hy := along(s)
				var glow, core, glint ui.Path
				hr := cw/2 + 1.2
				pt.FillPath(glow.Circle(hx, hy, hr+3), d.col.Alpha(.18*fade))
				pt.FillPath(core.Circle(hx, hy, hr), d.col.Alpha(fade))
				pt.FillPath(glint.Circle(hx, hy, max(hr*.45, .9)), ui.RGBA(255, 255, 255, .95*fade))
				// It lands: a ripple at the exit, as the next one sets off.
				if t < .3 {
					u := t / .3
					var ring ui.Path
					pt.StrokePath(ring.Circle(x1, y1, 5+11*u), 1.2*(1-u)+.3, d.col.Alpha(.45*(1-u)*(1-u)))
				}
			}
		}
		seen := map[string]bool{}
		for _, d := range ds {
			if d.a < .05 || seen[d.label] {
				continue
			}
			seen[d.label] = true
			tx, ty := at(d.x, d.y)
			if !d.known {
				var ring ui.Path
				ring.Circle(tx, ty, 9).Dash(0, 2, 3)
				pt.StrokePath(&ring, 1.5, p.warn)
				if !small {
					pt.RichText(tx+14, ty-8, 0, ui.Span{Text: d.label, Font: sans, Size: 12, Weight: 700, Color: p.warn})
				}
				continue
			}
			rad := float32(4)
			if d.head {
				rad = 6
			}
			var dot ui.Path
			pt.FillPath(dot.Circle(tx, ty, rad), p.paper)
			var ring ui.Path
			w := float32(1.5)
			if d.head {
				w = 2.5
			}
			pt.StrokePath(ring.Circle(tx, ty, rad), w, p.ink)
			if !small || d.head {
				size, weight := float32(11.5), 500
				if d.head {
					size, weight = 13, 700
				}
				span := ui.Span{Text: d.label, Font: sans, Size: size, Weight: weight, Color: p.ink}
				lw, lh := pt.MeasureText(0, span)
				lx := tx + 10
				if lx+lw > r.X+r.W-6 {
					lx = tx - 10 - lw
				}
				pt.RichText(lx, ty-lh/2, 0, span)
			}
		}
		// You.
		if youTrace != nil {
			youTrace(ox, oy, lx, ly)
		}
		var you ui.Path
		col := p.ink
		if !on {
			col = p.faint
		}
		pt.FillPath(you.Circle(ox, oy, 5), col)
		pulse := float32(math.Mod(float64(time.Now().UnixMilli())/60, 14))
		var ring ui.Path
		pt.StrokePath(ring.Circle(ox, oy, 8+pulse), 1, col.Alpha(max(0, .3-pulse/46)))
		pt.RichText(ox+9, oy-22, 0, ui.Span{Text: i18n.T("你", "You"), Font: sans, Size: 13, Weight: 700, Color: col})
	})
	if !small {
		c.After(50 * time.Millisecond) // the pulse around 你
	}

	// Caption and legend.
	live := 0
	for _, a := range arcs {
		if a.live {
			live++
		}
	}
	caption := i18n.T("开始接管后，这里会画出流量去往的出口。", "Once takeover starts, the exits your traffic goes to are drawn here.")
	if on && len(arcs) == 0 {
		caption = i18n.T("还没有节点：添加提供商后，这里会画出流量去往的出口。", "No nodes yet: add a provider and the exits your traffic goes to are drawn here.")
	} else if on {
		caption = strings.ReplaceAll(i18n.T("从你出发的 {n} 条线路　线越粗流量越大，光点是正在传输的数据。", "{n} routes from you · thicker lines carry more traffic, dots are data on the move."), "{n}", itoa(live))
		if v == "expanded" {
			caption = i18n.T("出口地图　拖动平移 · 捏合缩放 · 轻点收起", "Exit map · drag to pan · pinch to zoom · tap to close")
		}
	}
	if !small {
		ui.Text(c, caption).FontSize(11).TextColor(p.mute).Absolute().Left(20).Top(16).MaxWidth(box.W - 40)
	}
	if v == "expanded" {
		ui.Column(c).Absolute().Right(18).Top(50).Width(270).Gap(8).Children(func() {
			for _, a := range arcs {
				rid := a.route
				b := ui.ButtonBase(c.Key(rid)).Row().Gap(10).Padding(8, 10).Radius(8).Background(p.sheet.Alpha(.88)).Border(1, p.hair).AlignItems(ui.Center).
					OnClick(func() { u.home.x, u.home.sub = "routes", rid })
				if !a.live {
					b.Opacity(.4)
				}
				b.Children(func() {
					swatch(c, p.routeColor(rid), 8)
					ui.Column(c).Grow(1).Gap(2).Children(func() {
						flagged(c, p, a.node, 14, func() {
							ui.Text(c, a.node.Display).FontSize(14).FontWeight(600).TextColor(p.ink).NoWrap()
						})
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							ui.Text(c, u.route(rid).Name+" · "+placeName(a.node)).FontSize(11.5).TextColor(p.mute)
							locTag(c, p, a.node.Where.Source)
						})
					})
					ui.Text(c, fmtDelay(a.node)).FontSize(12).FontFeatures("tnum").TextColor(delayColor(p, a.node.Delay, a.node.Tested))
				})
			}
		})
	} else if !small && on {
		ui.Column(c).Absolute().Right(20).Bottom(14).Gap(3).AlignItems(ui.End).Children(func() {
			for _, a := range arcs {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Opacity(map[bool]float32{true: 1, false: .35}[a.live]).Children(func() {
					ui.Text(c, u.route(a.route).Name).FontSize(11).TextColor(p.ink2)
					ui.Box(c).Size(14, 2).Radius(1).Background(p.routeColor(a.route))
				})
			}
		})
	}
}

// alongQuad returns where along the quadratic curve (x0, y0) (cx, cy)
// (x1, y1) a fraction of its length is, as Path.Trim measures it, and
// that length.
func alongQuad(x0, y0, cx, cy, x1, y1 float32) (func(f float32) (float32, float32), float32) {
	const n = 48
	var xs, ys, ls [n + 1]float32
	for i := 0; i <= n; i++ {
		t := float32(i) / n
		it := 1 - t
		xs[i] = it*it*x0 + 2*it*t*cx + t*t*x1
		ys[i] = it*it*y0 + 2*it*t*cy + t*t*y1
		if i > 0 {
			ls[i] = ls[i-1] + float32(math.Hypot(float64(xs[i]-xs[i-1]), float64(ys[i]-ys[i-1])))
		}
	}
	return func(f float32) (float32, float32) {
		at := max(0, min(f, 1)) * ls[n]
		i := 1
		for i < n && ls[i] < at {
			i++
		}
		seg := ls[i] - ls[i-1]
		k := float32(0)
		if seg > 0 {
			k = (at - ls[i-1]) / seg
		}
		return xs[i-1] + (xs[i]-xs[i-1])*k, ys[i-1] + (ys[i]-ys[i-1])*k
	}, ls[n]
}

func frac(v float32) float32 { return v - float32(math.Floor(float64(v))) }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
