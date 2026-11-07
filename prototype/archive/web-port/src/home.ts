// 今日: the whole home page is one grid board. Expanding a cell animates the
// grid's tracks, so the cell grows in place and every other cell is
// squeezed: narrower if it shares the row, shorter if it shares the
// column, both otherwise. Each cell has three views: normal (laid out at
// its normal size), compact (a variant chosen from the size the squeezed
// cell will have) and detail. Views switch without cross-fades: the new
// one opens like a drawer at its final size, the old one folds up, and
// shared pieces fly (motion.ts).

import { api } from "./api";
import type { NodeView, RouteView } from "./mygo";
import { countryName, delayText, esc, fmt, minSec, tone, unit } from "./format";
import { startMap, stopMap } from "./map";
import { morph } from "./motion";
import {
  S, act, active, busy, busyPrefix, defaultRoute, exitNode, hasNodes, mode, nodeByName, usableNodes, provider, rerender, route, routes, takeoverOn,
} from "./store";
import { $, cssVar, setHTML, typingIn } from "./ui";

// ---------------------------------------------------------------- layout

interface Cell { id: string; area: string; r: number; c: number[] }
const CELLS: Cell[] = [
  { id: "hero", area: "1 / 1 / 2 / 2", r: 1, c: [1] },
  { id: "map", area: "1 / 2 / 2 / 4", r: 1, c: [2, 3] },
  { id: "mode", area: "2 / 1 / 3 / 2", r: 2, c: [1] },
  { id: "takeover", area: "2 / 2 / 3 / 3", r: 2, c: [2] },
  { id: "routes", area: "2 / 3 / 3 / 4", r: 2, c: [3] },
  { id: "speed", area: "3 / 1 / 4 / 2", r: 3, c: [1] },
  { id: "apps", area: "3 / 2 / 4 / 3", r: 3, c: [2] },
  { id: "events", area: "3 / 3 / 4 / 4", r: 3, c: [3] },
];
const cellOf = (id: string | null) => (id ? CELLS.find((c) => c.id === id) : undefined);
/** The size of a squeezed row and column. */
const SQ = { row: 78, col: 190 };

/**
 * Track sizes in px for the board's real size: normal, off, or one cell
 * expanded. Every view is laid out at the size its cell has at the end of
 * the morph, so nothing reflows while the grid moves.
 */
function trackArrays(W: number, H: number, xId: string | null) {
  const x = cellOf(xId);
  let rows = [318, 150, 218].map((v) => (v * H) / 686);
  let cols = [400, 330, 330].map((v) => (v * W) / 1060);
  if (!takeoverOn()) rows = [H, 0, 0];
  if (x) {
    const top = Math.min(150, rows[0]!);
    rows = x.r === 1 ? [H - 2 * SQ.row, SQ.row, SQ.row] : x.r === 2 ? [top, H - top - SQ.row, SQ.row] : [top, SQ.row, H - top - SQ.row];
    cols = x.c.length === 2 ? [SQ.col, (W - SQ.col) / 2, (W - SQ.col) / 2] : [SQ.col, SQ.col, SQ.col].map((v, i) => (x.c[0] === i + 1 ? W - 2 * SQ.col : v));
  }
  return { cols, rows };
}
/** The px size a cell has for some tracks (1px gaps between cells). */
function sizeIn(c: Cell, t: { cols: number[]; rows: number[] }, W: number, H: number): [number, number] {
  const px = (a: number[], total: number) => { const sum = a.reduce((s, v) => s + v, 0) || 1; return a.map((v) => ((total - (a.length - 1)) * v) / sum); };
  const cp = px(t.cols, W), rp = px(t.rows, H);
  return [c.c.reduce((s, i) => s + cp[i - 1]!, 0) + (c.c.length - 1), rp[c.r - 1]!];
}
/** Squeezed cells pick a layout that fits their box, never truncating. */
type Variant = "mini" | "strip" | "column" | "wide";
const variantOf = (w: number, h: number): Variant => (h < 120 ? (w < 260 ? "mini" : "strip") : w < 260 ? "column" : "wide");

function viewOf(cell: Cell): "normal" | "compact" | "expanded" {
  const x = cellOf(S.x);
  if (!x || !takeoverOn()) return "normal";
  return x.id === cell.id ? "expanded" : "compact";
}

// ---------------------------------------------------------------- small pieces

const flagOf = (n?: NodeView | null) => esc(n?.where.place?.flag || "◌");
const cityOf = (n?: NodeView | null) => esc(n?.where.place?.city || "位置未知");
const provName = (n?: NodeView | null) => esc((n && provider(n.provider)?.name) || "未知来源");
const sw = (r: RouteView, size = 8) => `<span class="sw" style="background:var(${esc(r.color)});width:${size}px;height:${size}px"></span>`;

function locTag(n: NodeView, locatable = false): string {
  const src = n.where.source;
  if (src === "probe") return `<span class="loc probe">实测</span>`;
  if (src === "name") return `<span class="loc name"${locatable ? ` data-locate="${esc(n.name)}" title="经这个节点实测出口位置"` : ""}>按名称</span>`;
  if (src === "locating" || busy("locate:" + n.name)) return `<span class="loc unknown shimmer">定位中</span>`;
  return `<span class="loc unknown"${locatable ? ` data-locate="${esc(n.name)}" title="经这个节点实测出口位置"` : ""}>未知</span>`;
}
/** Stability: the last tests as bars, taller is faster, red failed. */
function stab(history: number[], n = 12, h = 20): string {
  const last = history.slice(-n);
  const pad = Array.from({ length: n - last.length }, () => `<i class="none" style="height:3px"></i>`);
  return pad.join("") + last.map((v) => (v <= 0 ? `<i class="fail" style="height:${h}px"></i>` : `<i style="height:${Math.max(3, h - (v * h) / 140).toFixed(1)}px"></i>`)).join("");
}
/** A node's name; its flag in front unless the name starts with one already. */
const startsWithFlag = (s: string) => /^\p{Regional_Indicator}{2}/u.test(s.trim());
const nodeLabel = (n?: NodeView | null) => (n ? (startsWithFlag(n.display) ? esc(n.display) : `${flagOf(n)} ${esc(n.display)}`) : "");

function cmp(v: Variant, id: string, k: string, val: string, o: { key?: string; sub?: string; list?: string; short?: string; cls?: string } = {}): string {
  const value = v === "mini" && o.short ? o.short : val;
  return `<div class="cmp ${v}"><span class="k" data-m="k-${id}">${k}</span>
    <span class="vl ${o.cls ?? ""}" ${o.key ? `data-m="${esc(o.key)}"` : ""}>${value}</span>
    ${o.sub && v !== "mini" ? `<span class="sb">${o.sub}</span>` : ""}
    ${o.list && (v === "column" || v === "wide") ? `<div class="li">${o.list}</div>` : ""}</div>`;
}
const detHead = (kicker: string, title: string, right = "", key = "") =>
  `<div class="det-h"><div><div class="k" ${key ? `data-m="${key}"` : ""}>${kicker}</div><h3>${title}</h3></div><div class="right">${right}<button class="x" data-collapse title="收起 (Esc)">×</button></div></div>`;

// ---------------------------------------------------------------- views

interface Views { normal: string; compact: (v: Variant) => string; detail: string }

function heroViews(): Views {
  const off = !takeoverOn(), m = mode(), r0 = defaultRoute(), ex = exitNode();
  const switching = !!r0 && busy("node:" + r0.id);
  const core = S.snap.core;
  if (off) {
    const tk = S.snap.takeover, sysB = busy("entry:sys"), tunB = busy("entry:tun");
    const last = [tk.system.wanted && "系统代理", tk.tun.wanted && "TUN"].filter(Boolean).join(" + ");
    const lastNode = nodeByName(r0?.node);
    const lastLine = [last, lastNode && provider(lastNode.provider)?.name, lastNode?.display].filter(Boolean).map(esc).join(" · ");
    const tunText = tunB ? (tk.tun.status === "install" ? "正在安装服务，请在系统弹窗中确认…" : "正在接管…") : tk.tun.status === "install" ? "所有应用 · 首次需安装服务" : "所有应用，包括不认系统代理的";
    return {
      normal: `<div class="pad hero-pad off-hero">
        <div class="kicker">MOHO</div>
        <h1 class="headline" data-m="exit-default">未接管</h1>
        <div class="deck">所有应用都直接连接互联网，流量不经过 Moho。</div>
        <div class="choose">
          <button class="choice ${sysB ? "busy" : ""}" data-choose="sys"><b class="${sysB ? "shimmer" : ""}">系统代理</b><span>${sysB ? "正在接管…" : "浏览器和大多数应用 · 推荐"}</span></button>
          <button class="choice ${tunB ? "busy" : ""}" data-choose="tun"><b class="${tunB ? "shimmer" : ""}">TUN</b><span>${tunText}</span></button>
        </div>
        ${lastLine ? `<div class="muted last">上次使用：${lastLine}</div>` : ""}
      </div>`,
      compact: () => "", detail: "",
    };
  }
  if (!hasNodes()) {
    return {
      normal: `<div class="pad hero-pad">
        <div class="kicker" data-m="k-hero">当前出口</div>
        <h1 class="headline" data-m="exit-default">还没有节点</h1>
        <div class="deck">添加一个 Provider：订阅链接、节点链接或配置文件。Moho 会从里面为每类流量挑最好的出口，其余直连。</div>
        <div class="hero-actions"><button class="pill" data-page="providers">添加 Provider</button><button class="pill ghost" data-addfile>${busy("add") ? '<span class="shimmer">正在添加…</span>' : "选择文件…"}</button></div>
      </div>`,
      compact: (v) => cmp(v, "hero", "当前出口", "还没有节点", { key: "exit-default", sub: "添加 Provider 后开始", list: `<span>订阅链接、节点链接或配置文件</span>` }),
      detail: "",
    };
  }
  let headline: string, deck: string;
  if (core.state === "error") { headline = "内核未运行"; deck = esc(core.error || "mihomo 没有在运行。"); }
  else if (m === "direct") { headline = "直接连接"; deck = `直连模式：所有流量<b>不经过任何节点</b>，规则暂停。适合排查网络问题。`; }
  else if (!ex) { headline = `<span class="shimmer">正在选择出口…</span>`; deck = core.state === "starting" ? `<span class="shimmer">正在启动内核…</span>` : "正在为默认路由挑选节点。"; }
  else {
    headline = startsWithFlag(ex.display) ? esc(ex.display) : `<span class="flag">${flagOf(ex)}</span>${esc(ex.display)}`;
    const where = ex.where.source === "locating" ? `<span class="shimmer">正在实测出口位置…</span>` : `${cityOf(ex)} ${locTag(ex, true)}`;
    deck = `${where} · ${esc(ex.type)} · 来自 <b>${provName(ex)}</b>。${m === "global" ? "全局模式下<b>所有流量</b>都从这里出去。" : `没命中其他规则的流量从这里出去，约占 <b>${Math.round((r0?.share ?? 0) * 100)}%</b>。`}`;
  }
  const testing = !!r0 && busy("test:" + r0.id);
  return {
    normal: `<div class="pad hero-pad">
      <div class="kicker" data-m="k-hero">当前出口 · <b>${m === "global" ? "全局" : "默认路由"}</b></div>
      <h1 class="headline ${switching ? "shimmer" : ""}" data-m="exit-default">${headline}</h1>
      <div class="deck">${switching ? `<span class="shimmer">正在切换并测速…</span>` : deck}</div>
      ${ex && r0 ? `<div class="lat-big"><span class="v num ${switching || testing ? "skel" : ""}" style="color:${tone(ex.delay, ex.tested)}">${ex.tested ? (ex.delay > 0 ? `${ex.delay}<small>ms</small>` : "超时") : `—<small>未测速</small>`}</span>
        <span class="stab"><span>最近 12 次测速</span><span class="bars">${stab(ex.history)}</span></span></div>
      <div class="hero-actions">
        <button class="pill" data-expand="hero">换节点</button>
        <button class="pill ghost ${r0.auto ? "on" : ""}" data-auto-toggle="${esc(r0.id)}">${r0.auto ? '<span class="gdot"></span>' : ""}自动选最快</button>
      </div>` : ""}
    </div>`,
    compact: (v) => cmp(v, "hero", "当前出口", m === "direct" ? "直接连接" : ex ? nodeLabel(ex) : "正在选择…", {
      key: "exit-default", cls: switching ? "shimmer" : "",
      sub: ex ? `<span style="color:${tone(ex.delay, ex.tested)}">${delayText(ex.delay, ex.tested)}</span> · ${cityOf(ex)} · ${esc(ex.type)}` : "",
      list: ex && r0 ? `<span>来自 ${provName(ex)}</span><span>${r0.auto ? "自动选最快" : "手动选择"}</span>` : "",
    }),
    detail: r0 ? `<div class="det">${detHead("当前出口 · 默认路由 · 选择出口", ex ? `<span class="${switching ? "shimmer" : ""}" data-m="exit-default">${nodeLabel(ex)}</span>` : "选择出口", autoSeg(r0), "k-hero")}
      <div class="det-b" data-keep-scroll="hero">${nodePicker(r0)}</div></div>` : "",
  };
}

function mapViews(): Views {
  const off = !takeoverOn();
  const px = routes().filter((r) => !r.direct);
  const lines = px.filter((r) => active(r) && r.node);
  return {
    normal: `<div class="map-cap">${off ? "开始接管后，这里会画出流量去往的出口。" : !hasNodes() ? "添加 Provider 后，这里会画出每条路由的出口。" : `<b>从你出发的 ${lines.length} 条线路</b>　线越粗流量越大，光点是正在传输的数据。`}</div>
      <div class="map-legend">${px.map((r) => `<span style="opacity:${active(r) ? 1 : 0.35}">${esc(r.name)}<i style="background:var(${esc(r.color)})"></i></span>`).join("")}</div>`,
    compact: () => "",
    detail: `<div class="map-cap"><b>出口地图</b>　点任一路由换节点 · 再点地图收起</div>
      <div class="map-side">${px.map((r) => {
        const n = nodeByName(r.node);
        return `<button class="ms-row" data-expand="routes" data-sub="${esc(r.id)}" style="opacity:${active(r) ? 1 : 0.4}">
          ${sw(r)}
          <span><b>${n ? nodeLabel(n) : "未选择节点"}</b><span class="muted">${esc(r.name)} · ${n ? `${cityOf(n)} ${locTag(n)}` : "—"}</span></span>
          <span class="num" style="color:${n ? tone(n.delay, n.tested) : "var(--mute)"}">${n ? delayText(n.delay, n.tested) : "—"}<br><span class="muted">${Math.round(r.share * 100)}%</span></span></button>`;
      }).join("")}</div>`,
  };
}

const MODES: [string, string, string][] = [
  ["rule", "规则", "按规则把流量分到不同路由。日常就用这个。"],
  ["global", "全局", "所有流量都走「默认」路由的出口，规则暂停。临时需要时用。"],
  ["direct", "直连", "所有流量直接连接，不经过任何节点。排查网络问题时用。"],
];
function modeViews(): Views {
  const m = mode(), rc = S.snap.ruleCount, nr = routes().length;
  const pend = busy("mode"), target = S.over.modeTarget ?? m;
  const desc: Record<string, string> = {
    rule: `按 <b>${rc} 条规则</b>把流量分到 ${nr} 条路由；没命中的走默认。`,
    global: "<b>所有流量</b>都走当前出口，规则暂不生效。",
    direct: "<b>所有流量直接连接</b>，不经过任何节点。",
  };
  const sub: Record<string, string> = { rule: `${rc} 条规则 · ${nr} 条路由`, global: "所有流量走默认出口", direct: "不经过任何节点" };
  return {
    normal: `<div class="pad"><span class="hint">展开</span><div class="kicker" data-m="k-mode">模式</div>
      <div class="words">${MODES.map(([id, l]) => `<button class="word ${m === id ? "on" : ""} ${pend && target === id ? "shimmer" : ""}" data-mode="${id}">${l}${m === id ? '<span class="ul" data-m="mode-ul" data-mbox></span>' : ""}</button>`).join("")}</div>
      <p class="note">${desc[m] ?? ""}</p></div>`,
    compact: (v) => cmp(v, "mode", "模式", MODES.find((w) => w[0] === m)?.[1] ?? m, { sub: sub[m], cls: pend ? "shimmer" : "" }),
    detail: `<div class="det">${detHead("模式", "模式与规则", "", "k-mode")}<div class="det-b" data-keep-scroll="mode">
      ${MODES.map(([id, l, d]) => `
        <div class="dsec"><h4>${l}<small>${m === id ? "当前" : ""}</small></h4><div class="muted" style="font-size:12.5px">${d}</div>
          <button class="btn" data-mode="${id}">${pend && target === id ? '<span class="shimmer">切换中…</span>' : m === id ? "✓ 使用中" : "切换"}</button></div>`).join("")}
      <div class="dsec"><h4>规则<small>${rc} 条 · Moho 默认模板</small></h4>
        <div class="kv">${routes().map((r) => `<span>${sw(r)} ${esc(r.name)}</span><span>${r.rules} 条 · ${esc(r.summary)}</span>`).join("")}</div>
        <button class="btn" disabled title="规则编辑在设置里，还没做">编辑规则…</button></div></div></div>`,
  };
}

function entryText(id: "sys" | "tun"): { cls: string; text: string } {
  const e = id === "sys" ? S.snap.takeover.system : S.snap.takeover.tun;
  if (busy("entry:" + id)) {
    const t = e.status === "on" && e.wanted ? "正在关闭…" : id === "tun" && e.status === "install" ? "正在安装服务，请在系统弹窗中确认…" : "正在接管…";
    return { cls: e.status === "on" ? "on" : "", text: `<span class="shimmer">${t}</span>` };
  }
  if (id === "sys" && busy("retake")) return { cls: "bad", text: `<span class="shimmer">正在重新接管…</span>` };
  switch (e.status) {
    case "on": return { cls: "on", text: `<i></i>生效中 · ${id === "sys" ? "浏览器和大多数应用" : "所有应用"}` };
    case "hijacked": return { cls: "bad", text: "<i></i>被其他程序改掉了 · 点此重新接管" };
    case "error": return { cls: "bad", text: `<i></i>异常${e.detail ? " · " + esc(e.detail) : ""}` };
    case "install": return { cls: "", text: "<i></i>未使用 · 首次需安装服务" };
    default: return { cls: "", text: `<i></i>${id === "tun" ? "未使用 · 接管所有应用" : "未使用"}` };
  }
}
function entryWord(id: "sys" | "tun", word: string): string {
  const { cls, text } = entryText(id);
  return `<button class="entry ${cls}" data-entry="${id}"><span class="w ${busy("entry:" + id) ? "shimmer" : ""}" data-m="tk-${id}">${word}</span><span class="s">${text}</span></button>`;
}
function entryStatus(id: "sys" | "tun"): string {
  const tk = S.snap.takeover, e = id === "sys" ? tk.system : tk.tun;
  if (busy("entry:" + id)) return `<span class="shimmer">${id === "tun" && e.status === "install" ? "正在安装提权服务，请在系统弹窗中确认…" : "正在应用…"}</span>`;
  const d = e.detail ? ` · ${esc(e.detail)}` : "";
  switch (e.status) {
    case "on": return `<b style="color:var(--good)">生效中</b>${d}`;
    case "hijacked": return `<b style="color:var(--signal)">被其他程序改掉了</b>${d}　<button class="btn sm" data-retake>${busy("retake") ? '<span class="shimmer">正在重新接管…</span>' : "重新接管"}</button>`;
    case "error": return `<b style="color:var(--signal)">异常</b>${d}`;
    case "install": return `未使用 · 需要先安装一次提权服务${d}`;
    default: return `未使用${d}`;
  }
}
function takeoverViews(): Views {
  const tk = S.snap.takeover;
  const names = [tk.system.wanted && "系统代理", tk.tun.wanted && "TUN"].filter(Boolean) as string[];
  const bad = tk.system.status === "hijacked" || tk.system.status === "error" || tk.tun.status === "error";
  const lan = busy("lan") ? !tk.lan : tk.lan;
  return {
    normal: `<div class="pad"><span class="hint">展开</span><div class="kicker" data-m="k-takeover">接管</div><div class="entries">${entryWord("sys", "系统代理")}${entryWord("tun", "TUN")}</div></div>`,
    compact: (v) => cmp(v, "takeover", "接管", names.length ? names.join(" + ") : "未接管", {
      cls: (busyPrefix("entry:") ? "shimmer " : "") + (bad ? "bad" : ""),
      sub: tk.system.status === "hijacked" ? "被其他程序改掉了" : bad ? "有入口异常" : names.length ? "生效中" : "",
      list: `<span>端口 ${tk.port || "—"}</span><span>局域网 ${tk.lan ? "允许" : "不允许"}</span>`,
    }),
    detail: `<div class="det">${detHead("接管", "接管方式", `<button class="btn" data-pause-takeover>${busy("takeover") ? '<span class="shimmer">正在暂停…</span>' : "暂停接管"}</button>`, "k-takeover")}<div class="det-b" data-keep-scroll="takeover">
      <div class="dsec"><h4><span data-m="tk-sys">系统代理</span><small>浏览器和大多数应用</small></h4>
        <div class="kv"><span>状态</span><span>${entryStatus("sys")}</span>
          <span>地址</span><span class="num">HTTP / HTTPS / SOCKS → 127.0.0.1:${tk.port || "—"}</span>
          <span>自检</span><span>定时确认系统代理仍指向 Moho</span>
          <span>不走代理</span><span>${tk.bypass.length ? esc(tk.bypass.join(", ")) : "—"}</span></div>
        <button class="toggle ${tk.system.wanted ? "on" : ""} ${busy("entry:sys") ? "busy" : ""}" data-entry-toggle="sys" title="${tk.system.wanted ? "关闭" : "打开"}系统代理"></button></div>
      <div class="dsec"><h4><span data-m="tk-tun">TUN</span><small>所有应用，包括不认系统代理的</small></h4>
        <div class="kv"><span>状态</span><span>${entryStatus("tun")}</span><span>说明</span><span>${tk.tun.status === "install" ? "首次打开时 macOS 会请求管理员授权" : "虚拟网卡接管所有 TCP / UDP 流量"}</span></div>
        <button class="toggle ${tk.tun.wanted ? "on" : ""} ${busy("entry:tun") ? "busy" : ""}" data-entry-toggle="tun" title="${tk.tun.wanted ? "关闭" : "打开"} TUN"></button></div>
      <div class="dsec"><h4>本地端口<small>给手动配置的应用用</small></h4>
        <div class="kv"><span>混合端口</span><span class="num">${tk.port || "—"} · HTTP 和 SOCKS5</span><span>局域网</span><span class="${busy("lan") ? "shimmer" : ""}">${lan ? `允许${tk.lanAddr ? " · " + esc(tk.lanAddr) : ""}` : "不允许"}</span></div>
        <button class="toggle ${lan ? "on" : ""} ${busy("lan") ? "busy" : ""}" data-lan title="允许局域网内的设备使用"></button></div>
    </div></div>`,
  };
}

function routeExit(r: RouteView): string {
  if (r.direct) return "直接连接";
  const n = nodeByName(r.node);
  return n ? nodeLabel(n) : `<span class="muted">未选择</span>`;
}
function routesViews(): Views {
  const all = routes(), others = all.filter((r) => r.id !== defaultRoute()?.id);
  const sub = S.sub ? route(S.sub) : undefined;
  const px = others.filter((r) => !r.direct);
  return {
    normal: `<div class="pad"><span class="hint">展开</span><div class="kicker" data-m="k-routes">其他路由</div><div class="routes">${others.map((r) => {
      const n = r.direct ? null : nodeByName(r.node);
      return `<button class="route ${active(r) ? "" : "dim"}" data-expand="routes" ${r.direct ? "" : `data-sub="${esc(r.id)}"`}>
        ${sw(r)}<span class="rn" data-m="rn-${esc(r.id)}">${esc(r.name)}</span>
        <span class="ex ${busy("node:" + r.id) ? "shimmer" : ""}" data-m="exit-${esc(r.id)}">${routeExit(r)}</span>
        <span class="ms num" ${n ? `style="color:${n.tested && n.delay <= 0 ? "var(--signal)" : "var(--mute)"}"` : ""}>${n ? delayText(n.delay, n.tested) : r.direct ? "国内 · 局域网" : "—"}</span></button>`;
    }).join("")}</div></div>`,
    compact: (v) => {
      const dots = px.map((r) => sw(r)).join(" ");
      if (v === "strip") {
        const inline = px.map((r) => `${sw(r)} ${esc(r.name)} → ${esc(nodeByName(r.node)?.display ?? "—")}`).join("　");
        return `<div class="cmp strip"><span class="k" data-m="k-routes">其他路由</span><span class="vl rlist-inline">${inline}</span></div>`;
      }
      return cmp(v, "routes", "其他路由", `${dots}<span class="cnt">${px.length} 条</span>`, {
        list: px.map((r) => `<span>${sw(r)} ${esc(r.name)}<br><b>${nodeLabel(nodeByName(r.node)) || "—"}</b></span>`).join(""),
      });
    },
    detail: sub && !sub.direct
      ? `<div class="det">${detHead(`<button class="back" data-sub-back>‹ 路由</button>`, `${sw(sub, 10)}<span data-m="rn-${esc(sub.id)}">${esc(sub.name)}</span> → <span class="${busy("node:" + sub.id) ? "shimmer" : ""}" ${sub.id === defaultRoute()?.id ? "" : `data-m="exit-${esc(sub.id)}"`}>${routeExit(sub)}</span>`, autoSeg(sub), "k-routes")}
        <div class="det-b" data-keep-scroll="sub">${nodePicker(sub)}</div></div>`
      : `<div class="det">${detHead("路由", "每类流量走哪里", `<button class="btn" disabled title="新建路由在设置里，还没做">＋ 新建路由</button>`, "k-routes")}<div class="det-b" data-keep-scroll="routes"><div class="rlist">
        ${all.map((r) => {
          const n = r.direct ? null : nodeByName(r.node);
          return `<button class="rr" ${r.direct ? "" : `data-sub-open="${esc(r.id)}"`}>
            ${sw(r, 10)}<span class="rn" data-m="rn-${esc(r.id)}">${esc(r.name)}</span>
            <span class="rules">${r.rules} 条规则 · ${esc(r.summary)}</span>
            <span class="ex ${busy("node:" + r.id) ? "shimmer" : ""}" ${r.id === defaultRoute()?.id ? "" : `data-m="exit-${esc(r.id)}"`}>${routeExit(r)}${r.auto && !r.direct ? ' <span class="loc name">自动</span>' : ""}</span>
            <span class="num" style="color:${n ? tone(n.delay, n.tested) : "var(--mute)"}">${n ? delayText(n.delay, n.tested) : "—"}</span>
            <span class="num muted">${Math.round(r.share * 100)}%</span><span class="muted">${r.direct ? "" : "›"}</span></button>`;
        }).join("")}
      </div></div></div>`,
  };
}

function speedViews(): Views {
  const spans: [number, string][] = [[60, "1 分钟"], [600, "10 分钟"], [3600, "1 小时"]];
  return {
    normal: `<div class="pad now-pad"><span class="hint">展开</span><div class="kicker" data-m="k-speed">此刻速度</div>
      <div class="speed" data-live="speed" data-m="spd"></div>
      <canvas class="spark" id="spark"></canvas>
      <div class="foot num"><span data-live="speed-foot"></span><span>颜色 = 路由</span></div></div>`,
    compact: (v) => cmp(v, "speed", "此刻速度", `<span data-live="speed-c"></span>`, { key: "spd", cls: "num", sub: `<span data-live="speed-c-sub"></span>` }),
    detail: `<div class="det">${detHead("流量", "按路由看流量", `<div class="seg">${spans.map(([s, l]) => `<button class="${S.speedSpan === s ? "on" : ""}" data-span="${s}">${l}</button>`).join("")}</div>`, "k-speed")}
      <div class="det-b" data-keep-scroll="speed"><canvas class="big-chart" id="big"></canvas>
      <div class="rlist" style="margin-top:8px" data-live="speed-d"></div>
      <p class="muted" style="font-size:11.5px">图表从打开窗口时开始记录；下面是今日按路由的流量。</p></div></div>`,
  };
}

function appsViews(): Views {
  return {
    normal: `<div class="pad now-pad"><span class="hint">展开</span><div class="kicker" data-m="k-apps">谁在用</div>
      <div class="apps" data-live="apps"></div>
      <div class="foot"><span data-live="apps-foot"></span><span>点开查看全部</span></div></div>`,
    compact: (v) => `<div data-live="apps-c" data-variant="${v}" style="height:100%"></div>`,
    detail: `<div class="det">${detHead("谁在用", "应用", `<button class="btn" data-goto-activity>在活动中查看 →</button>`, "k-apps")}<div class="det-b" data-keep-scroll="apps"><div class="rlist" data-live="apps-d"></div></div></div>`,
  };
}

function eventsViews(): Views {
  const p = S.ev.paused;
  return {
    normal: `<div class="pad now-pad"><span class="hint">展开</span><div class="kicker" data-m="k-events">刚刚</div><div class="ticker" data-live="ticker" data-n="5"></div></div>`,
    compact: (v) => `<div data-live="events-c" data-variant="${v}" style="height:100%"></div>`,
    detail: `<div class="det">${detHead("刚刚", "实时事件", `<span class="live ${p ? "paused" : ""}"><i></i>${p ? "已暂停" : "实时"}</span><button class="btn" data-ev-pause>${p ? "继续" : "暂停"}</button><button class="btn" data-goto-log>完整日志 →</button>`, "k-events")}
      <div class="det-b" data-keep-scroll="events"><div class="ticker big" data-live="ticker" data-n="60"></div></div></div>`,
  };
}

const VIEWS: Record<string, () => Views> = { hero: heroViews, map: mapViews, mode: modeViews, takeover: takeoverViews, routes: routesViews, speed: speedViews, apps: appsViews, events: eventsViews };

// ---------------------------------------------------------------- node picker

function autoSeg(r: RouteView): string {
  return `<div class="seg"><button class="${r.auto ? "on" : ""}" data-auto-set="1" data-route="${esc(r.id)}">自动</button><button class="${r.auto ? "" : "on"}" data-auto-set="0" data-route="${esc(r.id)}">手动</button></div>`;
}
const delayKey = (n: NodeView) => (!n.tested ? 1e7 : n.delay > 0 ? n.delay : 1e6);

function nodePicker(r: RouteView): string {
  const nodes = usableNodes();
  if (!nodes.length) return `<div class="empty"><b>还没有节点</b><span>添加一个 Provider 后，这里会列出它的全部节点。</span><button class="pill sm" data-page="providers">添加 Provider</button></div>`;
  const testing = busy("test:" + r.id);
  const counts = new Map<string, { n: number; flag: string }>();
  let unknown = 0;
  for (const n of nodes) {
    const p = n.where.place;
    if (!p) { unknown++; continue; }
    const c = counts.get(p.country) ?? { n: 0, flag: p.flag };
    c.n++;
    counts.set(p.country, c);
  }
  const regions: [string, string][] = [["all", "全部"], ...[...counts].sort((a, b) => b[1].n - a[1].n).map(([c, v]): [string, string] => [c, `${esc(v.flag)} ${esc(countryName(c))}`])];
  if (unknown) regions.push(["?", "◌ 未知"]);
  const q = S.nd.q.trim().toLowerCase();
  const inRegion = (n: NodeView) => S.nd.region === "all" || (S.nd.region === "?" ? !n.where.place : n.where.place?.country === S.nd.region);
  const hay = (n: NodeView) => `${n.display} ${n.name} ${n.where.place?.city ?? ""} ${n.where.place ? countryName(n.where.place.country) : ""} ${n.type} ${provider(n.provider)?.name ?? ""}`.toLowerCase();
  const list = nodes.filter((n) => inRegion(n) && (!q || hay(n).includes(q))).sort((a, b) => delayKey(a) - delayKey(b) || a.display.localeCompare(b.display));
  const switching = busy("node:" + r.id);
  return `<div class="toolrow"><div class="chips">${regions.map(([id, l]) => `<button class="${S.nd.region === id ? "on" : ""}" data-region="${esc(id)}">${l}</button>`).join("")}</div>
      <div class="right"><input class="search" id="nd-q" placeholder="搜索节点" value="${esc(S.nd.q)}" autocomplete="off" spellcheck="false" /><button class="btn" data-test="${esc(r.id)}">${testing ? '<span class="shimmer">测速中…</span>' : "全部测速"}</button></div></div>
    ${r.auto ? `<p class="muted pick-note">自动：选延迟最低的可用节点，连续失败会自动换下一个（◆ 为当前选中）。点任一节点即切换为手动。</p>` : ""}
    <div class="nodes">${list.length ? list.map((n, i) => {
      const on = r.node === n.name, sw = switching && on;
      return `<button class="nrow ${on ? "on" : ""}" data-pick data-route="${esc(r.id)}" data-node="${esc(n.name)}" style="--i:${Math.min(i, 14)}">
        <span class="fl">${flagOf(n)}</span>
        <span class="nmw"><span class="nm ${sw ? "shimmer" : ""}">${esc(n.display)}</span><span class="meta">${sw ? "正在切换…" : `${cityOf(n)} ${locTag(n, true)} · ${esc(n.type)}${n.udp ? "" : " · 无 UDP"} · ${provName(n)}`}</span></span>
        <span class="bars">${testing ? `<span class="skel" style="width:60px;height:12px;display:block"></span>` : stab(n.history, 10, 14)}</span>
        <span class="ms num" style="color:${testing ? "var(--mute)" : tone(n.delay, n.tested)}">${testing ? "…" : delayText(n.delay, n.tested)}</span>
        <span class="ck">${on ? (r.auto ? "◆" : "✓") : ""}</span></button>`;
    }).join("") : `<div class="empty"><span>没有符合条件的节点</span></div>`}</div>`;
}

// ---------------------------------------------------------------- live slots (Tick)

const routeColor = (id: string) => { const r = route(id); return r ? `var(${esc(r.color)})` : "var(--faint)"; };
const speedHTML = () => {
  const off = !takeoverOn();
  const [dv, du] = off ? ["0", "KB"] : unit(S.tick.down), [uv, uu] = off ? ["0", "KB"] : unit(S.tick.up);
  return `<span class="v num">↓ ${dv}<small>${du}/s</small></span><span class="v u num">↑ ${uv}<small>${uu}/s</small></span>`;
};
let freshEvent = "";
const evKey = (e: { time: string; text: string }) => e.time + e.text;
function eventsNow() { return S.ev.paused && S.ev.frozen ? S.ev.frozen : S.tick.events; }
function tickerHTML(n: number): string {
  if (!takeoverOn()) return `<div class="tk"><span></span><span class="muted">没有经过 Moho 的流量</span></div>`;
  const evs = eventsNow().slice(0, n);
  if (!evs.length) return `<div class="tk"><span></span><span class="muted">还没有新的连接</span></div>`;
  return evs.map((e, i) => {
    const r = route(e.route), where = r?.direct || (!e.node && !r) ? "直连" : [r?.name, e.node].filter(Boolean).join(" · ");
    return `<div class="tk ${i === 0 && evKey(e) === freshEvent ? "new" : ""} ${e.failed ? "bad" : ""}"><span class="num">${minSec(e.time)}</span><span>${esc(e.text)} <span class="w">· ${esc(where)}${e.failed ? " 失败" : ""}</span></span></div>`;
  }).join("");
}
function appRows(n: number): string {
  const apps = S.tick.apps.slice(0, n), max = Math.max(1, ...S.tick.apps.map((a) => a.down));
  if (!takeoverOn() || !apps.length) return `<div class="app"><span class="an muted">${takeoverOn() ? "现在没有应用在用网络" : "没有经过 Moho 的流量"}</span></div>`;
  return apps.map((a) => {
    const r = route(a.route), on = !r || active(r);
    return `<div class="app" style="opacity:${on ? 1 : 0.35}"><span class="an"><span data-m="app-${esc(a.app)}">${esc(a.app)}</span><small>${esc(a.host)}</small></span><span class="bar"><i style="width:${(a.down / max) * 100}%;background:${routeColor(a.route)}"></i></span><span class="av num">${fmt(a.down, "/s")}</span></div>`;
  }).join("");
}

const LIVE: Record<string, (el: HTMLElement) => string> = {
  speed: speedHTML,
  "speed-foot": () => `今日 ${fmt(S.tick.todayDown)} · 本月 ${fmt(S.tick.monthDown)}`,
  "speed-c": () => `↓ ${fmt(takeoverOn() ? S.tick.down : 0, "/s")}`,
  "speed-c-sub": () => `↑ ${fmt(takeoverOn() ? S.tick.up : 0, "/s")} · 今日 ${fmt(S.tick.todayDown)}`,
  "speed-d": () => routes().map((r) => `<div class="rr flat" style="grid-template-columns:10px 90px 1fr 100px 100px 60px">${sw(r, 10)}<span class="rn" style="font-size:14px">${esc(r.name)}</span><span class="bar"><i style="width:${r.share * 100}%;background:var(${esc(r.color)})"></i></span><span class="num">↓ ${fmt(S.tick.todayDown * r.share)}</span><span class="num muted">${fmt(S.tick.byRoute[r.id] ?? 0, "/s")}</span><span class="num muted">${Math.round(r.share * 100)}%</span></div>`).join(""),
  apps: () => appRows(4),
  "apps-foot": () => `${S.tick.active} 个连接`,
  "apps-c": (el) => {
    const v = (el.dataset.variant ?? "mini") as Variant, a0 = S.tick.apps[0];
    if (!a0 || !takeoverOn()) return cmp(v, "apps", "谁在用", "—", { sub: `${S.tick.active} 个连接` });
    return cmp(v, "apps", "谁在用", esc(a0.app), {
      key: `app-${a0.app}`, sub: `${fmt(a0.down, "/s")} · 共 ${S.tick.active} 个连接`,
      list: S.tick.apps.slice(1, 4).map((a) => `<span>${esc(a.app)}　<span class="muted num">${fmt(a.down, "/s")}</span></span>`).join(""),
    });
  },
  "apps-d": () => {
    const apps = S.tick.apps, max = Math.max(1, ...apps.map((a) => a.down));
    if (!apps.length) return `<div class="empty"><span>现在没有应用在用网络</span></div>`;
    return apps.map((a) => `<button class="rr" style="grid-template-columns:1fr 1.4fr 140px 90px 70px 14px" data-goapp="${esc(a.app)}">
      <span class="rn" style="font-size:15px" data-m="app-${esc(a.app)}">${esc(a.app)}</span><span class="rules">${esc(a.host)}</span>
      <span class="bar"><i style="width:${(a.down / max) * 100}%;background:${routeColor(a.route)}"></i></span>
      <span class="num">${fmt(a.down, "/s")}</span><span class="muted num">${a.conns} 连接</span><span class="muted">›</span></button>`).join("");
  },
  ticker: (el) => tickerHTML(Number(el.dataset.n) || 5),
  "events-c": (el) => {
    const v = (el.dataset.variant ?? "mini") as Variant, evs = eventsNow(), e0 = evs[0];
    return cmp(v, "events", "刚刚", v === "strip" && e0 ? esc(e0.text) : "实时", {
      short: "实时",
      sub: v === "strip" && e0 ? esc([route(e0.route)?.name, e0.node].filter(Boolean).join(" · ") || "直连") : "",
      list: evs.slice(0, 4).map((e) => `<span>${esc(e.text)}</span>`).join(""),
    });
  },
};
function fillLive(root: ParentNode): void {
  root.querySelectorAll<HTMLElement>("[data-live]").forEach((el) => {
    const fn = LIVE[el.dataset.live ?? ""];
    if (fn) setHTML(el, fn(el));
  });
}

// ---------------------------------------------------------------- bars

function drawBars(c: HTMLCanvasElement | null, span: number, bars: number, gap: number): void {
  if (!c || !c.clientWidth) return;
  const dpr = devicePixelRatio || 1, w = c.clientWidth, h = c.clientHeight;
  c.width = Math.round(w * dpr); c.height = Math.round(h * dpr);
  const g = c.getContext("2d");
  if (!g) return;
  g.scale(dpr, dpr);
  const hair = cssVar("--hair");
  if (!takeoverOn()) { g.fillStyle = hair; g.fillRect(0, h - 1, w, 1); return; }
  const now = Date.now(), bucket = (span * 1000) / bars;
  const sums: Record<string, number>[] = Array.from({ length: bars }, () => ({}));
  const counts = new Array<number>(bars).fill(0);
  for (const s of S.hist) {
    const i = bars - 1 - Math.floor((now - s.t) / bucket);
    if (i < 0 || i >= bars) continue;
    counts[i]!++;
    for (const [k, v] of Object.entries(s.by)) sums[i]![k] = (sums[i]![k] ?? 0) + v;
  }
  const order = [...routes()].reverse().filter((r) => active(r));
  const avg = sums.map((b, i) => (counts[i] ? Object.fromEntries(Object.entries(b).map(([k, v]) => [k, v / counts[i]!])) : {}));
  const totals = avg.map((b) => order.reduce((a, r) => a + (b[r.id] ?? 0), 0));
  const max = Math.max(1024, ...totals) * 1.05, bw = w / bars;
  const colorOf = Object.fromEntries(order.map((r) => [r.id, cssVar(r.color) || cssVar("--ink")]));
  g.fillStyle = hair; g.fillRect(0, h - 1, w, 1);
  avg.forEach((b, i) => {
    let y = h;
    for (const r of order) {
      const bh = ((b[r.id] ?? 0) / max) * h;
      if (bh <= 0) continue;
      g.fillStyle = colorOf[r.id]!; g.globalAlpha = 0.85;
      g.fillRect(i * bw + 0.5, y - bh, Math.max(0.5, bw - gap - 0.5), bh);
      y -= bh;
    }
  });
  g.globalAlpha = 1;
}
function drawCharts(): void {
  drawBars($<HTMLCanvasElement>("spark"), 60, 60, 0.9);
  if (S.x === "speed") drawBars($<HTMLCanvasElement>("big"), S.speedSpan, 90, 1);
}

// ---------------------------------------------------------------- board

const viewCache = new Map<string, string>();
function setView(el: HTMLElement, key: string, html: string): void {
  if (viewCache.get(key) === html && el.innerHTML) return;
  if (typingIn(el)) return;
  const scrollers = [...el.querySelectorAll<HTMLElement>("[data-keep-scroll]")].map((s) => [s.dataset.keepScroll, s.scrollTop] as const);
  const active = document.activeElement as HTMLInputElement | null;
  const focused = active && el.contains(active) && active.id ? active.id : null;
  const pos = focused ? active!.selectionStart : null;
  el.innerHTML = html;
  viewCache.set(key, html);
  for (const [k, top] of scrollers) { if (!top) continue; const s = el.querySelector<HTMLElement>(`[data-keep-scroll="${k}"]`); if (s) s.scrollTop = top; }
  if (focused) { const i = document.getElementById(focused) as HTMLInputElement | null; if (i) { i.focus({ preventScroll: true }); if (pos != null) i.setSelectionRange(pos, pos); } }
}
/** Forget the cached markup, e.g. after prep() moved data-m keys around. */
const invalidate = () => viewCache.clear();

let resizeObs: ResizeObserver | null = null;
function buildBoard(page: HTMLElement): void {
  viewCache.clear();
  page.innerHTML = `<div class="board" id="board">${CELLS.map((c) => `
    <section class="cell" data-cell="${c.id}" style="grid-area:${c.area}">
      ${c.id === "map" ? '<canvas class="map-cv" id="map"></canvas>' : ""}
      <div class="vw v-normal"></div><div class="vw v-compact"></div><div class="vw v-detail"></div>
    </section>`).join("")}</div>`;
  const b = $("board")!;
  const map = $<HTMLCanvasElement>("map");
  if (map) startMap(map);
  resizeObs?.disconnect();
  let last = "";
  resizeObs = new ResizeObserver(() => {
    const k = `${b.clientWidth}x${b.clientHeight}`;
    if (k !== last) { last = k; updateBoard(); }
  });
  resizeObs.observe(b);
}

function updateBoard(): void {
  const b = $("board");
  if (!b) return;
  const W = b.clientWidth, H = b.clientHeight;
  const final = trackArrays(W, H, takeoverOn() ? S.x : null), normal = trackArrays(W, H, null);
  const t = (a: number[]) => a.map((v) => `minmax(0, ${Math.max(0, v).toFixed(2)}fr)`).join(" ");
  b.classList.toggle("off", !takeoverOn());
  b.style.gridTemplateColumns = t(final.cols);
  b.style.gridTemplateRows = t(final.rows);
  for (const c of CELLS) {
    const el = b.querySelector<HTMLElement>(`[data-cell="${c.id}"]`)!;
    const prev = el.dataset.v, nv = viewOf(c);
    const views = { normal: el.querySelector<HTMLElement>(".v-normal")!, compact: el.querySelector<HTMLElement>(".v-compact")!, expanded: el.querySelector<HTMLElement>(".v-detail")! };
    const [w, h] = sizeIn(c, final, W, H), [nw, nh] = sizeIn(c, normal, W, H);
    const V = VIEWS[c.id]!();
    if (c.id !== "map") {
      // Fixed sizes: the normal view at its normal size, the active one at its final size.
      Object.assign(views.normal.style, { width: nw + "px", height: nh + "px" });
      if (nv !== "normal") Object.assign(views[nv].style, { width: w + "px", height: h + "px" });
    } else views.normal.style.inset = "0";
    setView(views.normal, c.id + ":n", V.normal);
    if (nv === "compact") setView(views.compact, c.id + ":c", c.id === "map" ? "" : V.compact(variantOf(w, h)));
    if (nv === "expanded") setView(views.expanded, c.id + ":d", V.detail);
    el.dataset.v = nv;
    if (prev && prev !== nv) {
      // The old view folds up like a drawer at its own size; the new one opens in place.
      const out = views[prev as keyof typeof views], inn = views[nv];
      out.classList.remove("entering"); out.classList.add("leaving");
      clearTimeout((out as any)._t);
      (out as any)._t = setTimeout(() => {
        out.classList.remove("leaving");
        if (prev !== "normal" && el.dataset.v !== prev) { out.innerHTML = ""; viewCache.delete(c.id + (prev === "compact" ? ":c" : ":d")); }
      }, 230);
      inn.classList.remove("leaving", "entering"); void inn.offsetWidth; inn.classList.add("entering");
      clearTimeout((inn as any)._t);
      (inn as any)._t = setTimeout(() => inn.classList.remove("entering"), 1000);
    }
  }
  fillLive(b);
  drawCharts();
  if (S.x === "speed") setTimeout(drawCharts, 380);
}

// ---------------------------------------------------------------- entry points

export function renderHome(page: HTMLElement, withMorph: boolean, prep?: () => void): void {
  if (!$("board")) { buildBoard(page); updateBoard(); return; }
  if (withMorph) morph(page, $("board"), () => { if (prep) invalidate(); updateBoard(); }, prep);
  else updateBoard();
}
export function leaveHome(): void {
  stopMap();
  resizeObs?.disconnect();
  resizeObs = null;
}
/** A new Tick: only the live slots and the charts change. */
export function tickHome(): void {
  const top = S.tick.events[0];
  if (top && !S.ev.paused) freshEvent = evKey(top) === lastTop ? "" : evKey(top);
  lastTop = top ? evKey(top) : "";
  const b = $("board");
  if (!b) return;
  fillLive(b);
  drawCharts();
}
let lastTop = "";

function expand(id: string, sub: string | null = null): void {
  if (!takeoverOn()) return;
  if (id === "hero" && !hasNodes()) return;
  if (S.x === id && S.sub === sub) return;
  S.x = id; S.sub = sub; S.nd = { region: "all", q: "" };
  rerender(true);
}
export function collapse(): boolean {
  if (!S.x) return false;
  S.x = null; S.sub = null;
  rerender(true);
  return true;
}

function pickNode(rid: string, name: string, rowEl: HTMLElement): void {
  const r = route(rid);
  if (!r || (r.node === name && !r.auto)) return;
  const prev = { node: r.node, auto: r.auto };
  void act("node:" + rid, () => api.Routes.select(rid, name), {
    optimistic: () => { S.over.routes[rid] = { node: name, auto: false }; },
    undo: () => { S.over.routes[rid] = prev; },
    morph: true,
    prep: () => {
      // The clicked name becomes the title: hand the shared identity over.
      const key = rid === defaultRoute()?.id ? "exit-default" : `exit-${rid}`;
      document.querySelectorAll(`[data-m="${key}"]`).forEach((e) => e.removeAttribute("data-m"));
      const nm = rowEl.querySelector<HTMLElement>(".nm");
      if (nm) nm.dataset.m = key;
    },
  });
}
function setAuto(rid: string, auto: boolean): void {
  const r = route(rid);
  if (!r || r.auto === auto) return;
  if (!auto && !r.node) return;
  void act("node:" + rid, () => api.Routes.select(rid, auto ? "" : r.node), {
    optimistic: () => { S.over.routes[rid] = { node: r.node, auto }; },
    morph: true,
  });
}
function toggleEntry(id: "sys" | "tun"): void {
  const tk = S.snap.takeover, e = id === "sys" ? tk.system : tk.tun;
  if (busy("entry:" + id)) return;
  if (id === "sys" && e.status === "hijacked") { void act("retake", () => api.Control.retake()); return; }
  const on = !e.wanted;
  void act("entry:" + id, () => (id === "sys" ? api.Control.setSystemProxy(on) : api.Control.setTun(on)));
}
/** Off: take over with one way in. */
function choose(id: "sys" | "tun"): void {
  if (busy("entry:" + id)) return;
  const e = id === "sys" ? S.snap.takeover.system : S.snap.takeover.tun;
  void act("entry:" + id, async () => {
    if (!e.wanted) await (id === "sys" ? api.Control.setSystemProxy(true) : api.Control.setTun(true));
    await api.Control.setTakeover(true);
  });
}
function setMode(m: string): void {
  if (mode() === m || busy("mode")) return;
  void act("mode", () => api.Control.setMode(m), {
    optimistic: () => { S.over.mode = m; S.over.modeTarget = m; },
    undo: () => { S.over.mode = null; S.over.modeTarget = null; },
    morph: true,
  });
}

/** Clicks on the board. Returns true when handled. */
export function homeClick(t: HTMLElement): boolean {
  const d = t.dataset;
  if (d.expand) { expand(d.expand, d.sub ?? null); return true; }
  if (d.collapse !== undefined) { collapse(); return true; }
  if (d.pick !== undefined && d.route && d.node) { pickNode(d.route, d.node, t); return true; }
  if (d.autoToggle) { const r = route(d.autoToggle); if (r) setAuto(r.id, !r.auto); return true; }
  if (d.autoSet && d.route) { setAuto(d.route, d.autoSet === "1"); return true; }
  if (d.region) { S.nd.region = d.region; rerender(true); return true; }
  if (d.test) { if (!busy("test:" + d.test)) void act("test:" + d.test, () => api.Routes.test(d.test!)); return true; }
  if (d.locate) { const n = d.locate; void act("locate:" + n, () => api.Routes.locate(n)); return true; }
  if (d.entry) { toggleEntry(d.entry as "sys" | "tun"); return true; }
  if (d.entryToggle) { const id = d.entryToggle as "sys" | "tun", e = id === "sys" ? S.snap.takeover.system : S.snap.takeover.tun; if (!busy("entry:" + id)) void act("entry:" + id, () => (id === "sys" ? api.Control.setSystemProxy(!e.wanted) : api.Control.setTun(!e.wanted))); return true; }
  if (d.choose) { choose(d.choose as "sys" | "tun"); return true; }
  if (d.retake !== undefined) { if (!busy("retake")) void act("retake", () => api.Control.retake()); return true; }
  if (d.lan !== undefined) { if (!busy("lan")) { const on = !S.snap.takeover.lan; void act("lan", () => api.Control.setLan(on)); } return true; }
  if (d.pauseTakeover !== undefined) { if (!busy("takeover")) void act("takeover", () => api.Control.setTakeover(false)); return true; }
  if (d.mode) { setMode(d.mode); return true; }
  if (d.subOpen) { S.nd = { region: "all", q: "" }; S.sub = d.subOpen; rerender(true); return true; }
  if (d.subBack !== undefined) { S.sub = null; rerender(true); return true; }
  if (d.span) { S.speedSpan = Number(d.span); rerender(false); drawCharts(); return true; }
  if (d.evPause !== undefined) { S.ev.paused = !S.ev.paused; S.ev.frozen = S.ev.paused ? [...S.tick.events] : null; rerender(false); tickHome(); return true; }
  return false;
}
/** A click on a cell's background expands it; on the expanded map, collapses it. */
export function cellClick(target: HTMLElement): void {
  const cell = target.closest<HTMLElement>("[data-cell]");
  if (!cell || target.closest("input,select,textarea,.det-b")) return;
  if (cell.dataset.v === "expanded") { if (cell.dataset.cell === "map") collapse(); return; }
  expand(cell.dataset.cell!);
}
export function homeInput(t: HTMLInputElement): boolean {
  if (t.id === "nd-q") { S.nd.q = t.value; rerender(false); return true; }
  return false;
}
