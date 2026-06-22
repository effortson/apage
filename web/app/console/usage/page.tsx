"use client";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { Card, Skeleton, Badge } from "@/components/ui";
import { formatBytes, pct, absoluteTime } from "@/lib/format";

const labels: Record<string, { name: string; bytes?: boolean }> = {
  instances: { name: "Instances" },
  storageBytes: { name: "Storage", bytes: true },
  tunnelEgress: { name: "Tunnel egress", bytes: true },
  cloudEgress: { name: "Cloud egress", bytes: true },
  conversions: { name: "Conversions" },
  customDomains: { name: "Custom domains" },
};

export default function Usage() {
  const [u, setU] = useState<any>(null);
  useEffect(() => { api<any>("/usage").then(setU).catch(() => setU(null)); }, []);
  if (!u) return <Skeleton rows={6} />;

  return (
    <div>
      <h1 style={{ marginBottom: 16 }}>Usage &amp; Billing</h1>
      <Card title={<span>Current plan <Badge tone="info">{u.plan}</Badge></span>}>
        <p style={{ fontSize: 13, color: "var(--color-text-muted)" }}>
          Period {absoluteTime(u.periodStart)} → {absoluteTime(u.periodEnd)}. Tunnel billed by instances/traffic/domains; Cloud by storage/download/conversions/retention. Lite prompts an upgrade past limits and never auto-bills.
        </p>
      </Card>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(280px,1fr))", gap: 16, marginTop: 16 }}>
        {Object.entries(u.metrics).map(([k, m]: any) => {
          const meta = labels[k] || { name: k };
          const p = pct(m.used, m.limit);
          const fmt = meta.bytes ? formatBytes : (n: number) => String(n);
          return (
            <Card key={k}>
              <div style={{ display: "flex", justifyContent: "space-between" }}>
                <span style={{ fontWeight: 600 }}>{meta.name}</span>
                <span style={{ fontSize: 13, color: p >= 80 ? "var(--color-danger)" : "var(--color-text-muted)" }}>{fmt(m.used)} / {fmt(m.limit)}</span>
              </div>
              <div style={{ height: 8, background: "var(--color-bg-muted)", borderRadius: 4, marginTop: 8, overflow: "hidden" }}>
                <div style={{ width: `${p}%`, height: "100%", background: p >= 80 ? "var(--color-danger)" : "var(--color-primary)" }} />
              </div>
            </Card>
          );
        })}
      </div>
    </div>
  );
}
