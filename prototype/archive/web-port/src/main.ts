// Boots the page: loads the Snapshot and the Tick, follows their events,
// renders the shell (tabs, state pill, news line) and the current page,
// and routes clicks, input and keys to the pages.

import "./style.css";
import { isMyGo } from "mygo-runtime";
import { api, install } from "./api";
import { activityClick, activityInput, renderActivity, showApp, startActivity, stopActivity, tickActivity } from "./activity";
import { esc } from "./format";
import { cellClick, collapse, homeClick, homeInput, leaveHome, renderHome, tickHome } from "./home";
import { addFile, looksAddable, providersClick, providersInput, providersKey, renderProviders, setPaste } from "./providers";
import { S, act, applySnapshot, busy, normalizeTick, pushSample, setRenderer, type Page } from "./store";
import { $, errorText, setHTML, toast } from "./ui";

// ---------------------------------------------------------------- shell

const TABS: [Page, string][] = [["home", "今日"], ["activity", "活动"], ["providers", "Provider"]];
const WORD: Record<string, string> = { ok: "正常", degraded: "降级", error: "异常", off: "未接管" };

function renderShell(): void {
  const s = S.snap;
  const provWarn = s.providers.find((p) => p.warn || p.error);
  setHTML($("tabs")!, TABS.map(([id, l]) => `<button class="tab ${S.page === id ? "on" : ""}" data-page="${id}">${l}${id === "providers" && provWarn ? `<span class="dot" title="${esc(provWarn.error ? `「${provWarn.name}」更新失败` : provWarn.warn === "quota" ? `「${provWarn.name}」流量快用完了` : `「${provWarn.name}」快到期了`)}"></span>` : ""}</button>`).join(""));
  const core = s.core.state === "starting" ? "正在启动…" : s.core.state === "error" ? "内核异常" : "";
  setHTML($("tbr")!, `<span class="state-mini ${esc(s.state)}" title="${esc(`mihomo ${s.core.version || ""}`.trim())}"><i></i>${S.ready ? core || WORD[s.state] || s.state : "连接中…"}</span>`);
  const is = s.issue;
  setHTML($("breaking")!, is && is.text
    ? `<div class="breaking ${is.level === "error" ? "" : "warn"}"><span class="label">${is.level === "error" ? "异常" : "注意"}</span><span>${esc(is.text)}</span>${is.action ? `<button class="act" data-issue="${esc(is.action)}">${is.action === "retake" && busy("retake") ? '<span class="shimmer">正在重新接管…</span>' : esc(is.label || "处理")}</button>` : ""}</div>`
    : "");
}

let shown: Page | null = null;
function render(morph = false, prep?: () => void): void {
  renderShell();
  if (!S.ready) return; // nothing to show before the first Snapshot
  const page = $("page")!;
  if (shown !== S.page) {
    if (shown === "home") leaveHome();
    if (shown === "activity") stopActivity();
    page.innerHTML = "";
    page.dataset.page = S.page;
    shown = S.page;
    morph = false;
    if (S.page === "activity") startActivity();
  }
  if (S.page === "home") renderHome(page, morph, prep);
  else if (S.page === "activity") renderActivity(page);
  else renderProviders(page);
}

function go(page: Page): void {
  if (S.page === page) return;
  S.page = page;
  S.x = null; S.sub = null;
  render();
}

// ---------------------------------------------------------------- events

const KEYS = [
  "page", "issue", "addfile", "gotoActivity", "gotoLog", "goapp",
  // home
  "expand", "collapse", "pick", "autoToggle", "autoSet", "region", "test", "locate", "entry", "entryToggle", "choose", "retake", "lan", "pauseTakeover", "mode", "subOpen", "subBack", "span", "evPause",
  // activity
  "acttab", "astatus", "aroute", "sort", "kill", "conn", "pause", "closeall", "lv", "copy", "relatedLog", "logCopy", "logClear",
  // providers
  "updall", "upd", "provoff", "add", "rename", "renameSave", "renameCancel", "del", "prov",
];
function actionOf(target: HTMLElement): HTMLElement | null {
  for (let el: HTMLElement | null = target; el && el !== document.body; el = el.parentElement) {
    if (KEYS.some((k) => k in el!.dataset)) return el;
    if (el.dataset.cell) return null;
  }
  return null;
}

document.addEventListener("click", (e) => {
  const target = e.target as HTMLElement;
  if (target.closest("input, select, textarea, option")) return;
  const t = actionOf(target);
  if (!t) { if (S.page === "home") cellClick(target); return; }
  if ((t as HTMLButtonElement).disabled) return;
  const d = t.dataset;
  if (d.page) { go(d.page as Page); return; }
  if (d.issue) {
    if (d.issue === "retake") { if (!busy("retake")) void act("retake", () => api.Control.retake()); }
    else if (d.issue === "activity") go("activity");
    else if (d.issue === "providers") go("providers");
    return;
  }
  if (d.addfile !== undefined) { void addFile(); return; }
  if (d.gotoActivity !== undefined) { S.act.tab = "conn"; go("activity"); return; }
  if (d.gotoLog !== undefined) { S.act.tab = "log"; go("activity"); return; }
  if (d.goapp) { showApp(d.goapp); go("activity"); return; }
  if (S.page === "home" && homeClick(t)) return;
  if (S.page === "activity" && activityClick(t, e)) return;
  if (S.page === "providers" && providersClick(t)) return;
});

function onInput(e: Event, kind: "input" | "change"): void {
  const t = e.target as HTMLInputElement;
  if ((e as InputEvent).isComposing) return;
  if (S.page === "home" && kind === "input") homeInput(t);
  else if (S.page === "activity") activityInput(t);
  else if (S.page === "providers") providersInput(t, kind);
}
document.addEventListener("input", (e) => onInput(e, "input"));
document.addEventListener("change", (e) => { if ((e.target as HTMLElement).tagName === "SELECT") onInput(e, "change"); });
// After an input method commits, handle the final text once.
document.addEventListener("compositionend", (e) => setTimeout(() => onInput(e, "input"), 0));

document.addEventListener("keydown", (e) => {
  if (e.isComposing) return;
  if (S.page === "providers" && providersKey(e)) { e.preventDefault(); return; }
  if (e.key === "Escape") {
    // Handled here, so MyGo does not take Escape for the window.
    if (S.page === "home" && collapse()) { e.preventDefault(); return; }
    if (S.page === "activity" && S.act.open) { S.act.open = null; render(); e.preventDefault(); return; }
  }
  if (e.metaKey && !e.altKey && !e.ctrlKey && ["1", "2", "3"].includes(e.key)) {
    go(TABS[Number(e.key) - 1]![0]);
    e.preventDefault();
  }
});

// ⌘V anywhere with a link: go to Provider with it filled in.
document.addEventListener("paste", (e) => {
  const t = e.target as HTMLElement;
  if (t.closest("input, textarea, [contenteditable]")) return;
  const text = e.clipboardData?.getData("text") ?? "";
  if (!looksAddable(text)) return;
  e.preventDefault();
  go("providers");
  setPaste(text.trim());
  const i = $<HTMLInputElement>("addp");
  if (i) { i.value = text.trim(); i.focus(); }
});

// ---------------------------------------------------------------- boot

function onTick(t: Parameters<Parameters<typeof api.events.tick.on>[0]>[0]): void {
  S.tick = normalizeTick(t);
  pushSample(S.tick);
  if (S.page === "home") tickHome();
  else if (S.page === "activity") tickActivity();
}

async function boot(): Promise<void> {
  if (!isMyGo()) {
    // A plain browser (`bunx vite`): no Go side, so pretend.
    const { createMock } = await import("./mock");
    install(createMock());
    document.documentElement.classList.add("mock");
  }
  setRenderer(render);
  render();
  api.events.snapshot.on((s) => { applySnapshot(s); render(true); });
  api.events.tick.on(onTick);
  try {
    const [s, t] = await Promise.all([api.Moho.snapshot(), api.Moho.tick()]);
    applySnapshot(s);
    onTick(t);
  } catch (err) {
    toast(`读取状态失败：${errorText(err)}`);
  }
  render();
}
void boot();
