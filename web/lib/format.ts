// Formatting helpers (UI §11: locale-aware relative time, numbers, bytes).
import { activeLocale } from "./i18n";

// relativeTime uses Intl.RelativeTimeFormat so units localize to the active locale.
export function relativeTime(iso?: string | null): string {
  if (!iso) return "—";
  const sec = Math.round((Date.now() - new Date(iso).getTime()) / 1000);
  const rtf = new Intl.RelativeTimeFormat(activeLocale(), { numeric: "auto" });
  if (Math.abs(sec) < 60) return rtf.format(-sec, "second");
  const min = Math.round(sec / 60);
  if (Math.abs(min) < 60) return rtf.format(-min, "minute");
  const hr = Math.round(min / 60);
  if (Math.abs(hr) < 24) return rtf.format(-hr, "hour");
  return rtf.format(-Math.round(hr / 24), "day");
}

export function absoluteTime(iso?: string | null): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString(activeLocale());
}

export function formatNumber(n?: number): string {
  if (n === undefined || n === null) return "—";
  return n.toLocaleString(activeLocale());
}

export function formatBytes(n?: number): string {
  if (n === undefined || n === null) return "—";
  if (n === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(n) / Math.log(1024));
  const val = (n / Math.pow(1024, i)).toLocaleString(activeLocale(), { maximumFractionDigits: i === 0 ? 0 : 1 });
  return `${val} ${units[i]}`;
}

export function pct(used: number, limit: number): number {
  if (!limit) return 0;
  return Math.min(100, Math.round((used / limit) * 100));
}
