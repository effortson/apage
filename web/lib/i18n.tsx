"use client";
import React, { createContext, useContext, useEffect, useState } from "react";

export type Locale = "en" | "zh";

// Translation dictionary. English strings are the keys (so untranslated strings
// fall back to readable English); add zh entries incrementally (UI §11).
const dict: Record<Locale, Record<string, string>> = {
  en: {},
  zh: {
    Overview: "概览",
    Instances: "实例",
    "Preview Links": "预览链接",
    "Cloud Files": "云文件",
    "Custom Domains": "自定义域名",
    "Audit Logs": "审计日志",
    "Usage & Billing": "用量与计费",
    Members: "成员",
    Settings: "设置",
    "Sign in": "登录",
    "Sign out": "退出",
    "Create one": "注册",
    "Forgot password?": "忘记密码?",
    Save: "保存",
    Cancel: "取消",
    Confirm: "确认",
    Email: "邮箱",
    Password: "密码",
  },
};

// currentLocale mirrors the active locale for the (context-free) formatters.
let currentLocale: Locale = "en";
export function activeLocale(): Locale {
  return currentLocale;
}

type Ctx = { locale: Locale; setLocale: (l: Locale) => void; t: (k: string) => string };
const LocaleCtx = createContext<Ctx>({ locale: "en", setLocale: () => {}, t: (k) => k });

export function LocaleProvider({ children }: { children: React.ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>("en");
  useEffect(() => {
    const saved = (typeof localStorage !== "undefined" && (localStorage.getItem("apage_locale") as Locale)) ||
      (typeof navigator !== "undefined" && navigator.language.startsWith("zh") ? "zh" : "en");
    apply(saved);
  }, []);
  function apply(l: Locale) {
    currentLocale = l;
    setLocaleState(l);
    if (typeof document !== "undefined") document.documentElement.lang = l;
  }
  function setLocale(l: Locale) {
    if (typeof localStorage !== "undefined") localStorage.setItem("apage_locale", l);
    apply(l);
  }
  const t = (k: string) => dict[locale][k] || k;
  return <LocaleCtx.Provider value={{ locale, setLocale, t }}>{children}</LocaleCtx.Provider>;
}

export const useLocale = () => useContext(LocaleCtx);
export const useT = () => useContext(LocaleCtx).t;

export function LocaleToggle() {
  const { locale, setLocale } = useLocale();
  return (
    <button
      onClick={() => setLocale(locale === "en" ? "zh" : "en")}
      aria-label="toggle language"
      style={{ background: "none", border: "1px solid var(--color-border)", borderRadius: "var(--radius-sm)", padding: "4px 8px", cursor: "pointer", color: "var(--color-text-muted)", fontSize: 13 }}
    >
      {locale === "en" ? "中文" : "EN"}
    </button>
  );
}
