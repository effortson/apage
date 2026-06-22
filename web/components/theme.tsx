"use client";
import { useEffect, useState } from "react";

export function ThemeToggle() {
  const [theme, setTheme] = useState<"light" | "dark">("light");
  useEffect(() => {
    const saved = (localStorage.getItem("apage_theme") as "light" | "dark") || "light";
    setTheme(saved);
    document.documentElement.setAttribute("data-theme", saved);
  }, []);
  const toggle = () => {
    const next = theme === "light" ? "dark" : "light";
    setTheme(next);
    localStorage.setItem("apage_theme", next);
    document.documentElement.setAttribute("data-theme", next);
  };
  return (
    <button onClick={toggle} aria-label="toggle theme" title="Toggle theme"
      style={{ background: "none", border: "1px solid var(--color-border)", borderRadius: "var(--radius-sm)", padding: "4px 8px", cursor: "pointer", color: "var(--color-text-muted)" }}>
      {theme === "light" ? "🌙" : "☀️"}
    </button>
  );
}
