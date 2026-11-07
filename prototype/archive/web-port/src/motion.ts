// Shared elements without cross-fades.
//
// Elements that mean the same thing carry the same data-m key in every
// view. morph(update) measures the visible keyed elements, applies the
// update, measures again at the final layout (the grid's end state), then
// flies a live-text clone of each element from where it was to where it
// will be: font size, colour and letter spacing interpolate per frame, so
// text is laid out again, never a scaled bitmap. The real element stays
// hidden until its clone lands. Text that changed in place rolls instead.

import { $, reduceMotion } from "./ui";

export const EASE = "cubic-bezier(.32,.86,.22,1)";
export const DUR = 560;

interface Shot {
  el: HTMLElement;
  x: number; y: number; w: number; h: number;
  text: string; html: string; cls: string;
  fs: number; lh: number; font: string; weight: string;
  color: string; ls: string; bg: string; box: boolean; pad: string;
}

/** Whether a keyed element sits in the view its cell shows now. */
function shownView(el: Element): boolean {
  const vw = el.closest(".vw");
  if (!vw) return true;
  const v = (vw.closest(".cell") as HTMLElement | null)?.dataset.v;
  return (v === "normal" && vw.classList.contains("v-normal")) || (v === "compact" && vw.classList.contains("v-compact")) || (v === "expanded" && vw.classList.contains("v-detail"));
}

function snap(root: Element): Map<string, Shot> {
  const m = new Map<string, Shot>();
  root.querySelectorAll<HTMLElement>("[data-m]").forEach((el) => {
    const key = el.dataset.m;
    if (!key || m.has(key) || !shownView(el)) return;
    const r = el.getBoundingClientRect();
    if (!r.width || !r.height) return;
    // Skip what the cell clips away entirely.
    const cell = el.closest(".cell");
    if (cell) {
      const c = cell.getBoundingClientRect();
      if (r.top >= c.bottom - 2 || r.left >= c.right - 2 || c.height < 2) return;
    }
    const cs = getComputedStyle(el), fs = parseFloat(cs.fontSize);
    m.set(key, {
      el, x: r.left, y: r.top, w: r.width, h: r.height,
      text: (el.textContent ?? "").replace(/\s+/g, ""), html: el.innerHTML, cls: el.className,
      fs, lh: parseFloat(cs.lineHeight) / fs || 1.2, font: cs.fontFamily, weight: cs.fontWeight,
      color: cs.color, ls: cs.letterSpacing, bg: cs.backgroundColor, box: el.hasAttribute("data-mbox"), pad: cs.padding,
    });
  });
  return m;
}

function cloneAt(s: Shot, html: string): HTMLElement {
  const c = document.createElement("div");
  c.className = s.cls.replace(/\bshimmer\b/, "");
  c.innerHTML = html;
  c.querySelectorAll("[id],[data-m],[data-live]").forEach((e) => { e.removeAttribute("id"); e.removeAttribute("data-m"); e.removeAttribute("data-live"); });
  Object.assign(c.style, {
    position: "absolute", left: s.x + "px", top: s.y + "px", margin: "0", padding: s.pad,
    whiteSpace: "nowrap", fontFamily: s.font, fontWeight: s.weight, fontSize: s.fs + "px", lineHeight: String(s.lh),
    color: s.color, letterSpacing: s.ls, visibility: "visible", pointerEvents: "none",
    boxShadow: "none", border: "0", background: s.box ? s.bg : "none", animation: "none",
  });
  if (s.box) { c.style.width = s.w + "px"; c.style.height = s.h + "px"; }
  $("fx")?.append(c);
  return c;
}

const frameOf = (s: Shot): Keyframe => ({
  left: s.x + "px", top: s.y + "px", fontSize: s.fs + "px", color: s.color, letterSpacing: s.ls,
  ...(s.box ? { width: s.w + "px", height: s.h + "px", backgroundColor: s.bg } : {}),
});

function fly(o: Shot, a: Shot): void {
  const moved = Math.abs(o.x - a.x) > 1 || Math.abs(o.y - a.y) > 1 || Math.abs(o.fs - a.fs) > 0.5 || (a.box && Math.abs(o.w - a.w) > 1);
  const same = o.text === a.text;
  if (!moved && same) return;
  if (!moved) return roll(o, a);
  const c = cloneAt(a, a.html);
  a.el.style.visibility = "hidden";
  const done = () => { c.remove(); a.el.style.visibility = ""; };
  const anim = c.animate([frameOf(o), frameOf(a)], { duration: DUR, easing: EASE });
  anim.onfinish = done;
  anim.oncancel = done;
  if (!same) {
    // The old words wipe away where they were; the new ones wipe in as they travel.
    const oc = cloneAt(o, o.html);
    oc.animate([{ clipPath: "inset(0 0 0 0)", transform: "none" }, { clipPath: "inset(0 0 100% 0)", transform: "translateY(-30%)" }],
      { duration: 220, easing: "cubic-bezier(.4,0,1,1)", fill: "forwards" }).onfinish = () => oc.remove();
    c.animate([{ clipPath: "inset(100% 0 0 0)" }, { clipPath: "inset(0 0 0 0)" }], { duration: 300, delay: 80, easing: EASE, fill: "backwards" });
  }
}

/** Same place, new text: the old line slides up out of a mask, the new one slides up into it. */
function roll(o: Shot, a: Shot): void {
  const oc = cloneAt(o, o.html);
  oc.animate([{ transform: "none", clipPath: "inset(0 0 0 0)" }, { transform: "translateY(-45%)", clipPath: "inset(0 0 100% 0)" }],
    { duration: 260, easing: "cubic-bezier(.4,0,1,1)", fill: "forwards" }).onfinish = () => oc.remove();
  a.el.animate([{ transform: "translateY(45%)", clipPath: "inset(100% 0 0 0)" }, { transform: "none", clipPath: "inset(0 0 0 0)" }],
    { duration: 420, delay: 90, easing: EASE, fill: "backwards" });
}

let busyUntil = 0;
/** Whether a morph is still running (live updates wait for it). */
export const morphing = (): boolean => performance.now() < busyUntil;

/**
 * Applies update; shared elements of root fly from their old place to
 * their new one, and the board's grid (if its tracks change) animates to
 * its new tracks. prep runs before the first measurement.
 */
export function morph(root: HTMLElement | null, board: HTMLElement | null, update: () => void, prep?: () => void): void {
  if (!root || reduceMotion()) { update(); return; }
  prep?.();
  const before = snap(root);
  const oc = board?.style.gridTemplateColumns ?? "", or = board?.style.gridTemplateRows ?? "";
  if (board) board.style.transition = "none";
  update();
  if (board) {
    const nc = board.style.gridTemplateColumns, nr = board.style.gridTemplateRows;
    void board.offsetHeight;
    const after = snap(root);
    if (oc !== nc || or !== nr) {
      // Measure the end layout, then go back and let the grid animate to it.
      board.style.gridTemplateColumns = oc; board.style.gridTemplateRows = or;
      void board.offsetHeight;
      board.style.transition = "";
      board.style.gridTemplateColumns = nc; board.style.gridTemplateRows = nr;
      busyUntil = performance.now() + DUR + 40;
    } else board.style.transition = "";
    for (const [k, a] of after) { const o = before.get(k); if (o) fly(o, a); }
  } else {
    const after = snap(root);
    for (const [k, a] of after) { const o = before.get(k); if (o) fly(o, a); }
  }
}
