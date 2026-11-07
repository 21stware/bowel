// Small DOM helpers shared by the pages.

import { isCallError } from "mygo-runtime";

export const $ = <T extends HTMLElement = HTMLElement>(id: string): T | null => document.getElementById(id) as T | null;

export const cssVar = (name: string): string => getComputedStyle(document.documentElement).getPropertyValue(name).trim();

/** The message of a rejected call, for people. */
export function errorText(err: unknown): string {
  if (isCallError(err)) return err.message;
  if (err instanceof Error) return err.message;
  return String(err);
}

/** A small message at the bottom of the window. */
export function toast(text: string, kind: "error" | "info" = "error"): void {
  const host = $("toasts");
  if (!host) return;
  const t = document.createElement("div");
  t.className = `toast ${kind}`;
  t.textContent = text;
  host.append(t);
  while (host.children.length > 3) host.firstElementChild?.remove();
  setTimeout(() => { t.classList.add("out"); setTimeout(() => t.remove(), 260); }, kind === "error" ? 5200 : 3200);
}

let composing = false;
document.addEventListener("compositionstart", () => { composing = true; }, true);
document.addEventListener("compositionend", () => { composing = false; }, true);
/** Whether an input method is composing text: re-rendering now would break it. */
export const isComposing = (): boolean => composing;
/** Whether re-rendering el would interrupt the user typing into it. */
export const typingIn = (el: Element): boolean => composing && !!document.activeElement && el.contains(document.activeElement);

const htmlCache = new WeakMap<Element, string>();

/**
 * Sets an element's markup only when it changed, keeping the focused input
 * (by id, with its caret) and the scroll position of scrollers marked
 * `data-keep-scroll`, so the frequent re-renders are invisible.
 */
export function setHTML(el: Element, html: string): boolean {
  if (htmlCache.get(el) === html && el.childNodes.length) return false;
  if (typingIn(el)) return false;
  const scrollers = [...el.querySelectorAll<HTMLElement>("[data-keep-scroll]")].map((s) => [s.dataset.keepScroll, s.scrollTop] as const);
  const active = document.activeElement as HTMLInputElement | null;
  const focused = active && el.contains(active) && active.id ? active.id : null;
  const sel = focused && "selectionStart" in active! ? [active!.selectionStart, active!.selectionEnd] : null;
  el.innerHTML = html;
  htmlCache.set(el, html);
  for (const [k, top] of scrollers) {
    if (!top) continue;
    const s = el.querySelector<HTMLElement>(`[data-keep-scroll="${k}"]`);
    if (s) s.scrollTop = top;
  }
  if (focused) {
    const i = document.getElementById(focused) as HTMLInputElement | null;
    if (i) {
      i.focus({ preventScroll: true });
      if (sel && sel[0] != null && typeof i.setSelectionRange === "function") try { i.setSelectionRange(sel[0], sel[1] ?? sel[0]); } catch { /* not a text input */ }
    }
  }
  return true;
}
export const forget = (el: Element): void => { htmlCache.delete(el); };

export const reduceMotion = (): boolean => matchMedia("(prefers-reduced-motion: reduce)").matches;
