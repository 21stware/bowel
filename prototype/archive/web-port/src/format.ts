// Formatting helpers. Everything that comes from Go is data: escape it
// before it goes into markup.

const ESC: Record<string, string> = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" };
export const esc = (s: unknown): string => String(s ?? "").replace(/[&<>"']/g, (c) => ESC[c] ?? c);

/** A byte count as [value, unit], 1024-based, three significant digits. */
export function unit(b: number): [string, string] {
  const u = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  b = Math.max(0, b || 0);
  while (b >= 1000 && i < u.length - 1) { b /= 1024; i++; }
  const v = i === 0 ? String(Math.round(b)) : b < 10 ? b.toFixed(2) : b < 100 ? b.toFixed(1) : String(Math.round(b));
  return [v, u[i]!];
}
export const fmt = (b: number, suffix = ""): string => { const [v, u] = unit(b); return `${v} ${u}${suffix}`; };

/** Seconds as a short duration. */
export const dur = (s: number): string => {
  s = Math.max(0, Math.round(s));
  return s < 60 ? `${s} 秒` : s < 3600 ? `${Math.round(s / 60)} 分` : s < 86400 ? `${Math.round(s / 3600)} 时` : `${Math.round(s / 86400)} 天`;
};

/** A time from Go (RFC 3339); null for Go's zero time or garbage. */
export function when(iso?: string | null): Date | null {
  if (!iso) return null;
  const d = new Date(iso);
  const t = d.getTime();
  return Number.isNaN(t) || d.getFullYear() < 1990 ? null : d;
}
const p2 = (n: number) => String(n).padStart(2, "0");
export const clock = (iso?: string | null, secs = true): string => {
  const d = when(iso);
  if (!d) return "";
  return secs ? `${p2(d.getHours())}:${p2(d.getMinutes())}:${p2(d.getSeconds())}` : `${p2(d.getHours())}:${p2(d.getMinutes())}`;
};
export const minSec = (iso?: string | null): string => {
  const d = when(iso);
  return d ? `${p2(d.getMinutes())}:${p2(d.getSeconds())}` : "";
};
export function ago(iso?: string | null): string {
  const d = when(iso);
  if (!d) return "";
  const s = (Date.now() - d.getTime()) / 1000;
  if (s < 60) return "刚刚";
  if (s < 3600) return `${Math.floor(s / 60)} 分钟前`;
  if (s < 86400) return `${Math.floor(s / 3600)} 小时前`;
  return `${Math.floor(s / 86400)} 天前`;
}

/** A subscription's expiry (unix seconds) for people. */
export function expireText(unix: number): string {
  if (!unix) return "不过期";
  const d = new Date(unix * 1000), days = Math.ceil((d.getTime() - Date.now()) / 86400000);
  if (days < 0) return "已过期";
  const date = `${d.getMonth() + 1} 月 ${d.getDate()} 日到期`;
  return days <= 60 ? `${date} · 剩 ${days} 天` : date;
}
export const gb = (b: number): string => {
  const v = b / 1024 ** 3;
  return v >= 100 ? String(Math.round(v)) : v >= 10 ? v.toFixed(1) : v.toFixed(2).replace(/\.?0+$/, "") || "0";
};

/** Delay colour: green, amber, red; mute when never tested. */
export const tone = (delay: number, tested = true): string =>
  !tested ? "var(--mute)" : delay <= 0 ? "var(--signal)" : delay < 120 ? "var(--good)" : delay < 250 ? "var(--warn)" : "var(--signal)";
export const delayText = (delay: number, tested: boolean): string => (!tested ? "未测" : delay > 0 ? `${delay} ms` : "超时");

/** Names of the countries Place.country may hold (ISO 3166-1 alpha-2). */
const COUNTRIES: Record<string, string> = {
  HK: "香港", MO: "澳门", TW: "台湾", JP: "日本", KR: "韩国", MN: "蒙古", CN: "中国", SG: "新加坡", MY: "马来西亚",
  TH: "泰国", VN: "越南", PH: "菲律宾", ID: "印尼", KH: "柬埔寨", MM: "缅甸", IN: "印度", PK: "巴基斯坦", BD: "孟加拉",
  KZ: "哈萨克斯坦", AU: "澳大利亚", NZ: "新西兰", US: "美国", CA: "加拿大", MX: "墨西哥", BR: "巴西", AR: "阿根廷",
  CL: "智利", CO: "哥伦比亚", GB: "英国", UK: "英国", DE: "德国", FR: "法国", NL: "荷兰", IT: "意大利", ES: "西班牙",
  PT: "葡萄牙", CH: "瑞士", AT: "奥地利", BE: "比利时", IE: "爱尔兰", SE: "瑞典", NO: "挪威", FI: "芬兰", DK: "丹麦",
  PL: "波兰", CZ: "捷克", HU: "匈牙利", RO: "罗马尼亚", BG: "保加利亚", UA: "乌克兰", RU: "俄罗斯", TR: "土耳其",
  GR: "希腊", IL: "以色列", AE: "阿联酋", SA: "沙特", QA: "卡塔尔", IR: "伊朗", EG: "埃及", ZA: "南非", NG: "尼日利亚",
  KE: "肯尼亚", LU: "卢森堡", IS: "冰岛", EE: "爱沙尼亚", LV: "拉脱维亚", LT: "立陶宛", RS: "塞尔维亚", HR: "克罗地亚",
  MD: "摩尔多瓦", GE: "格鲁吉亚", AM: "亚美尼亚", AZ: "阿塞拜疆", UZ: "乌兹别克斯坦", NP: "尼泊尔", LK: "斯里兰卡",
  PE: "秘鲁", VE: "委内瑞拉", PA: "巴拿马", CR: "哥斯达黎加",
};
export const countryName = (code: string): string => COUNTRIES[code.toUpperCase()] ?? code.toUpperCase();

export const plural = (n: number, w: string): string => `${n} ${w}`;
