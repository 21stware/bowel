// Provider: where nodes come from. A paste box on top (detected as you
// type), the list below; a row opens to its nodes.

import { api } from "./api";
import type { NodeView, ProviderView } from "./mygo";
import { ago, delayText, esc, expireText, gb, tone } from "./format";
import { S, act, busy, rerender, routes } from "./store";
import { $, errorText, setHTML, toast } from "./ui";

const KIND: Record<string, string> = { subscription: "订阅", link: "节点链接", file: "本地文件" };
const HOURS = [0, 6, 12, 24, 48, 72];

const nodesOf = (p: ProviderView): NodeView[] => S.snap.nodes.filter((n) => n.provider === p.id);
const attention = (p: ProviderView) => !!p.warn || !!p.error;

function kindText(p: ProviderView, ns: NodeView[]): string {
  if (p.kind === "link") {
    const types = [...new Set(ns.map((n) => n.type.toLowerCase()))];
    return `节点链接${types.length ? " · " + esc(types.join(" / ")) : ""}`;
  }
  return KIND[p.kind] ?? esc(p.kind);
}

function head(): string {
  const ps = S.snap.providers, n = S.snap.nodes.length, warn = ps.filter(attention).length;
  const subs = ps.filter((p) => p.kind === "subscription" && p.enabled).length;
  const all = busy("providers");
  return `<div><div class="kicker">PROVIDER · 节点从哪里来</div><h2>${ps.length ? `${ps.length} 个来源` : "还没有来源"}<small class="num">${n} 个节点${warn ? ` · ${warn} 个需要关注` : ""}</small></h2></div>
    <div class="right">${subs ? `<button class="btn" data-updall>${all ? `<span class="shimmer">正在更新 ${subs} 个订阅…</span>` : "全部更新"}</button>` : ""}</div>`;
}

const HINT = "自动识别：订阅（Clash / mihomo / Base64）、单个或多个节点链接、YAML 配置。在任何页面按 ⌘V 粘贴链接也会来到这里。";
function detectLine(): string {
  const p = S.prov;
  if (!p.paste.trim()) return HINT;
  if (p.detecting && !p.detect) return `<span class="shimmer">正在识别…</span>`;
  return p.detect || HINT;
}
function describe(kind: string, text: string): string {
  const links = text.split(/\s+/).filter((l) => /^[a-z0-9+-]+:\/\//i.test(l) && !/^https?:/i.test(l));
  const scheme = links[0]?.split("://")[0]?.toUpperCase();
  switch (kind) {
    case "subscription": return `识别为 <b>订阅</b>，添加后读取节点、流量和到期时间。`;
    case "link": return links.length > 1
      ? `识别为 <b>${links.length} 个节点链接</b>，会作为一个 Provider 加入；添加后实测出口位置。`
      : `识别为 <b>节点链接${scheme ? " · " + esc(scheme) : ""}</b>，会作为一个单独的 Provider 加入；添加后实测出口位置。`;
    case "file": return `识别为 <b>配置内容</b>（Clash YAML），会作为本地 Provider 加入。`;
    default: return `识别为 <b>${esc(kind)}</b>。`;
  }
}
function addButtons(): string {
  const adding = busy("add");
  return `<button class="btn" data-addfile>选择文件…</button><button class="pill sm" data-add ${S.prov.paste.trim() ? "" : "disabled"}>${adding ? '<span class="shimmer-inv">添加中…</span>' : "添加"}</button>`;
}

function usageCell(p: ProviderView): string {
  const u = p.usage;
  if (u && u.total > 0) {
    const used = u.upload + u.download, pct = Math.min(100, (used / u.total) * 100);
    return `<div class="q ${p.warn ? "warn" : ""}"><span class="num">已用 <b>${gb(used)}</b> / ${gb(u.total)} GB</span><span class="bar"><i style="width:${pct}%;background:var(--ink)"></i></span><span>${expireText(u.expire)}</span></div>`;
  }
  if (u && u.expire) return `<div class="q ${p.warn ? "warn" : ""}"><span>不限流量</span><span>${expireText(u.expire)}</span></div>`;
  return `<div class="q"><span>${p.kind === "subscription" ? "没有用量信息" : "不限流量 · 不过期"}</span></div>`;
}
function histogram(ns: NodeView[]): string {
  if (!ns.length) return `<div class="hl"></div>`;
  const ds = ns.slice().sort((a, b) => (a.tested ? (a.delay || 9e5) : 1e6) - (b.tested ? (b.delay || 9e5) : 1e6)).slice(0, 40);
  return `<div class="hl" title="各节点延迟，从快到慢">${ds.map((n) => !n.tested
    ? `<i style="height:3px;background:var(--faint)"></i>`
    : `<i style="height:${n.delay <= 0 ? 20 : Math.max(3, 20 - n.delay / 12)}px;background:${tone(n.delay)};opacity:.7"></i>`).join("")}</div>`;
}
function autoCell(p: ProviderView): string {
  if (p.kind !== "subscription") return `<div class="q"><span>—</span></div>`;
  const opts = HOURS.includes(p.autoUpdate) ? HOURS : [...HOURS, p.autoUpdate].sort((a, b) => a - b);
  return `<div class="q"><span>自动更新</span><select class="sel" data-autoupd="${esc(p.id)}">${opts.map((h) => `<option value="${h}" ${p.autoUpdate === h ? "selected" : ""}>${h ? `每 ${h} 小时` : "关闭"}</option>`).join("")}</select></div>`;
}

function locTag(n: NodeView): string {
  const s = n.where.source;
  if (s === "probe") return `<span class="loc probe">实测</span>`;
  if (s === "locating" || busy("locate:" + n.name)) return `<span class="loc unknown shimmer">定位中</span>`;
  return `<span class="loc ${s === "name" ? "name" : "unknown"}" data-locate="${esc(n.name)}" title="经这个节点实测出口位置">${s === "name" ? "按名称" : "未知"}</span>`;
}

function row(p: ProviderView): string {
  const ns = nodesOf(p), open = S.prov.open === p.id;
  const upd = busy("provider:" + p.id) || (busy("providers") && p.kind === "subscription" && p.enabled);
  const renaming = S.prov.renaming === p.id;
  const updated = ago(p.updatedAt);
  const name = renaming
    ? `<input class="rename" id="prn" value="${esc(S.prov.draft)}" autocomplete="off" spellcheck="false" />`
    : `<b class="${upd ? "shimmer" : ""}">${esc(p.name)}</b>`;
  const sub = upd ? "正在更新…" : `${kindText(p, ns)} · ${p.nodes} 个节点${updated ? ` · ${updated}更新` : ""}${p.enabled ? "" : " · 已停用"}`;
  const armed = S.prov.armed === p.id;
  const used = (n: NodeView) => routes().filter((r) => r.node === n.name);
  return `<div class="prow ${open ? "open" : ""} ${p.enabled ? "" : "off"}" data-prov="${esc(p.id)}">
      <div class="pn">${name}<small>${sub}</small>${p.error ? `<small class="err">${esc(p.error)}</small>` : ""}</div>
      ${usageCell(p)}
      ${histogram(ns)}
      ${autoCell(p)}
      <div class="ops">${p.kind === "subscription" ? `<button class="btn" data-upd="${esc(p.id)}" ${upd ? "disabled" : ""}>更新</button>` : ""}<button class="btn" data-provoff="${esc(p.id)}">${p.enabled ? "停用" : "启用"}</button><span class="muted">${open ? "▴" : "▾"}</span></div>
    </div>
    ${open ? `<div class="pnodes">
      <div class="pn-row h"><span></span><span>节点</span><span>出口位置</span><span>协议</span><span>被哪些路由使用</span><span style="text-align:right">延迟</span></div>
      ${ns.length ? ns.map((n) => `<div class="pn-row"><span>${esc(n.where.place?.flag || "◌")}</span><b>${esc(n.display)}</b><span>${esc(n.where.place?.city || "位置未知")} ${locTag(n)}</span><span class="muted">${esc(n.type)}${n.udp ? "" : " · 无 UDP"}</span>
        <span>${used(n).map((r) => `<span class="sw" style="background:var(${esc(r.color)})"></span> ${esc(r.name)}`).join("　") || '<span class="muted">—</span>'}</span>
        <span class="num" style="text-align:right;color:${tone(n.delay, n.tested)}">${delayText(n.delay, n.tested)}</span></div>`).join("")
        : `<div class="pn-row"><span></span><span class="muted">${p.enabled ? "这个来源没有可用的节点" : "已停用：它的节点不会被使用"}</span></div>`}
      <div class="pfoot muted">来源：<span class="mono">${esc(p.source || "—")}</span>
        <span class="right">${renaming
          ? `<button class="btn" data-rename-save>保存</button><button class="btn" data-rename-cancel>取消</button>`
          : `<button class="btn" data-rename="${esc(p.id)}">重命名</button>`}
        <button class="btn danger ${armed ? "armed" : ""}" data-del="${esc(p.id)}">${busy("provider:" + p.id) && armed ? '<span class="shimmer">删除中…</span>' : armed ? "确认删除？" : "删除"}</button></span></div>
    </div>` : ""}`;
}

function list(): string {
  const ps = S.snap.providers;
  if (!ps.length) return `<div class="empty big"><b>还没有 Provider</b><span>把机场给的订阅链接、单个节点链接（vmess:// trojan:// hysteria2:// …）或 Clash 配置粘贴到上面，或者选择一个配置文件。</span></div>`;
  return ps.map(row).join("");
}

export function renderProviders(page: HTMLElement): void {
  if (!$("prov-root")) {
    page.innerHTML = `<div class="page-in" id="prov-root"><div class="p-head" id="prov-head"></div>
      <div class="addbar">
        <div class="row"><input id="addp" placeholder="粘贴订阅链接，或 vmess:// trojan:// hysteria2:// vless:// 等节点链接" autocomplete="off" spellcheck="false" /><span class="addbtns" id="addbtns"></span></div>
        <div class="detect" id="detect"></div>
      </div>
      <div class="plist" id="plist"></div></div>`;
    const input = $<HTMLInputElement>("addp")!;
    input.value = S.prov.paste;
  }
  const input = $<HTMLInputElement>("addp")!;
  if (document.activeElement !== input && input.value !== S.prov.paste) input.value = S.prov.paste;
  setHTML($("prov-head")!, head());
  setHTML($("addbtns")!, addButtons());
  setHTML($("detect")!, detectLine());
  setHTML($("plist")!, list());
  if (S.prov.renaming) { const r = $<HTMLInputElement>("prn"); if (r && document.activeElement !== r) { r.focus(); r.select(); } }
}

// ---------------------------------------------------------------- actions

let detectTimer = 0, detectSeq = 0;
/** Fill the paste box (e.g. from ⌘V anywhere) and detect what it is. */
export function setPaste(text: string): void {
  S.prov.paste = text;
  S.prov.detect = "";
  clearTimeout(detectTimer);
  const v = text.trim();
  if (!v) { S.prov.detecting = false; rerender(); return; }
  S.prov.detecting = true;
  rerender();
  const seq = ++detectSeq;
  detectTimer = window.setTimeout(() => {
    api.Providers.detect(v).then(
      (kind) => { if (seq === detectSeq) { S.prov.detect = describe(kind, v); S.prov.detecting = false; rerender(); } },
      (err) => { if (seq === detectSeq) { S.prov.detect = `<span class="err">无法识别：${esc(errorText(err))}</span>`; S.prov.detecting = false; rerender(); } },
    );
  }, 250);
}

async function add(): Promise<void> {
  const text = S.prov.paste.trim();
  if (!text || busy("add")) return;
  const p = await act("add", () => api.Providers.add(text));
  if (!p) return;
  S.prov.paste = ""; S.prov.detect = ""; S.prov.open = p.id;
  const input = $<HTMLInputElement>("addp");
  if (input) input.value = "";
  toast(`已添加「${p.name}」· ${p.nodes} 个节点`, "info");
  rerender();
}
export async function addFile(): Promise<void> {
  if (busy("add")) return;
  const p = await act("add", () => api.Providers.addFile());
  if (!p) return;
  S.prov.open = p.id;
  toast(`已添加「${p.name}」· ${p.nodes} 个节点`, "info");
  rerender();
}
function saveRename(): void {
  const id = S.prov.renaming, name = S.prov.draft.trim();
  S.prov.renaming = null;
  if (!id) return;
  const p = S.snap.providers.find((x) => x.id === id);
  if (!name || !p || name === p.name) { rerender(); return; }
  void act("provider:" + id, () => api.Providers.rename(id, name));
}

let armTimer = 0;
export function providersClick(t: HTMLElement): boolean {
  const d = t.dataset;
  if (d.updall !== undefined) { if (!busy("providers")) void act("providers", () => api.Providers.updateAll()); return true; }
  if (d.upd) { const id = d.upd; if (!busy("provider:" + id)) void act("provider:" + id, () => api.Providers.update(id)); return true; }
  if (d.provoff) { const p = S.snap.providers.find((x) => x.id === d.provoff); if (p) void act("provider:" + p.id, () => api.Providers.setEnabled(p.id, !p.enabled)); return true; }
  if (d.add !== undefined) { void add(); return true; }
  if (d.rename) { const p = S.snap.providers.find((x) => x.id === d.rename); if (p) { S.prov.renaming = p.id; S.prov.draft = p.name; rerender(); } return true; }
  if (d.renameSave !== undefined) { saveRename(); return true; }
  if (d.renameCancel !== undefined) { S.prov.renaming = null; rerender(); return true; }
  if (d.del) {
    const id = d.del;
    clearTimeout(armTimer);
    if (S.prov.armed !== id) { S.prov.armed = id; armTimer = window.setTimeout(() => { S.prov.armed = null; rerender(); }, 3000); rerender(); return true; }
    void act("provider:" + id, () => api.Providers.delete(id)).then(() => { S.prov.armed = null; if (S.prov.open === id) S.prov.open = null; rerender(); });
    return true;
  }
  if (d.prov) { if (S.prov.renaming === d.prov) return true; S.prov.open = S.prov.open === d.prov ? null : d.prov; rerender(); return true; }
  return false;
}
export function providersInput(t: HTMLInputElement | HTMLSelectElement, kind: "input" | "change"): boolean {
  if (t.id === "addp" && kind === "input") { setPaste(t.value); return true; }
  if (t.id === "prn" && kind === "input") { S.prov.draft = t.value; return true; }
  if (kind === "change" && t instanceof HTMLSelectElement && t.dataset.autoupd) {
    const id = t.dataset.autoupd, h = Number(t.value);
    void act("provider:" + id, () => api.Providers.setAutoUpdate(id, h));
    return true;
  }
  return false;
}
export function providersKey(e: KeyboardEvent): boolean {
  const t = e.target as HTMLElement;
  if (e.isComposing) return false;
  if (t.id === "addp" && e.key === "Enter") { void add(); return true; }
  if (t.id === "prn" && e.key === "Enter") { saveRename(); return true; }
  if (t.id === "prn" && e.key === "Escape") { S.prov.renaming = null; rerender(); return true; }
  return false;
}
/** Text that looks like something to add as a provider. */
export const looksAddable = (text: string): boolean => /^(https?|vmess|vless|trojan|ss|ssr|hysteria2?|hy2|tuic|wireguard|anytls|socks5?|http):\/\//im.test(text.trim()) || /^\s*(proxies|proxy-providers)\s*:/m.test(text);
