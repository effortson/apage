"use client";
// APAGE shared component library (UI spec §3). Variants and key states per spec.
import React, { useState, useEffect, useRef, createContext, useContext, useCallback } from "react";

type CSS = React.CSSProperties;

// useDialogA11y traps focus inside an overlay, closes on Escape, and restores
// focus to the trigger on unmount (UI §10 accessibility / focus trap).
function useDialogA11y(onClose: () => void) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const node = ref.current;
    const prev = document.activeElement as HTMLElement | null;
    const focusables = () =>
      node
        ? Array.from(
            node.querySelectorAll<HTMLElement>(
              'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea,[tabindex]:not([tabindex="-1"])',
            ),
          )
        : [];
    (focusables()[0] || node)?.focus();
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        onClose();
        return;
      }
      if (e.key === "Tab") {
        const items = focusables();
        if (items.length === 0) return;
        const first = items[0];
        const last = items[items.length - 1];
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    }
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("keydown", onKey);
      prev?.focus?.();
    };
  }, [onClose]);
  return ref;
}

// ---------- Button ----------
export function Button({
  variant = "primary", size = "md", loading, children, ...rest
}: {
  variant?: "primary" | "secondary" | "ghost" | "danger";
  size?: "sm" | "md";
  loading?: boolean;
} & React.ButtonHTMLAttributes<HTMLButtonElement>) {
  const base: CSS = {
    display: "inline-flex", alignItems: "center", justifyContent: "center", gap: 6,
    border: "1px solid transparent", borderRadius: "var(--radius-md)", cursor: "pointer",
    fontWeight: 500, fontFamily: "var(--font-sans)",
    padding: size === "sm" ? "4px 10px" : "8px 14px", fontSize: size === "sm" ? 13 : 14,
    opacity: rest.disabled || loading ? 0.6 : 1,
  };
  const styles: Record<string, CSS> = {
    primary: { background: "var(--color-primary)", color: "var(--color-primary-fg)" },
    secondary: { background: "var(--color-bg)", color: "var(--color-text)", borderColor: "var(--color-border-strong)" },
    ghost: { background: "transparent", color: "var(--color-text-muted)" },
    danger: { background: "var(--color-danger)", color: "#fff" },
  };
  return (
    <button {...rest} disabled={rest.disabled || loading} style={{ ...base, ...styles[variant], ...rest.style }}>
      {loading ? "…" : children}
    </button>
  );
}

// ---------- Card ----------
export function Card({ title, action, children, style }: { title?: React.ReactNode; action?: React.ReactNode; children: React.ReactNode; style?: CSS }) {
  return (
    <div style={{ background: "var(--color-bg-subtle)", border: "1px solid var(--color-border)", borderRadius: "var(--radius-md)", padding: "var(--space-4)", ...style }}>
      {(title || action) && (
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "var(--space-3)" }}>
          <h3>{title}</h3>
          {action}
        </div>
      )}
      {children}
    </div>
  );
}

// ---------- Badge (semantic, text+icon dual encoding, UI §4.1) ----------
const sem: Record<string, { fg: string; bg: string; dot: string }> = {
  success: { fg: "var(--color-success)", bg: "var(--color-success-subtle)", dot: "●" },
  warning: { fg: "var(--color-warning)", bg: "var(--color-warning-subtle)", dot: "◐" },
  danger: { fg: "var(--color-danger)", bg: "var(--color-danger-subtle)", dot: "■" },
  info: { fg: "var(--color-info)", bg: "var(--color-info-subtle)", dot: "●" },
  muted: { fg: "var(--color-text-muted)", bg: "var(--color-bg-muted)", dot: "○" },
};
export function Badge({ tone = "muted", children }: { tone?: keyof typeof sem; children: React.ReactNode }) {
  const s = sem[tone];
  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 4, color: s.fg, background: s.bg, borderRadius: "var(--radius-sm)", padding: "2px 8px", fontSize: 12, fontWeight: 500 }}>
      <span aria-hidden style={{ fontSize: 9 }}>{s.dot}</span>{children}
    </span>
  );
}

// Map domain statuses to badge tones (UI §4.1).
export function statusTone(status: string): keyof typeof sem {
  switch (status) {
    case "online": case "ready": case "verified": case "active": return "success";
    case "scanning": case "converting": case "pending": case "uploading": case "expiring": return "warning";
    case "offline": case "rejected": case "failed": case "revoked": case "frozen": return "danger";
    case "uploaded": return "info";
    default: return "muted";
  }
}

export function StatusDot({ online, label }: { online: boolean; label?: string }) {
  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
      <span aria-hidden style={{ width: 8, height: 8, borderRadius: "50%", background: online ? "var(--color-success)" : "var(--color-danger)" }} />
      {label || (online ? "online" : "offline")}
    </span>
  );
}

// ---------- Input ----------
export function Input(props: React.InputHTMLAttributes<HTMLInputElement> & { label?: string; error?: string }) {
  const { label, error, ...rest } = props;
  return (
    <label style={{ display: "block", marginBottom: "var(--space-3)" }}>
      {label && <div style={{ fontSize: 13, color: "var(--color-text-muted)", marginBottom: 4 }}>{label}</div>}
      <input {...rest} style={{ width: "100%", padding: "8px 10px", fontSize: 14, borderRadius: "var(--radius-sm)", border: `1px solid ${error ? "var(--color-danger)" : "var(--color-border-strong)"}`, background: "var(--color-bg)", color: "var(--color-text)", ...rest.style }} />
      {error && <div role="alert" style={{ color: "var(--color-danger)", fontSize: 12, marginTop: 4 }}>{error}</div>}
    </label>
  );
}

export function Select(props: React.SelectHTMLAttributes<HTMLSelectElement> & { label?: string }) {
  const { label, children, ...rest } = props;
  return (
    <label style={{ display: "block", marginBottom: "var(--space-3)" }}>
      {label && <div style={{ fontSize: 13, color: "var(--color-text-muted)", marginBottom: 4 }}>{label}</div>}
      <select {...rest} style={{ width: "100%", padding: "8px 10px", fontSize: 14, borderRadius: "var(--radius-sm)", border: "1px solid var(--color-border-strong)", background: "var(--color-bg)", color: "var(--color-text)" }}>
        {children}
      </select>
    </label>
  );
}

// ---------- Table ----------
export function Table({ head, children }: { head: React.ReactNode[]; children: React.ReactNode }) {
  return (
    <div style={{ overflowX: "auto", border: "1px solid var(--color-border)", borderRadius: "var(--radius-md)" }}>
      <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 14 }}>
        <thead>
          <tr style={{ background: "var(--color-bg-muted)" }}>
            {head.map((h, i) => (
              <th key={i} style={{ textAlign: "left", padding: "10px 12px", fontWeight: 600, color: "var(--color-text-muted)", fontSize: 12, whiteSpace: "nowrap" }}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>{children}</tbody>
      </table>
    </div>
  );
}
export function Td({ children, mono, style, title }: { children: React.ReactNode; mono?: boolean; style?: CSS; title?: string }) {
  return <td title={title} style={{ padding: "10px 12px", borderTop: "1px solid var(--color-border)", fontFamily: mono ? "var(--font-mono)" : undefined, fontSize: mono ? 13 : 14, ...style }}>{children}</td>;
}

// ---------- EmptyState ----------
export function EmptyState({ title, hint, action }: { title: string; hint?: string; action?: React.ReactNode }) {
  return (
    <div style={{ textAlign: "center", padding: "var(--space-10) var(--space-4)", color: "var(--color-text-muted)" }}>
      <div style={{ fontSize: 32, marginBottom: 8 }}>◇</div>
      <div style={{ fontWeight: 600, color: "var(--color-text)" }}>{title}</div>
      {hint && <div style={{ fontSize: 13, marginTop: 4 }}>{hint}</div>}
      {action && <div style={{ marginTop: "var(--space-4)" }}>{action}</div>}
    </div>
  );
}

// ---------- Stat ----------
export function Stat({ label, value, sub }: { label: string; value: React.ReactNode; sub?: string }) {
  return (
    <Card>
      <div style={{ fontSize: 12, color: "var(--color-text-muted)" }}>{label}</div>
      <div style={{ fontSize: 24, fontWeight: 700, marginTop: 4 }}>{value}</div>
      {sub && <div style={{ fontSize: 12, color: "var(--color-text-subtle)", marginTop: 2 }}>{sub}</div>}
    </Card>
  );
}

// ---------- Banner ----------
export function Banner({ tone = "info", children, onClose }: { tone?: keyof typeof sem; children: React.ReactNode; onClose?: () => void }) {
  const s = sem[tone];
  return (
    <div role="status" style={{ display: "flex", justifyContent: "space-between", alignItems: "center", background: s.bg, color: s.fg, border: `1px solid ${s.fg}33`, borderRadius: "var(--radius-md)", padding: "10px 14px", marginBottom: "var(--space-4)" }}>
      <div>{children}</div>
      {onClose && <button onClick={onClose} aria-label="dismiss" style={{ background: "none", border: "none", color: s.fg, cursor: "pointer" }}>✕</button>}
    </div>
  );
}

// ---------- CopyField / CodeBlock ----------
export function CopyField({ value, mono = true }: { value: string; mono?: boolean }) {
  const [copied, setCopied] = useState(false);
  return (
    <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
      <code style={{ flex: 1, padding: "6px 10px", background: "var(--color-bg-muted)", borderRadius: "var(--radius-sm)", overflowX: "auto", whiteSpace: "nowrap", fontFamily: mono ? "var(--font-mono)" : "var(--font-sans)", fontSize: 13 }}>{value}</code>
      <Button size="sm" variant="secondary" onClick={() => { navigator.clipboard?.writeText(value); setCopied(true); setTimeout(() => setCopied(false), 1500); }}>{copied ? "Copied" : "Copy"}</Button>
    </div>
  );
}
export function CodeBlock({ children }: { children: string }) {
  return (
    <div style={{ position: "relative" }}>
      <pre style={{ background: "var(--color-bg-muted)", borderRadius: "var(--radius-md)", padding: "var(--space-4)", overflowX: "auto", fontFamily: "var(--font-mono)", fontSize: 13, margin: 0 }}>{children}</pre>
    </div>
  );
}

// ---------- SecretReveal (one-time, UI §4.3) ----------
export function SecretReveal({ value }: { value: string }) {
  const [shown, setShown] = useState(false);
  return (
    <div>
      <Banner tone="warning">This secret is shown only once. Store it securely; don&apos;t paste into public channels.</Banner>
      <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
        <code style={{ flex: 1, padding: "8px 10px", background: "var(--color-bg-muted)", borderRadius: "var(--radius-sm)", overflowX: "auto", whiteSpace: "nowrap", fontFamily: "var(--font-mono)", fontSize: 13 }}>
          {shown ? value : "•".repeat(Math.min(40, value.length))}
        </code>
        <Button size="sm" variant="secondary" onClick={() => setShown((s) => !s)}>{shown ? "Hide" : "Reveal"}</Button>
        <Button size="sm" onClick={() => navigator.clipboard?.writeText(value)}>Copy</Button>
      </div>
    </div>
  );
}

// ---------- Toast ----------
type Toast = { id: number; tone: keyof typeof sem; msg: string };
const ToastCtx = createContext<(t: Omit<Toast, "id">) => void>(() => {});
export function useToast() { return useContext(ToastCtx); }
export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const push = useCallback((t: Omit<Toast, "id">) => {
    const id = Date.now() + Math.random();
    setToasts((p) => [...p, { ...t, id }]);
    setTimeout(() => setToasts((p) => p.filter((x) => x.id !== id)), 4000);
  }, []);
  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div style={{ position: "fixed", top: 16, right: 16, display: "flex", flexDirection: "column", gap: 8, zIndex: 1000 }}>
        {toasts.map((t) => (
          <div key={t.id} style={{ background: sem[t.tone].bg, color: sem[t.tone].fg, border: `1px solid ${sem[t.tone].fg}44`, borderRadius: "var(--radius-md)", padding: "10px 14px", boxShadow: "var(--shadow-md)", minWidth: 220 }}>{t.msg}</div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}

// ---------- Modal / ConfirmDialog (danger variant, UI §4.2) ----------
export function Modal({ title, onClose, children }: { title: string; onClose: () => void; children: React.ReactNode }) {
  const ref = useDialogA11y(onClose);
  return (
    <div onClick={onClose} style={{ position: "fixed", inset: 0, background: "rgba(0,0,0,0.4)", display: "flex", alignItems: "center", justifyContent: "center", zIndex: 900 }}>
      <div ref={ref} role="dialog" aria-modal="true" aria-label={title} tabIndex={-1} onClick={(e) => e.stopPropagation()} style={{ background: "var(--color-bg)", borderRadius: "var(--radius-lg)", padding: "var(--space-5)", width: 460, maxWidth: "90vw", boxShadow: "var(--shadow-lg)", border: "1px solid var(--color-border)", outline: "none" }}>
        <h2 style={{ marginBottom: "var(--space-4)" }}>{title}</h2>
        {children}
      </div>
    </div>
  );
}
export function ConfirmDialog({ title, message, confirmWord, onConfirm, onCancel, danger }: { title: string; message: string; confirmWord?: string; onConfirm: () => void; onCancel: () => void; danger?: boolean }) {
  const [typed, setTyped] = useState("");
  const ready = !confirmWord || typed === confirmWord;
  return (
    <Modal title={title} onClose={onCancel}>
      <p style={{ color: "var(--color-text-muted)" }}>{message}</p>
      {confirmWord && <Input label={`Type "${confirmWord}" to confirm`} value={typed} onChange={(e) => setTyped(e.target.value)} />}
      <div style={{ display: "flex", justifyContent: "flex-end", gap: 8, marginTop: "var(--space-4)" }}>
        <Button variant="secondary" onClick={onCancel}>Cancel</Button>
        <Button variant={danger ? "danger" : "primary"} disabled={!ready} onClick={onConfirm}>Confirm</Button>
      </div>
    </Modal>
  );
}

// ---------- Drawer ----------
export function Drawer({ title, onClose, children }: { title: string; onClose: () => void; children: React.ReactNode }) {
  const ref = useDialogA11y(onClose);
  return (
    <div onClick={onClose} style={{ position: "fixed", inset: 0, background: "rgba(0,0,0,0.4)", zIndex: 900 }}>
      <div ref={ref} role="dialog" aria-modal="true" aria-label={title} tabIndex={-1} onClick={(e) => e.stopPropagation()} style={{ position: "absolute", top: 0, right: 0, bottom: 0, width: 480, maxWidth: "92vw", background: "var(--color-bg)", borderLeft: "1px solid var(--color-border)", padding: "var(--space-5)", overflowY: "auto", boxShadow: "var(--shadow-lg)", outline: "none" }}>
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "var(--space-4)" }}>
          <h2>{title}</h2>
          <button onClick={onClose} aria-label="close" style={{ background: "none", border: "none", fontSize: 18, cursor: "pointer", color: "var(--color-text-muted)" }}>✕</button>
        </div>
        {children}
      </div>
    </div>
  );
}

// ---------- Skeleton ----------
export function Skeleton({ rows = 3 }: { rows?: number }) {
  return (
    <div>
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} style={{ height: 16, background: "var(--color-bg-muted)", borderRadius: 4, margin: "8px 0", opacity: 1 - i * 0.1 }} />
      ))}
    </div>
  );
}
