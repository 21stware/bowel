// A pretend Go side, for working on the page in a plain browser
// (`bunx vite`). Loaded by main.ts only when isMyGo() is false. It keeps a
// Snapshot, sends it on every change, sends a Tick every second, and makes
// every action take a moment (with its key in Snapshot.pending) so loading
// states can be seen.
//
// URL switches: ?you=us (你 in San Francisco), ?state=off|empty|degraded|error.
// window.__mock exposes the state and a few levers for poking at it.

import type { GoEvent } from "mygo-runtime";
import type { Api } from "./api";
import type { AppRate, Conn, Event as GoEvt, LogLine, NodeView, Place, ProviderView, RouteView, Snapshot, Tick } from "./mygo";

class MockCallError extends Error {
  override readonly name = "CallError" as const;
  constructor(readonly method: string, message: string) { super(message); }
}

// ---------------------------------------------------------------- places

const P = (country: string, city: string, flag: string, lat: number, lon: number): Place => ({ country, city, flag, lat, lon });
const PL = {
  SH: P("CN", "上海", "🇨🇳", 31.23, 121.47), SF: P("US", "旧金山", "🇺🇸", 37.77, -122.42),
  HK: P("HK", "香港", "🇭🇰", 22.32, 114.17), TYO: P("JP", "东京", "🇯🇵", 35.68, 139.69), OSA: P("JP", "大阪", "🇯🇵", 34.69, 135.5),
  JP: P("JP", "日本", "🇯🇵", 35.68, 139.69), SG: P("SG", "新加坡", "🇸🇬", 1.35, 103.82), TPE: P("TW", "台北", "🇹🇼", 25.03, 121.56),
  SEL: P("KR", "首尔", "🇰🇷", 37.57, 126.98), LA: P("US", "洛杉矶", "🇺🇸", 34.05, -118.24), SJC: P("US", "圣何塞", "🇺🇸", 37.34, -121.89),
  US: P("US", "美国", "🇺🇸", 34.05, -118.24), FRA: P("DE", "法兰克福", "🇩🇪", 50.11, 8.68), LON: P("GB", "伦敦", "🇬🇧", 51.51, -0.13),
};

const q = new URLSearchParams(location.search);
const scenario = q.get("state") ?? "";
const now = () => new Date().toISOString();
const minsAgo = (m: number) => new Date(Date.now() - m * 60000).toISOString();
const rnd = (a: number, b: number) => a + Math.random() * (b - a);
const pick = <T>(xs: readonly T[]): T => xs[Math.floor(Math.random() * xs.length)]!;

// ---------------------------------------------------------------- nodes and providers

/** Where a node really is, for nodes whose place is not known yet. */
const realPlace = new Map<string, Place>();

function mkNode(provider: string, display: string, type: string, base: number, place: Place | null, source: string, udp = true): NodeView {
  const n: NodeView = {
    name: `${provider}/${display}`, display, provider, type, udp, delay: 0, tested: base !== 0, history: [],
    where: place ? { place, source } : { source },
  };
  setDelay(n, base);
  return n;
}
function setDelay(n: NodeView, base: number): void {
  if (base < 0) { n.delay = 0; n.tested = true; n.history = Array.from({ length: 12 }, () => (Math.random() < 0.7 ? 0 : Math.round(rnd(300, 600)))); return; }
  if (base === 0) { n.delay = 0; n.tested = false; n.history = []; return; }
  n.history = Array.from({ length: 12 }, () => (Math.random() < 0.04 ? 0 : Math.round(base * rnd(0.8, 1.25))));
  n.delay = Math.round(base * rnd(0.9, 1.1));
  n.tested = true;
}
const baseDelay = new Map<string, number>();
function addNode(nodes: NodeView[], n: NodeView, base: number): void { nodes.push(n); baseDelay.set(n.name, base); }

function initialNodes(): NodeView[] {
  const ns: NodeView[] = [];
  const A = (d: string, t: string, b: number, p: Place | null, s = "name", udp = true) => addNode(ns, mkNode("a", d, t, b, p, s, udp), b);
  A("香港 03 IPLC", "Shadowsocks", 41, PL.HK, "probe");
  A("香港 02", "Trojan", 64, PL.HK, "probe");
  A("香港 01", "Trojan", 72, PL.HK);
  A("东京 01", "VMess", 118, PL.TYO, "probe");
  A("东京 02", "VMess", -1, PL.TYO);
  A("新加坡 01", "Shadowsocks", 143, PL.SG, "probe");
  A("台北 01 Hinet", "Trojan", 96, PL.TPE);
  A("首尔 01", "VLESS", 104, PL.SEL);
  A("洛杉矶 01", "Shadowsocks", 172, PL.LA);
  A("圣何塞 01 原生", "Trojan", 0, PL.SJC, "name", false);
  const B = (d: string, t: string, b: number, p: Place | null, s = "name") => addNode(ns, mkNode("b", d, t, b, p, s), b);
  B("🇭🇰 HK-01", "Trojan", 70, PL.HK);
  B("大阪 01", "Trojan", 131, PL.OSA);
  B("日本 02 | 0.5x", "VMess", 0, PL.JP);
  B("SG-02", "Trojan", 150, PL.SG);
  B("美国 01", "VMess", 190, PL.US);
  B("英国 01", "VMess", 228, PL.LON);
  addNode(ns, mkNode("c", "洛杉矶 VPS", "Hysteria2", 168, PL.LA, "probe"), 168);
  const relay = mkNode("d", "Relay-7 专线", "VLESS", 212, null, "unknown");
  realPlace.set(relay.name, PL.FRA);
  addNode(ns, relay, 212);
  for (const d of ["办公室网关", "测试机 01", "测试机 02"]) addNode(ns, mkNode("e", d, "Socks5", 0, null, "unknown"), 30);
  return ns;
}

function initialProviders(): ProviderView[] {
  const day = 86400, nowS = Math.floor(Date.now() / 1000);
  return [
    { id: "a", name: "机场 A", kind: "subscription", source: "https://sub.example-a.com/api/v1/client/••••", enabled: true, autoUpdate: 12, updatedAt: minsAgo(10), nodes: 10, usage: { upload: 2.1e9, download: 31.2e9, total: 100 * 1024 ** 3, expire: nowS + 55 * day } },
    { id: "b", name: "机场 B", kind: "subscription", source: "https://b.example.net/link/••••", enabled: true, autoUpdate: 0, updatedAt: minsAgo(3 * 1440), nodes: 6, usage: { upload: 4e9, download: 166e9, total: 200 * 1024 ** 3, expire: nowS + 28 * day } },
    { id: "c", name: "自建 · 洛杉矶 VPS", kind: "link", source: "hysteria2://••••@la.my-vps.net:443", enabled: true, autoUpdate: 0, updatedAt: minsAgo(60 * 24 * 12), nodes: 1 },
    { id: "d", name: "朋友分享的专线", kind: "link", source: "vless://••••@relay7.example.org:443", enabled: true, autoUpdate: 0, updatedAt: minsAgo(60 * 30), nodes: 1 },
    { id: "e", name: "公司内网.yaml", kind: "file", source: "~/Documents/office.yaml", enabled: false, autoUpdate: 0, updatedAt: minsAgo(60 * 24 * 40), nodes: 3 },
  ];
}

function initialRoutes(empty: boolean): RouteView[] {
  const r = (id: string, name: string, summary: string, rules: number, share: number, node: string, auto: boolean, direct = false): RouteView =>
    ({ id, name, color: `--r-${id}`, summary, rules, direct, auto, node: empty || direct ? "" : node, share });
  return [
    r("default", "默认", "没命中其他规则的流量", 1, 0.31, "a/香港 03 IPLC", true),
    r("ai", "AI 服务", "OpenAI · Claude · Gemini · Perplexity 等", 26, 0.08, "c/洛杉矶 VPS", false),
    r("media", "流媒体", "YouTube · Netflix · Spotify · Disney+ 等", 10, 0.52, "a/新加坡 01", false),
    r("dev", "开发", "GitHub · npm · Docker · PyPI · Go 等", 24, 0.04, "a/东京 01", true),
    r("direct", "直连", "国内网站 · 局域网", 4, 0.05, "", false, true),
  ];
}

// ---------------------------------------------------------------- the state

const empty = scenario === "empty";
const you = q.get("you") === "us" ? PL.SF : PL.SH;
const M = {
  snap: {
    state: "ok", core: { state: "running", version: "v1.19.12" }, mode: "rule",
    takeover: {
      on: scenario !== "off",
      system: { wanted: true, status: scenario === "off" ? "off" : scenario === "error" ? "hijacked" : "on", detail: scenario === "error" ? "现在指向 127.0.0.1:1087" : undefined },
      tun: { wanted: false, status: "install" },
      port: 7899, lan: false, lanAddr: "192.168.1.23:7899", bypass: ["localhost", "127.0.0.1", "*.local", "192.168.0.0/16", "10.0.0.0/8"],
    },
    routes: initialRoutes(empty), nodes: empty ? [] : initialNodes(), providers: empty ? [] : initialProviders(),
    you: { place: you, source: "probe", ip: you === PL.SF ? "73.15.x.x" : "101.88.x.x" },
    ruleCount: 66, pending: [], version: "0.1.0-mock",
  } as Snapshot,
  todayDown: 2.31e9, todayUp: 1.86e8, monthDown: 48.6e9, t: 0,
  conns: [] as Conn[], logs: [] as LogLine[], seq: 0, events: [] as GoEvt[],
};
if (scenario === "degraded" && !empty) { const b = M.snap.providers[1]!; b.usage!.download = 188 * 1024 ** 3; }

const snapListeners = new Set<(s: Snapshot) => void>();
const tickListeners = new Set<(t: Tick) => void>();

function derive(): void {
  const s = M.snap, tk = s.takeover;
  for (const p of s.providers) {
    p.nodes = s.nodes.filter((n) => n.provider === p.id).length;
    const u = p.usage, used = u ? (u.upload + u.download) / (u.total || 1) : 0;
    p.warn = u && u.total && used > 0.9 ? "quota" : u && u.expire && u.expire - Date.now() / 1000 < 7 * 86400 ? "expire" : undefined;
  }
  const def = s.routes.find((r) => r.id === "default");
  const exitNode = s.nodes.find((n) => n.name === def?.node);
  s.exit = exitNode ? { node: exitNode.name, where: exitNode.where } : undefined;
  tk.on = tk.on && (tk.system.wanted || tk.tun.wanted);
  s.issue = undefined;
  if (!tk.on) s.state = "off";
  else if (tk.system.status === "hijacked") {
    s.state = "error";
    s.issue = { level: "error", text: "系统代理被其他程序改掉了，网页流量现在没有经过 Moho。", action: "retake", label: "重新接管" };
  } else {
    const warn = s.providers.find((p) => p.warn);
    s.state = warn ? "degraded" : "ok";
    if (warn) s.issue = { level: "warn", text: `「${warn.name}」${warn.warn === "quota" ? "流量快用完了" : "快到期了"}。`, action: "providers", label: "查看" };
  }
}
function emit(): void {
  derive();
  const copy = structuredClone(M.snap);
  setTimeout(() => snapListeners.forEach((f) => f(copy)), 0);
}
const pend = (k: string) => { if (!M.snap.pending.includes(k)) M.snap.pending.push(k); emit(); };
const unpend = (k: string) => { M.snap.pending = M.snap.pending.filter((x) => x !== k); };
const wait = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

/** Shows key as pending for ms, then applies fn and sends the new Snapshot. */
async function apply<T>(key: string, ms: number, fn: () => T): Promise<T> {
  pend(key);
  await wait(ms);
  try { return fn(); } finally { unpend(key); emit(); }
}
const fail = (method: string, msg: string): never => { throw new MockCallError(method, msg); };

const route = (id: string) => M.snap.routes.find((r) => r.id === id) ?? fail("Routes", `没有路由「${id}」`);
const fastest = () => M.snap.nodes.filter((n) => n.tested && n.delay > 0 && M.snap.providers.find((p) => p.id === n.provider)?.enabled).sort((a, b) => a.delay - b.delay)[0];
function locateLater(n: NodeView, ms = 2200): void {
  if (n.where.source === "probe") return;
  n.where = { ...n.where, source: "locating" };
  emit();
  setTimeout(() => {
    const p = realPlace.get(n.name) ?? n.where.place ?? pick([PL.HK, PL.TYO, PL.SG, PL.FRA]);
    n.where = { place: p, source: "probe", ip: `${Math.floor(rnd(20, 220))}.${Math.floor(rnd(1, 250))}.x.x` };
    log("info", `实测出口：${n.display} → ${p.country} · ${p.city}`);
    emit();
  }, ms);
}

// ---------------------------------------------------------------- live numbers

const APPS: [string, string, string, number][] = [
  ["Arc", "youtube.com", "media", 1.21e6], ["Claude", "api.anthropic.com", "ai", 8.4e4], ["Docker", "registry-1.docker.io", "dev", 2.1e5],
  ["Slack", "wss-primary.slack.com", "default", 1.2e4], ["WeChat", "weixin.qq.com", "direct", 9e3], ["git", "github.com", "dev", 4e3],
  ["Spotify", "audio-ak.spotifycdn.com", "media", 4.1e4], ["Safari", "www.apple.com", "direct", 6e3],
];
const CONN_T: [string, string, string, number, string, string, string, string, string][] = [
  ["Arc", "rr3---sn-i3b7kn.googlevideo.com", "142.250.66.78", 443, "tcp", "GEOSITE", "youtube", "media", "/Applications/Arc.app/Contents/MacOS/Arc"],
  ["Arc", "www.youtube.com", "142.250.66.46", 443, "tcp", "GEOSITE", "youtube", "media", "/Applications/Arc.app/Contents/MacOS/Arc"],
  ["Arc", "x.com", "104.244.42.1", 443, "tcp", "MATCH", "", "default", "/Applications/Arc.app/Contents/MacOS/Arc"],
  ["Arc", "chat.openai.com", "104.18.32.47", 443, "udp", "DOMAIN-SUFFIX", "openai.com", "ai", "/Applications/Arc.app/Contents/MacOS/Arc"],
  ["Arc", "fonts.gstatic.com", "142.250.66.67", 443, "udp", "MATCH", "", "default", "/Applications/Arc.app/Contents/MacOS/Arc"],
  ["Docker", "registry-1.docker.io", "54.227.20.253", 443, "tcp", "DOMAIN-SUFFIX", "docker.io", "dev", "/Applications/Docker.app/Contents/MacOS/com.docker.backend"],
  ["Docker", "production.cloudflare.docker.com", "104.16.98.215", 443, "tcp", "DOMAIN-SUFFIX", "docker.com", "dev", "/Applications/Docker.app/Contents/MacOS/com.docker.backend"],
  ["Claude", "api.anthropic.com", "160.79.104.10", 443, "tcp", "DOMAIN-SUFFIX", "anthropic.com", "ai", "/Applications/Claude.app/Contents/MacOS/Claude"],
  ["Spotify", "audio-ak.spotifycdn.com", "23.45.12.9", 443, "tcp", "GEOSITE", "spotify", "media", "/Applications/Spotify.app/Contents/MacOS/Spotify"],
  ["Slack", "wss-primary.slack.com", "3.120.44.8", 443, "tcp", "MATCH", "", "default", "/Applications/Slack.app/Contents/MacOS/Slack"],
  ["WeChat", "szshort.weixin.qq.com", "183.3.224.139", 443, "tcp", "GEOSITE", "cn", "direct", "/Applications/WeChat.app/Contents/MacOS/WeChat"],
  ["git", "github.com", "20.205.243.166", 22, "tcp", "DOMAIN-SUFFIX", "github.com", "dev", "/usr/bin/ssh"],
  ["mDNSResponder", "dns.google", "8.8.8.8", 853, "tcp", "MATCH", "", "default", "/usr/sbin/mDNSResponder"],
  ["Safari", "www.apple.com.cn", "17.253.85.204", 443, "tcp", "GEOSITE", "cn", "direct", "/Applications/Safari.app/Contents/MacOS/Safari"],
  ["node", "registry.npmjs.org", "104.16.24.34", 443, "tcp", "DOMAIN-SUFFIX", "npmjs.org", "dev", "/opt/homebrew/bin/node"],
];
let connId = 0;
function newConn(t = pick(CONN_T), status = "active"): Conn {
  const [app, host, ip, port, network, rule, payload, rid, process] = t;
  const r = M.snap.routes.find((x) => x.id === rid);
  const n = M.snap.nodes.find((x) => x.name === r?.node);
  const failed = status === "failed";
  return {
    id: `c${++connId}`, app, process, host, ip, port, network, rule, payload, route: rid,
    node: r?.direct ? "" : n?.display ?? "", chain: r?.direct ? ["DIRECT"] : [r?.name ?? "", n?.display ?? ""].filter(Boolean),
    down: failed ? 0 : Math.round(rnd(1e4, 4e8)), up: failed ? 0 : Math.round(rnd(1e3, 2e6)), downRate: 0, upRate: 0,
    start: new Date(Date.now() - rnd(1, 3600) * 1000).toISOString(), end: status === "active" ? undefined : minsAgo(rnd(0, 5)),
    status, error: failed ? `dial tcp ${ip}:${port}: i/o timeout (5000 ms)` : undefined,
  };
}
function initConns(): void {
  M.conns = CONN_T.map((t) => newConn(t));
  for (let i = 0; i < 26; i++) M.conns.push(newConn());
  M.conns.push(newConn(CONN_T[2], "failed"), newConn(CONN_T[11], "closed"), newConn(CONN_T[4], "closed"));
}
function log(level: string, text: string): void {
  M.logs.push({ seq: ++M.seq, time: now(), level, text });
  if (M.logs.length > 1000) M.logs.splice(0, M.logs.length - 1000);
}
function seedLogs(): void {
  log("info", "mihomo v1.19.12 已启动，监听 127.0.0.1:7899");
  log("info", "[TCP] Arc → rr3---sn-i3b7kn.googlevideo.com:443 命中 GEOSITE(youtube) → 流媒体[新加坡 01]");
  log("info", "[TCP] Claude → api.anthropic.com:443 命中 DOMAIN-SUFFIX(anthropic.com) → AI 服务[洛杉矶 VPS]");
  log("warning", "东京 02 健康检查超时（连续第 3 次）");
  log("error", "[TCP] Arc → x.com:443 拨号失败：东京 02 i/o timeout");
  log("info", "默认 自动切换：东京 02 → 香港 03 IPLC（41 ms）");
  log("debug", "[DNS] x.com → 104.244.42.1（fake-ip 198.18.0.42）");
  log("info", "Provider「机场 A」已更新：10 个节点");
}
function isActive(r: RouteView): boolean {
  const s = M.snap;
  if (!s.takeover.on) return false;
  return s.mode === "rule" || (s.mode === "global" && r.id === "default") || (s.mode === "direct" && r.direct);
}
function mkTick(): Tick {
  const s = M.snap, i = ++M.t;
  const w = (p: number, a: number) => Math.max(0.05, 1 + Math.sin(i / p) * a + (Math.random() - 0.5) * 0.2);
  const base: Record<string, number> = { default: 4e5 * w(9, 0.5), ai: 1e5 * w(4, 0.8), media: 7.2e5 * w(13, 0.45), dev: 6e4 * w(3, 1), direct: 7e4 * w(6, 0.5) };
  const byRoute: Record<string, number> = {};
  const routed = s.routes.filter((r) => (r.direct || r.node) && isActive(r));
  if (s.mode !== "rule" && s.takeover.on) {
    // Everything goes one way.
    const total = Object.values(base).reduce((a, v) => a + v, 0), only = s.mode === "direct" ? s.routes.find((r) => r.direct) : s.routes.find((r) => r.id === "default");
    if (only && (only.direct || only.node)) byRoute[only.id] = Math.round(total);
  } else for (const r of routed) byRoute[r.id] = Math.round(base[r.id] ?? 2e4 * w(5, 0.5));
  const down = Object.values(byRoute).reduce((a, v) => a + v, 0), up = s.takeover.on ? Math.round(6e4 * w(5, 0.5)) : 0;
  M.todayDown += down; M.todayUp += up; M.monthDown += down;
  const apps: AppRate[] = s.takeover.on && down > 0 ? APPS.map(([app, host, rid, v]) => ({
    app, host, route: s.mode === "rule" ? rid : s.mode === "direct" ? "direct" : "default",
    down: Math.round(v * rnd(0.6, 1.4)), up: Math.round(v * rnd(0.02, 0.1)), conns: M.conns.filter((c) => c.app === app && c.status === "active").length || 1,
  })).filter((a) => byRoute[a.route] !== undefined).sort((a, b) => b.down - a.down) : [];
  if (s.takeover.on && Math.random() < 0.8) {
    const t = pick(CONN_T), rid = s.mode === "rule" ? t[7] : s.mode === "direct" ? "direct" : "default";
    const r = s.routes.find((x) => x.id === rid), n = s.nodes.find((x) => x.name === r?.node);
    const failed = Math.random() < 0.04;
    M.events.unshift({ time: now(), text: `${t[0]} → ${t[1].replace(/^(rr\d---sn-[^.]+\.|www\.|audio-ak\.|wss-primary\.)/, "")}${t[3] !== 443 ? ":" + t[3] : ""}`, route: rid, node: r?.direct ? "" : n?.display ?? "", failed });
    M.events = M.events.slice(0, 60);
    if (Math.random() < 0.3) log(failed ? "error" : "info", `[${t[4].toUpperCase()}] ${t[0]} → ${t[1]}:${t[3]} ${failed ? "拨号失败：i/o timeout" : `命中 ${t[5]}${t[6] ? `(${t[6]})` : ""} → ${r?.name ?? "直连"}${n ? `[${n.display}]` : ""}`}`);
    else if (Math.random() < 0.15) log("debug", `[DNS] ${t[1]} → ${t[2]}`);
  }
  return { down, up, byRoute, apps, events: s.takeover.on ? M.events.slice(0, 30) : [], active: M.conns.filter((c) => c.status === "active").length + 31, todayDown: M.todayDown, todayUp: M.todayUp, monthDown: M.monthDown };
}
function stepConns(): void {
  for (const c of M.conns) {
    if (c.status !== "active") continue;
    const r = M.snap.routes.find((x) => x.id === c.route);
    const on = r ? isActive(r) : false;
    c.downRate = on ? Math.round(Math.max(0, (c.downRate || rnd(1e3, 4e5)) * rnd(0.7, 1.3))) : 0;
    c.upRate = on ? Math.round(c.downRate * rnd(0.01, 0.08)) : 0;
    c.down += c.downRate; c.up += c.upRate;
    if (Math.random() < 0.01) { c.status = "closed"; c.end = now(); c.downRate = c.upRate = 0; }
  }
  if (Math.random() < 0.3) M.conns.unshift(newConn(pick(CONN_T), Math.random() < 0.08 ? "failed" : "active"));
  const keep = M.conns.filter((c) => c.status === "active");
  const done = M.conns.filter((c) => c.status !== "active").slice(0, 40);
  M.conns = [...keep, ...done].sort((a, b) => b.start.localeCompare(a.start));
}

// ---------------------------------------------------------------- services

function event<T>(name: string, set: Set<(v: T) => void>): GoEvent<T> {
  return {
    name,
    on: (fn) => { set.add(fn); return () => set.delete(fn); },
    once: (fn) => { const g = (v: T) => { set.delete(g); fn(v); }; set.add(g); return () => set.delete(g); },
  };
}

const DETECT = (input: string): string => {
  const v = input.trim();
  if (/^https?:\/\//i.test(v)) return "subscription";
  if (/^[a-z0-9+-]+:\/\//i.test(v)) return "link";
  if (/^\s*(proxies|proxy-providers|mixed-port|port)\s*:/m.test(v)) return "file";
  return fail("Providers.Detect", "不是订阅链接、节点链接或 Clash 配置");
};
let provSeq = 0;
function addProvider(kind: string, input: string): ProviderView {
  const s = M.snap, id = `n${++provSeq}`;
  let p: ProviderView;
  if (kind === "subscription") {
    const host = input.replace(/^https?:\/\//, "").split("/")[0] ?? "订阅";
    p = { id, name: host.replace(/^(sub|api|www)\./, ""), kind, source: input.replace(/(token|key)=[^&]+/gi, "$1=••••").slice(0, 60), enabled: true, autoUpdate: 12, updatedAt: now(), nodes: 0,
      usage: { upload: 1e8, download: rnd(1, 20) * 1e9, total: 200 * 1024 ** 3, expire: Math.floor(Date.now() / 1000) + 90 * 86400 } };
    const pool: [string, Place, number][] = [["香港 A1", PL.HK, 48], ["香港 A2", PL.HK, 61], ["东京 A1", PL.TYO, 92], ["新加坡 A1", PL.SG, 120], ["洛杉矶 A1", PL.LA, 160], ["法兰克福 A1", PL.FRA, 240], ["首尔 A1", PL.SEL, 88], ["台北 A1", PL.TPE, 77]];
    for (const [d, pl, b] of pool) addNode(s.nodes, mkNode(id, d, pick(["Trojan", "VMess", "Shadowsocks"]), b, pl, "name"), b);
  } else if (kind === "link") {
    const links = input.split(/\s+/).filter(Boolean);
    const scheme = (links[0] ?? "").split("://")[0] ?? "link";
    const host = (links[0] ?? "").split("@")[1]?.split(/[:/?#]/)[0] ?? "node";
    p = { id, name: links.length > 1 ? `${links.length} 个节点` : host, kind, source: `${scheme}://••••@${host}`, enabled: true, autoUpdate: 0, updatedAt: now(), nodes: 0 };
    links.forEach((l, i) => {
      const nm = decodeURIComponent(l.split("#")[1] ?? "") || (links.length > 1 ? `${host} ${i + 1}` : host);
      const n = mkNode(id, nm, scheme === "hy2" ? "Hysteria2" : scheme[0]!.toUpperCase() + scheme.slice(1), 130, null, "unknown");
      realPlace.set(n.name, pick([PL.LA, PL.TYO, PL.FRA, PL.SG]));
      addNode(s.nodes, n, 130);
      setTimeout(() => locateLater(n, 1800), 400);
    });
  } else {
    p = { id, name: "粘贴的配置", kind: "file", source: "（粘贴的内容）", enabled: true, autoUpdate: 0, updatedAt: now(), nodes: 0 };
    addNode(s.nodes, mkNode(id, "配置节点 01", "Shadowsocks", 110, PL.HK, "name"), 110);
  }
  s.providers.push(p);
  // A fresh install: routes start using the new nodes.
  for (const r of s.routes) if (!r.direct && !r.node) { r.node = fastest()?.name ?? ""; r.auto = true; }
  log("info", `Provider「${p.name}」已添加：${s.nodes.filter((n) => n.provider === id).length} 个节点`);
  derive();
  return structuredClone(s.providers.find((x) => x.id === id)!);
}

export function createMock(): Api {
  derive();
  initConns();
  seedLogs();
  // Live numbers every second.
  setInterval(() => {
    stepConns();
    const t = mkTick();
    tickListeners.forEach((f) => f(t));
  }, 1000);
  // Now and then a test result changes, as health checks do.
  setInterval(() => {
    const s = M.snap;
    if (!s.takeover.on || !s.nodes.length) return;
    const n = pick(s.nodes), b = baseDelay.get(n.name) ?? 100;
    setDelay(n, b);
    emit();
  }, 7000);

  const Moho: Api["Moho"] = {
    snapshot: async () => { derive(); return structuredClone(M.snap); },
    tick: async () => mkTick(),
  };
  const Control: Api["Control"] = {
    retake: () => apply("retake", 1100, () => { M.snap.takeover.system.status = "on"; M.snap.takeover.system.detail = undefined; log("info", "已重新接管系统代理"); }),
    setLan: (on) => apply("lan", 300, () => { M.snap.takeover.lan = on; }),
    setMode: (mode) => {
      if (!["rule", "global", "direct"].includes(mode)) return Promise.reject(new MockCallError("Control.SetMode", `没有「${mode}」模式`));
      M.snap.mode = mode;
      return apply("mode", 500, () => { log("info", `模式：${mode}`); });
    },
    setSystemProxy: (on) => apply("entry:sys", 900, () => {
      const tk = M.snap.takeover;
      tk.system = { wanted: on, status: on ? "on" : "off" };
      if (on) tk.on = true;
      log("info", on ? "系统代理已指向 127.0.0.1:7899" : "已恢复系统代理设置");
    }),
    setTakeover: (on) => apply("takeover", 600, () => {
      const tk = M.snap.takeover;
      tk.on = on && (tk.system.wanted || tk.tun.wanted);
      if (tk.system.wanted) tk.system.status = tk.on ? "on" : "off";
      if (tk.tun.wanted) tk.tun.status = tk.on ? "on" : "off";
    }),
    setTun: async (on) => {
      const tk = M.snap.takeover, first = on && tk.tun.status === "install";
      return apply("entry:tun", first ? 2400 : 900, () => {
        tk.tun = { wanted: on, status: on ? "on" : "off", detail: on ? "utun9" : undefined };
        if (on) tk.on = true;
      });
    },
  };
  const Routes: Api["Routes"] = {
    locate: async (node) => {
      const n = M.snap.nodes.find((x) => x.name === node) ?? fail("Routes.Locate", "没有这个节点");
      locateLater(n, 1800);
    },
    select: (rid, node) => {
      const r = route(rid);
      if (r.direct) return Promise.reject(new MockCallError("Routes.Select", "直连路由不用节点"));
      const n = node ? M.snap.nodes.find((x) => x.name === node) : fastest();
      if (node && !n) return Promise.reject(new MockCallError("Routes.Select", `没有节点「${node}」`));
      return apply("node:" + rid, 1100, () => {
        r.auto = !node;
        r.node = n?.name ?? r.node;
        if (n) setDelay(n, baseDelay.get(n.name) ?? 100);
        log("info", `${r.name} → ${n?.display ?? "—"}${node ? "" : "（自动）"}`);
        if (n && n.where.source !== "probe" && (rid === "default" || n.where.source === "unknown")) setTimeout(() => locateLater(n), 50);
      });
    },
    test: (rid) => { route(rid); return apply("test:" + rid, 1500, () => { for (const n of M.snap.nodes) setDelay(n, baseDelay.get(n.name) ?? 100); for (const r of M.snap.routes) if (r.auto && !r.direct) r.node = fastest()?.name ?? r.node; }); },
  };
  const Providers: Api["Providers"] = {
    add: async (input) => { const kind = DETECT(input); return apply("add", 1400, () => addProvider(kind, input)); },
    addFile: () => apply("add", 900, () => addProvider("file", "proxies:")),
    delete: (id) => apply("provider:" + id, 600, () => {
      const s = M.snap;
      s.providers = s.providers.filter((p) => p.id !== id);
      const gone = new Set(s.nodes.filter((n) => n.provider === id).map((n) => n.name));
      s.nodes = s.nodes.filter((n) => n.provider !== id);
      for (const r of s.routes) if (gone.has(r.node)) { r.node = fastest()?.name ?? ""; r.auto = true; }
    }),
    detect: async (input) => { await wait(120); return DETECT(input); },
    rename: (id, name) => apply("provider:" + id, 300, () => { const p = M.snap.providers.find((x) => x.id === id); if (p) p.name = name; }),
    setAutoUpdate: (id, hours) => apply("provider:" + id, 300, () => { const p = M.snap.providers.find((x) => x.id === id); if (p) p.autoUpdate = hours; }),
    setEnabled: (id, on) => apply("provider:" + id, 500, () => {
      const p = M.snap.providers.find((x) => x.id === id);
      if (p) p.enabled = on;
      if (!on) for (const r of M.snap.routes) if (M.snap.nodes.find((n) => n.name === r.node)?.provider === id) { r.node = fastest()?.name ?? ""; r.auto = true; }
    }),
    update: (id) => {
      const p = M.snap.providers.find((x) => x.id === id);
      if (!p) return Promise.reject(new MockCallError("Providers.Update", "没有这个 Provider"));
      if (p.kind !== "subscription") return Promise.reject(new MockCallError("Providers.Update", "只有订阅可以更新"));
      return apply("provider:" + id, 1500, () => { p.updatedAt = now(); p.error = undefined; log("info", `Provider「${p.name}」已更新`); });
    },
    updateAll: () => apply("providers", 1800, () => { for (const p of M.snap.providers) if (p.kind === "subscription" && p.enabled) p.updatedAt = now(); }),
  };
  const Activity: Api["Activity"] = {
    close: async (ids) => { await wait(200); for (const c of M.conns) if (ids.includes(c.id) && c.status === "active") { c.status = "closed"; c.end = now(); c.downRate = c.upRate = 0; } },
    connections: async () => structuredClone(M.conns),
    logs: async (after) => structuredClone(M.logs.filter((l) => l.seq > after)),
  };
  const events: Api["events"] = { snapshot: event("snapshot", snapListeners), tick: event("tick", tickListeners) };

  // Levers for poking at the page from the console.
  (window as unknown as { __mock: unknown }).__mock = {
    M,
    emit,
    hijack: () => { M.snap.takeover.system = { wanted: true, status: "hijacked", detail: "现在指向 127.0.0.1:1087" }; emit(); },
    quota: () => { const b = M.snap.providers.find((p) => p.id === "b"); if (b?.usage) b.usage.download = 188 * 1024 ** 3; emit(); },
    failover: () => { const r = M.snap.routes.find((x) => x.id === "dev")!; r.node = "b/大阪 01"; emit(); },
  };
  return { Moho, Control, Routes, Providers, Activity, events };
}
