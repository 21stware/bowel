// 活动: connections and logs. Polled every second while the page is open
// (unless paused); filtering and sorting happen here.

import { api } from "./api";
import type { Conn, LogLine } from "./mygo";
import { clock, dur, esc, fmt, when } from "./format";
import { S, nodeByDisplay, normalizeConns, normalizeLogs, rerender, route, routes } from "./store";
import { $, errorText, setHTML, toast } from "./ui";

let timer = 0;
let polling = false;

export function startActivity(): void {
  stopActivity();
  void poll();
  timer = window.setInterval(() => void poll(), 1000);
}
export function stopActivity(): void { clearInterval(timer); timer = 0; }

async function poll(): Promise<void> {
  if (polling || S.page !== "activity" || document.hidden) return;
  polling = true;
  try {
    if (!S.act.paused || !S.conns.length) {
      const [cs, ls] = await Promise.all([api.Activity.connections(), api.Activity.logs(S.lastSeq)]);
      S.conns = normalizeConns(cs);
      for (const id of [...S.closing]) if (!S.conns.some((c) => c.id === id && c.status === "active")) S.closing.delete(id);
      addLogs(normalizeLogs(ls));
    }
  } catch (err) {
    // A failing poll is retried next second; say it once.
    if (!pollFailed) toast(errorText(err));
    pollFailed = true;
    polling = false;
    return;
  }
  pollFailed = false;
  polling = false;
  if (S.page === "activity") patchBody();
}
let pollFailed = false;

function addLogs(ls: LogLine[]): void {
  if (!ls.length) return;
  for (const l of ls) if (l.seq > S.lastSeq) { S.logs.push(l); S.lastSeq = l.seq; }
  if (S.logs.length > 3000) S.logs.splice(0, S.logs.length - 3000);
}

// ---------------------------------------------------------------- filtering

const durOf = (c: Conn) => {
  const s = when(c.start)?.getTime(), e = when(c.end)?.getTime() ?? Date.now();
  return s ? Math.max(0, (e - s) / 1000) : 0;
};
const nodeText = (c: Conn) => c.node || (route(c.route)?.direct || !c.route ? "直连" : "—");
const SORTS: Record<string, (c: Conn) => number> = {
  rate: (c) => c.downRate + c.upRate, down: (c) => c.down, up: (c) => c.up, dur: durOf,
};

function filtered(): Conn[] {
  const a = S.act, q = a.q.trim().toLowerCase();
  // Tokens: app:Arc route:ai node:香港 host:x.com; other words match any field.
  const tokens = q.split(/\s+/).filter(Boolean);
  const rows = S.conns.filter((c) => {
    if (a.status !== "all" && c.status !== a.status) return false;
    if (a.route !== "all" && c.route !== a.route) return false;
    if (a.app !== "all" && c.app !== a.app) return false;
    if (a.net !== "all" && c.network.toLowerCase() !== a.net) return false;
    const r = route(c.route), rn = (r?.name ?? "").toLowerCase(), nn = nodeText(c).toLowerCase();
    const hay = `${c.app} ${c.process} ${c.host} ${c.ip} ${c.port} ${c.rule} ${c.payload} ${rn} ${c.route} ${nn} ${c.chain.join(" ")} ${c.error ?? ""}`.toLowerCase();
    return tokens.every((t) => {
      const i = t.indexOf(":");
      const [k, v] = i > 0 ? [t.slice(0, i), t.slice(i + 1)] : [null, t];
      if (k === "app") return c.app.toLowerCase().includes(v);
      if (k === "route") return rn.includes(v) || c.route.toLowerCase().includes(v);
      if (k === "node") return nn.includes(v) || c.chain.some((x) => x.toLowerCase().includes(v));
      if (k === "host") return c.host.toLowerCase().includes(v) || c.ip.includes(v);
      return hay.includes(t);
    });
  });
  const [k, dir] = a.sort, f = SORTS[k] ?? SORTS.rate!;
  return rows.sort((x, y) => (f(x) - f(y)) * dir);
}

// ---------------------------------------------------------------- markup

const sw = (color?: string) => `<span class="sw" style="background:${color ? `var(${esc(color)})` : "var(--faint)"}"></span>`;

function head(): string {
  const a = S.act, n = S.conns.filter((c) => c.status === "active").length;
  return `<div><div class="kicker">活动</div><h2>${n} 个连接<small class="num">↓ ${fmt(S.tick.down, "/s")} · ↑ ${fmt(S.tick.up, "/s")} · 今日 ${fmt(S.tick.todayDown)}</small></h2></div>
    <div class="right"><div class="subtabs">${([["conn", "连接"], ["log", "日志"]] as const).map(([id, l]) => `<button class="${a.tab === id ? "on" : ""}" data-acttab="${id}">${l}</button>`).join("")}</div></div>`;
}
const liveBadge = () => `<span class="live ${S.act.paused ? "paused" : ""}"><i></i>${S.act.paused ? "已暂停" : "实时"}</span><button class="btn" data-pause>${S.act.paused ? "▶ 继续" : "❙❙ 暂停"}</button>`;

function q1(): string {
  const a = S.act;
  if (a.tab === "log") {
    const lvs: [string, string][] = [["all", "全部"], ["debug", "调试"], ["info", "信息"], ["warning", "警告"], ["error", "错误"]];
    return `<label class="qinput"><span class="muted">⌕</span><input id="lq" placeholder="搜索日志" value="${esc(a.lq)}" autocomplete="off" spellcheck="false" /></label>
      <div class="seg">${lvs.map(([id, l]) => `<button class="${a.lv === id ? "on" : ""}" data-lv="${id}">${l}</button>`).join("")}</div>
      ${liveBadge()}<button class="btn" data-log-copy>复制</button><button class="btn" data-log-clear>清空</button>`;
  }
  const n = filtered().filter((c) => c.status === "active" && !S.closing.has(c.id)).length;
  return `<label class="qinput"><span class="muted">⌕</span><input id="aq" placeholder="搜索域名、IP、应用、节点、规则" value="${esc(a.q)}" autocomplete="off" spellcheck="false" /><span class="hint">app: route: node: host:</span></label>
    ${liveBadge()}
    <button class="btn danger ${a.armed ? "armed" : ""}" data-closeall ${n ? "" : "disabled"}>${a.armed ? `确认断开 ${n} 个？` : "断开筛选结果"}</button>`;
}
function q2a(): string {
  const a = S.act, count = (s: string) => S.conns.filter((c) => s === "all" || c.status === s).length;
  const rs = routes();
  const unknownRoute = S.conns.some((c) => !c.route);
  return `<div class="seg">${[["active", `活跃 ${count("active")}`], ["closed", `已关闭 ${count("closed")}`], ["failed", `失败 ${count("failed")}`], ["all", "全部"]].map(([id, l]) => `<button class="${a.status === id ? "on" : ""}" data-astatus="${id}">${l}</button>`).join("")}</div>
    <div class="chips">${[["all", "所有路由", ""], ...rs.map((r) => [r.id, r.name, r.color]), ...(unknownRoute ? [["", "未知", ""]] : [])].map(([id, l, c]) => `<button class="${a.route === id ? "on" : ""}" data-aroute="${esc(id)}">${c ? sw(c) : ""}${esc(l)}</button>`).join("")}</div>`;
}
function q2b(): string {
  const a = S.act, apps = [...new Set(S.conns.map((c) => c.app))].sort((x, y) => x.localeCompare(y));
  if (a.app !== "all" && !apps.includes(a.app)) apps.unshift(a.app);
  return `<select class="sel" id="aapp"><option value="all">所有应用</option>${apps.map((x) => `<option value="${esc(x)}" ${a.app === x ? "selected" : ""}>${esc(x)}</option>`).join("")}</select>
    <select class="sel" id="anet">${[["all", "TCP + UDP"], ["tcp", "TCP"], ["udp", "UDP"]].map(([v, l]) => `<option value="${v}" ${a.net === v ? "selected" : ""}>${l}</option>`).join("")}</select>`;
}

function connTable(): string {
  const rows = filtered(), [sk, sd] = S.act.sort;
  const H = (k: string, l: string) => `<button class="${sk === k ? "sorted" : ""}" data-sort="${k}">${l}${sk === k ? (sd < 0 ? " ↓" : " ↑") : ""}</button>`;
  const header = `<div class="tr h"><span></span><span>应用</span><span>目标</span><span>协议</span><span>命中规则</span><span>路由 · 出口</span><span>${H("rate", "速率")}</span><span>${H("down", "下载")}</span><span>${H("up", "上传")}</span><span>${H("dur", "时长")}</span><span></span></div>`;
  if (!rows.length) return header + `<div class="empty-row muted">${S.conns.length ? "没有符合条件的连接" : "还没有连接"}</div>`;
  return header + rows.map((c) => {
    const r = route(c.route), open = S.act.open === c.id, closing = S.closing.has(c.id);
    const via = c.status === "failed" ? `${nodeText(c)} 失败` : nodeText(c);
    return `<div class="tr ${esc(c.status)} ${open ? "open" : ""}" data-conn="${esc(c.id)}">
      <span class="st ${esc(c.status)}"></span><span class="clip">${esc(c.app || "—")}</span>
      <span class="host">${esc(c.host || c.ip)}<small>:${c.port}</small></span><span class="muted">${esc(c.network.toUpperCase())}</span>
      <span class="clip rule">${esc(c.rule)}${c.payload ? `(${esc(c.payload)})` : ""}</span>
      <span class="via">${sw(r?.color)}<span>${esc(r?.name ?? "未知")} · ${esc(via)}</span></span>
      <span class="num">${c.status === "active" ? fmt(c.downRate + c.upRate, "/s") : "—"}</span><span class="num">${fmt(c.down)}</span><span class="num muted">${fmt(c.up)}</span>
      <span class="num t">${c.status === "failed" ? "失败" : dur(durOf(c))}</span>
      ${c.status === "active" ? `<button class="x ${closing ? "shimmer" : ""}" data-kill="${esc(c.id)}" title="断开">×</button>` : "<span></span>"}</div>
      ${open ? detail(c) : ""}`;
  }).join("");
}
function detail(c: Conn): string {
  const failed = c.status === "failed", r = route(c.route), n = c.node ? nodeByDisplay(c.node) : undefined;
  const chain = [c.app || "应用", ...(r ? [`${sw(r.color)} ${esc(r.name)}`] : []), ...c.chain.filter((x) => x !== r?.name).map(esc)];
  if (!c.chain.length) chain.push(esc(nodeText(c)));
  const last = chain.length - 1;
  const base = c.host.split(".").slice(-2).join(".");
  return `<div class="detail">
    <div class="kv"><span>进程</span><span>${esc(c.process || "—")}</span><span>目标</span><span>${esc(c.host || "—")}:${c.port}</span><span>目标 IP</span><span>${esc(c.ip || "—")}</span>
      <span>开始</span><span>${clock(c.start) || "—"}</span>${c.end && when(c.end) ? `<span>结束</span><span>${clock(c.end)}</span>` : ""}</div>
    <div class="kv"><span>规则</span><span>${esc(c.rule)}${c.payload ? `,${esc(c.payload)}` : ""}</span>
      <span>链路</span><span class="chain">${chain.map((x, i) => `<span style="${failed && i === last ? "color:var(--signal)" : ""}">${i === 0 ? esc(x) : x}</span>`).join('<span class="arr">→</span>')}</span>
      <span>出口</span><span>${n ? `${esc(n.where.place?.city ?? "位置未知")} · ${esc(n.type)}` : c.node ? esc(c.node) : "本机网络"}</span>
      <span>流量</span><span>↓ ${fmt(c.down)} · ↑ ${fmt(c.up)}</span>
      ${c.error ? `<span>错误</span><span style="color:var(--signal)">${esc(c.error)}</span>` : ""}</div>
    <div class="acts">${c.status === "active" ? `<button class="btn danger" data-kill="${esc(c.id)}">断开</button>` : ""}
      ${base ? `<button class="btn" disabled title="规则编辑在设置里，还没做">让 ${esc(base)} 走… ▾</button>` : ""}
      ${c.host ? `<button class="btn" data-copy="${esc(c.host)}">复制域名</button>` : ""}
      <button class="btn" data-related-log="${esc(c.host || c.ip)}">相关日志</button></div>
  </div>`;
}

const LV: Record<string, [string, string]> = { debug: ["d", "DBG"], info: ["i", "INFO"], warning: ["w", "WARN"], error: ["e", "ERR"] };
function logLines(): LogLine[] {
  const a = S.act, q = a.lq.trim().toLowerCase();
  return S.logs.filter((l) => l.seq > a.clearedSeq && (a.lv === "all" || l.level === a.lv) && (!q || l.text.toLowerCase().includes(q)));
}
function logRows(): string {
  const ls = logLines();
  if (!ls.length) return `<div class="empty-row muted">${S.logs.length ? "没有符合条件的日志" : "还没有日志"}</div>`;
  return ls.map((l) => { const [c, t] = LV[l.level] ?? ["i", l.level.toUpperCase()]; return `<div class="logl"><span class="lt">${clock(l.time)}</span><span class="lv ${c}">${t}</span><span>${esc(l.text)}</span></div>`; }).join("");
}

// ---------------------------------------------------------------- render

export function renderActivity(page: HTMLElement): void {
  if (!$("act-root")) {
    page.innerHTML = `<div class="page-in" id="act-root"><div class="p-head" id="act-head"></div>
      <div class="qbar"><div class="q1" id="aq1"></div><div class="q2" id="aq2"><div class="q2a" id="aq2a"></div><span class="q2b" id="aq2b"></span></div></div>
      <div class="tbl" id="abody"></div></div>`;
  }
  setHTML($("act-head")!, head());
  setHTML($("aq1")!, q1());
  $("aq2")!.hidden = S.act.tab === "log";
  if (S.act.tab === "conn") { setHTML($("aq2a")!, q2a()); setHTML($("aq2b")!, q2b()); }
  patchBody();
}
function patchBody(): void {
  const body = $("abody");
  if (!body) return;
  if (S.act.tab === "conn") {
    setHTML(body, connTable());
    setHTML($("aq1")!, q1());
    setHTML($("aq2a")!, q2a());
    setHTML($("aq2b")!, q2b());
  } else {
    const atEnd = body.scrollHeight - body.scrollTop - body.clientHeight < 40;
    if (setHTML(body, logRows()) && atEnd) body.scrollTop = body.scrollHeight;
  }
  setHTML($("act-head")!, head());
}
export function tickActivity(): void { const h = $("act-head"); if (h) setHTML(h, head()); }

// ---------------------------------------------------------------- events

let armTimer = 0;
export function activityClick(t: HTMLElement, e: MouseEvent): boolean {
  const d = t.dataset, a = S.act;
  if (d.acttab) { a.tab = d.acttab as "conn" | "log"; rerender(); if (a.tab === "log") scrollLogsEnd(); return true; }
  if (d.astatus) { a.status = d.astatus; a.armed = false; rerender(); return true; }
  if (d.aroute !== undefined) { a.route = d.aroute; a.armed = false; rerender(); return true; }
  if (d.sort) { const [k, dir] = a.sort; a.sort = [d.sort, k === d.sort ? -dir : -1]; rerender(); return true; }
  if (d.kill) { e.stopPropagation(); close([d.kill]); return true; }
  if (d.conn) { a.open = a.open === d.conn ? null : d.conn; rerender(); return true; }
  if (d.pause !== undefined) { a.paused = !a.paused; rerender(); if (!a.paused) void poll(); return true; }
  if (d.closeall !== undefined) {
    clearTimeout(armTimer);
    if (!a.armed) { a.armed = true; armTimer = window.setTimeout(() => { a.armed = false; rerender(); }, 3000); rerender(); return true; }
    a.armed = false;
    close(filtered().filter((c) => c.status === "active" && !S.closing.has(c.id)).map((c) => c.id));
    return true;
  }
  if (d.lv) { a.lv = d.lv; rerender(); return true; }
  if (d.copy) { void navigator.clipboard?.writeText(d.copy).then(() => toast("已复制 " + d.copy, "info")); return true; }
  if (d.relatedLog) { a.tab = "log"; a.lq = d.relatedLog; a.lv = "all"; rerender(); scrollLogsEnd(); return true; }
  if (d.logCopy !== undefined) {
    const text = logLines().map((l) => `${clock(l.time)} ${(LV[l.level]?.[1] ?? l.level).padEnd(4)} ${l.text}`).join("\n");
    void navigator.clipboard?.writeText(text).then(() => toast(`已复制 ${logLines().length} 行日志`, "info"));
    return true;
  }
  if (d.logClear !== undefined) { a.clearedSeq = S.lastSeq; rerender(); return true; }
  return false;
}
function scrollLogsEnd(): void { const b = $("abody"); if (b) b.scrollTop = b.scrollHeight; }

function close(ids: string[]): void {
  if (!ids.length) return;
  ids.forEach((id) => S.closing.add(id));
  rerender();
  api.Activity.close(ids).then(() => poll(), (err) => {
    ids.forEach((id) => S.closing.delete(id));
    toast(errorText(err));
    rerender();
  });
}

export function activityInput(t: HTMLInputElement | HTMLSelectElement): boolean {
  if (t.id === "aq") { S.act.q = t.value; S.act.armed = false; rerender(); return true; }
  if (t.id === "lq") { S.act.lq = t.value; rerender(); return true; }
  if (t.id === "aapp") { S.act.app = t.value; S.act.armed = false; rerender(); return true; }
  if (t.id === "anet") { S.act.net = t.value; S.act.armed = false; rerender(); return true; }
  return false;
}

/** Open the page filtered to one app (from 谁在用). */
export function showApp(app: string): void {
  Object.assign(S.act, { tab: "conn", app, status: "all", q: "", route: "all", armed: false });
}
