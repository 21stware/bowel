// The page's state: the latest Snapshot and Tick from Go, what the user is
// looking at, and the optimistic bits shown between a click and the next
// Snapshot.

import type { Conn, Event as GoEvent, LogLine, NodeView, ProviderView, RouteView, Snapshot, Tick } from "./mygo";
import { errorText, toast } from "./ui";

export type Page = "home" | "activity" | "providers";
export type Sample = { t: number; by: Record<string, number>; up: number };

export function emptySnapshot(): Snapshot {
  const entry = { wanted: false, status: "off" };
  return {
    state: "off", core: { state: "starting", version: "" }, mode: "rule",
    takeover: { on: false, system: { ...entry }, tun: { ...entry }, port: 0, lan: false, bypass: [] },
    routes: [], nodes: [], providers: [], ruleCount: 0, pending: [], version: "",
  };
}
export function emptyTick(): Tick {
  return { down: 0, up: 0, byRoute: {}, apps: [], events: [], active: 0, todayDown: 0, todayUp: 0, monthDown: 0 };
}

const arr = <T>(v: T[] | null | undefined): T[] => (Array.isArray(v) ? v : []);
const STATES = new Set(["ok", "degraded", "error", "off"]);

/** Go encodes nil slices and maps as null: make everything safe to read. */
export function normalizeSnapshot(s: Snapshot | null | undefined): Snapshot {
  const e = emptySnapshot();
  if (!s) return e;
  const t = s.takeover ?? e.takeover;
  const snap: Snapshot = {
    ...e, ...s,
    core: s.core ?? e.core,
    mode: s.mode || "rule",
    takeover: { ...e.takeover, ...t, system: t.system ?? e.takeover.system, tun: t.tun ?? e.takeover.tun, bypass: arr(t.bypass) },
    routes: arr(s.routes),
    nodes: arr(s.nodes).map((n) => ({ ...n, history: arr(n.history), where: n.where ?? { source: "unknown" } })),
    providers: arr(s.providers),
    pending: arr(s.pending),
  };
  if (!STATES.has(snap.state)) snap.state = snap.takeover.on ? "ok" : "off";
  return snap;
}
export function normalizeTick(t: Tick | null | undefined): Tick {
  const e = emptyTick();
  if (!t) return e;
  return { ...e, ...t, byRoute: t.byRoute ?? {}, apps: arr(t.apps), events: arr(t.events) };
}
export const normalizeConns = (cs: Conn[] | null | undefined): Conn[] => arr(cs).map((c) => ({ ...c, chain: arr(c.chain) }));
export const normalizeLogs = (ls: LogLine[] | null | undefined): LogLine[] => arr(ls);

export const S = {
  snap: emptySnapshot(),
  tick: emptyTick(),
  ready: false,
  page: "home" as Page,
  /** The expanded cell of the home board, and the route open inside "routes". */
  x: null as string | null,
  sub: null as string | null,
  nd: { region: "all", q: "" },
  ev: { paused: false, frozen: null as GoEvent[] | null },
  speedSpan: 600,
  act: {
    tab: "conn" as "conn" | "log",
    q: "", status: "active", route: "all", app: "all", net: "all",
    sort: ["rate", -1] as [string, number],
    paused: false, open: null as string | null, armed: false,
    lv: "all", lq: "", clearedSeq: 0,
  },
  prov: { open: null as string | null, armed: null as string | null, renaming: null as string | null, draft: "", paste: "", detect: "", detecting: false },
  conns: [] as Conn[],
  closing: new Set<string>(),
  logs: [] as LogLine[],
  lastSeq: 0,
  /** Download rate per route, one sample a second, the last hour. */
  hist: [] as Sample[],
  /** Optimistic pending keys (same names as Snapshot.pending). */
  local: new Set<string>(),
  /** Keys whose call returned; cleared by the next Snapshot. */
  awaiting: new Set<string>(),
  /** Optimistic values shown until the Snapshot that confirms them. */
  over: { routes: {} as Record<string, { node: string; auto: boolean }>, mode: null as string | null, modeTarget: null as string | null },
};

// ---- rendering hook (set by main.ts) ----
let renderer: (morph: boolean, prep?: () => void) => void = () => {};
export const setRenderer = (fn: (morph: boolean, prep?: () => void) => void): void => { renderer = fn; };
/**
 * Re-renders the page; with morph, shared elements fly to their new
 * places (prep runs before they are measured).
 */
export const rerender = (morph = false, prep?: () => void): void => renderer(morph, prep);

// ---- reading the state ----
export const busy = (key: string): boolean => S.snap.pending.includes(key) || S.local.has(key);
export const busyPrefix = (prefix: string): boolean => S.snap.pending.some((k) => k.startsWith(prefix)) || [...S.local].some((k) => k.startsWith(prefix));

export function routes(): RouteView[] {
  return S.snap.routes.map((r) => {
    const o = S.over.routes[r.id];
    return o ? { ...r, node: o.node, auto: o.auto } : r;
  });
}
export const route = (id: string): RouteView | undefined => routes().find((r) => r.id === id);
export const defaultRoute = (): RouteView | undefined => route("default") ?? routes().find((r) => !r.direct);
export const nodeByName = (name: string | undefined): NodeView | undefined => (name ? S.snap.nodes.find((n) => n.name === name) : undefined);
export const nodeByDisplay = (display: string): NodeView | undefined => S.snap.nodes.find((n) => n.display === display) ?? nodeByName(display);
export const provider = (id: string): ProviderView | undefined => S.snap.providers.find((p) => p.id === id);
export const mode = (): string => S.over.mode ?? S.snap.mode;
export const takeoverOn = (): boolean => S.snap.takeover.on;
/** Nodes routes can use: those of enabled providers. */
export const usableNodes = (): NodeView[] => S.snap.nodes.filter((n) => provider(n.provider)?.enabled ?? true);
export const hasNodes = (): boolean => usableNodes().length > 0;

/** Whether a route carries traffic now, given the takeover and the mode. */
export function active(r: RouteView): boolean {
  if (!takeoverOn()) return false;
  const m = mode();
  return m === "rule" || (m === "global" && r.id === defaultRoute()?.id) || (m === "direct" && r.direct);
}
/** The node the default route's traffic leaves from; null when nothing goes through a node. */
export function exitNode(): NodeView | null {
  if (!takeoverOn() || mode() === "direct") return null;
  return nodeByName(defaultRoute()?.node) ?? nodeByName(S.snap.exit?.node) ?? null;
}

// ---- history of live numbers ----
export function pushSample(t: Tick): void {
  S.hist.push({ t: Date.now(), by: { ...t.byRoute }, up: t.up });
  if (S.hist.length > 3600) S.hist.splice(0, S.hist.length - 3600);
}

// ---- actions ----
const clears = new Map<string, () => void>();

/**
 * Runs a call, showing `key` as pending (shimmer) from the click on: the
 * Snapshot's pending list takes over once Go reports it. `optimistic`
 * applies the expected result right away; it is undone if the call fails.
 */
export async function act<T>(key: string | null, call: () => Promise<T>, opts: { optimistic?: () => void; undo?: () => void; morph?: boolean; prep?: () => void } = {}): Promise<T | undefined> {
  if (key) {
    S.local.add(key);
    S.awaiting.delete(key);
    clears.set(key, opts.undo ?? (() => {}));
  }
  opts.optimistic?.();
  rerender(opts.morph ?? false, opts.prep);
  try {
    const v = await call();
    if (key) {
      S.awaiting.add(key);
      // In case no Snapshot follows (nothing changed), stop showing it.
      setTimeout(() => { if (S.awaiting.has(key)) settle(key, true); }, 2500);
    }
    return v;
  } catch (err) {
    if (key) settle(key, false);
    else rerender(true);
    toast(errorText(err));
    return undefined;
  }
}
function settle(key: string, ok: boolean): void {
  S.local.delete(key);
  S.awaiting.delete(key);
  const undo = clears.get(key);
  clears.delete(key);
  if (!ok) undo?.();
  // Optimistic values are dropped either way: the Snapshot is the truth now.
  dropOverride(key);
  rerender(true);
}
function dropOverride(key: string): void {
  if (key.startsWith("node:")) delete S.over.routes[key.slice(5)];
  if (key === "mode") { S.over.mode = null; S.over.modeTarget = null; }
}

/** A new Snapshot: the optimistic state of calls that returned is replaced by it. */
export function applySnapshot(s: Snapshot): void {
  S.snap = normalizeSnapshot(s);
  S.ready = true;
  for (const k of [...S.awaiting]) {
    if (S.snap.pending.includes(k)) continue; // Go is still applying it
    S.local.delete(k);
    S.awaiting.delete(k);
    clears.delete(k);
    dropOverride(k);
  }
  // Optimistic values whose call is still running stay.
  if (S.x && !takeoverOn()) { S.x = null; S.sub = null; }
}
