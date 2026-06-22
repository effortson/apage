"use client";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { api, setTenant, getTenant } from "@/lib/api";
import { ThemeToggle } from "@/components/theme";
import { Badge } from "@/components/ui";
import { useT, LocaleToggle } from "@/lib/i18n";

type Session = {
  user: { userId: string; email: string; emailVerified: boolean };
  tenants: { tenantId: string; name: string; plan: string; role: string }[];
};

const roleRank: Record<string, number> = { viewer: 0, member: 1, admin: 2, owner: 3 };

// Nav with the minimum role each surface requires (mirrors backend RBAC, UI §7).
const nav: [string, string, string][] = [
  ["Overview", "/console", "viewer"],
  ["Instances", "/console/instances", "viewer"],
  ["Preview Links", "/console/links", "viewer"],
  ["Cloud Files", "/console/files", "viewer"],
  ["Custom Domains", "/console/domains", "admin"],
  ["Audit Logs", "/console/audit", "admin"],
  ["Usage & Billing", "/console/usage", "admin"],
  ["Members", "/console/members", "member"],
  ["Settings", "/console/settings", "member"],
];

export default function ConsoleLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const t = useT();
  const [session, setSession] = useState<Session | null>(null);
  const [tenant, setT] = useState<string | null>(null);

  useEffect(() => {
    api<Session>("/auth/session", { tenant: false })
      .then((s) => {
        setSession(s);
        let t = getTenant();
        if (!t || !s.tenants.find((x) => x.tenantId === t)) {
          t = s.tenants[0]?.tenantId || null;
          setTenant(t);
        }
        setT(t);
      })
      .catch(() => router.push("/login"));
  }, [router]);

  if (!session) return <div style={{ padding: 40 }}>Loading…</div>;
  const current = session.tenants.find((x) => x.tenantId === tenant);
  const rank = roleRank[current?.role || "viewer"] ?? 0;
  const visibleNav = nav.filter(([, , min]) => rank >= (roleRank[min] ?? 0));

  return (
    <div style={{ display: "flex", minHeight: "100vh" }}>
      <aside style={{ width: 240, borderRight: "1px solid var(--color-border)", padding: "var(--space-4)", background: "var(--color-bg-subtle)" }}>
        <div style={{ fontWeight: 700, fontSize: 18, marginBottom: "var(--space-5)" }}>APAGE</div>
        <nav>
          {visibleNav.map(([label, href]) => {
            const active = pathname === href;
            return (
              <Link key={href} href={href} style={{
                display: "block", padding: "8px 10px", borderRadius: "var(--radius-sm)", marginBottom: 2,
                color: active ? "var(--color-primary)" : "var(--color-text-muted)",
                background: active ? "var(--color-primary-subtle)" : "transparent",
                fontWeight: active ? 600 : 400, textDecoration: "none",
              }}>{t(label)}</Link>
            );
          })}
        </nav>
      </aside>
      <div style={{ flex: 1, display: "flex", flexDirection: "column" }}>
        <header style={{ display: "flex", justifyContent: "space-between", alignItems: "center", padding: "12px 24px", borderBottom: "1px solid var(--color-border)" }}>
          <select value={tenant || ""} onChange={(e) => { setTenant(e.target.value); setT(e.target.value); location.reload(); }}
            style={{ padding: "6px 10px", borderRadius: "var(--radius-sm)", border: "1px solid var(--color-border-strong)", background: "var(--color-bg)", color: "var(--color-text)" }}>
            {session.tenants.map((t) => <option key={t.tenantId} value={t.tenantId}>{t.name}</option>)}
          </select>
          <div style={{ display: "flex", gap: 12, alignItems: "center" }}>
            {current && <Badge tone="info">{current.plan}</Badge>}
            {current && <span style={{ fontSize: 13, color: "var(--color-text-muted)" }}>{current.role}</span>}
            <span style={{ fontSize: 13 }}>{session.user.email}</span>
            <LocaleToggle />
            <ThemeToggle />
            <button onClick={async () => { await api("/auth/logout", { method: "POST", tenant: false }); router.push("/login"); }}
              style={{ background: "none", border: "1px solid var(--color-border)", borderRadius: "var(--radius-sm)", padding: "4px 8px", cursor: "pointer", color: "var(--color-text-muted)" }}>{t("Sign out")}</button>
          </div>
        </header>
        <main style={{ flex: 1, padding: "var(--space-5)", maxWidth: 1280 }}>{children}</main>
      </div>
    </div>
  );
}
