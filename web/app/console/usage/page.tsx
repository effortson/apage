"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { api, ApiException } from "@/lib/api";
import { Card, Skeleton, Badge, Banner, Button } from "@/components/ui";
import { formatBytes, pct, absoluteTime } from "@/lib/format";

const labels: Record<string, { name: string; bytes?: boolean }> = {
  instances: { name: "Instances" },
  storageBytes: { name: "Storage", bytes: true },
  tunnelEgress: { name: "Tunnel egress", bytes: true },
  cloudEgress: { name: "Cloud egress", bytes: true },
  customDomains: { name: "Custom domains" },
};

type Metric = { used: number; limit: number };
type Day = { day: string; tunnelEgress: number; cloudEgress: number; storageBytes: number };
type Billing = { plan: string; price: { monthlyCents: number; currency: string }; upgradeOptions: string[]; autoCharge: boolean };

export default function Usage() {
  const [u, setU] = useState<any>(null);
  const [series, setSeries] = useState<Day[]>([]);
  const [billing, setBilling] = useState<Billing | null>(null);

  useEffect(() => {
    api<any>("/usage").then(setU).catch(() => setU(null));
    api<{ series: Day[] }>("/usage/timeseries?days=30").then((r) => setSeries(r.series || [])).catch(() => {});
    // /billing is owner-only; silently skip for non-owners (RBAC, UI §7.7).
    api<Billing>("/billing").then(setBilling).catch((e) => { if (!(e instanceof ApiException && e.status === 403)) setBilling(null); });
  }, []);

  if (!u) return <Skeleton rows={6} />;

  const metrics: [string, Metric][] = Object.entries(u.metrics);
  const overLimit = metrics.filter(([, m]) => pct(m.used, m.limit) >= 80);

  return (
    <div>
      <h1 style={{ marginBottom: 16 }}>Usage &amp; Billing</h1>

      {overLimit.length > 0 && (
        <Banner tone="warning">
          Approaching limits: {overLimit.map(([k]) => labels[k]?.name || k).join(", ")}. Lite prompts an upgrade past
          limits and never auto-bills — <Link href="/pricing">see plans</Link>.
        </Banner>
      )}

      <Card title={<span>Current plan <Badge tone="info">{u.plan}</Badge></span>}>
        <p style={{ fontSize: 13, color: "var(--color-text-muted)" }}>
          Period {absoluteTime(u.periodStart)} → {absoluteTime(u.periodEnd)}. Tunnel billed by instances/traffic/domains;
          Cloud by storage/download/retention. Lite prompts an upgrade past limits and never auto-bills.
        </p>
      </Card>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(280px,1fr))", gap: 16, marginTop: 16 }}>
        {metrics.map(([k, m]) => {
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

      <Card title="Egress trend (30 days)" style={{ marginTop: 16 }}>
        <TrendChart series={series} />
      </Card>

      {billing && (
        <Card title="Billing" style={{ marginTop: 16 }}>
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
            <div>
              <div style={{ fontWeight: 600, textTransform: "capitalize" }}>{billing.plan}</div>
              <div style={{ fontSize: 13, color: "var(--color-text-muted)" }}>
                {billing.price.monthlyCents === 0 ? "Free" : `${billing.price.currency} ${(billing.price.monthlyCents / 100).toFixed(2)}/mo`}
                {" · "}over-limit prompts an upgrade, never auto-bills.
              </div>
            </div>
            {billing.upgradeOptions.length > 0 && (
              <Link href="/pricing"><Button>Upgrade ({billing.upgradeOptions.join(" / ")})</Button></Link>
            )}
          </div>
        </Card>
      )}
    </div>
  );
}

function TrendChart({ series }: { series: Day[] }) {
  if (!series.length) {
    return <p style={{ color: "var(--color-text-muted)", fontSize: 13 }}>No usage recorded yet — egress appears here as links are viewed.</p>;
  }
  const vals = series.map((s) => (s.tunnelEgress || 0) + (s.cloudEgress || 0));
  const max = Math.max(1, ...vals);
  const W = 600, H = 120, pad = 4;
  const bw = (W - pad * 2) / series.length;
  return (
    <div>
      <svg viewBox={`0 0 ${W} ${H}`} style={{ width: "100%", height: 140 }} role="img" aria-label="Daily egress trend">
        {series.map((s, i) => {
          const v = (s.tunnelEgress || 0) + (s.cloudEgress || 0);
          const bh = (v / max) * (H - 16);
          return (
            <rect key={s.day} x={pad + i * bw + 1} y={H - bh} width={Math.max(1, bw - 2)} height={bh} rx={2} fill="var(--color-primary)">
              <title>{`${s.day}: ${formatBytes(v)}`}</title>
            </rect>
          );
        })}
      </svg>
      <div style={{ fontSize: 12, color: "var(--color-text-muted)" }}>Peak day: {formatBytes(max)} (tunnel + cloud egress)</div>
    </div>
  );
}
