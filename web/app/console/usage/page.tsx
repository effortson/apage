"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { AlertTriangle } from "lucide-react";
import { api, ApiException } from "@/lib/api";
import { formatBytes, pct, absoluteTime } from "@/lib/format";
import { PageHeader, Stat } from "@/components/composites";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";

const labels: Record<string, { name: string; bytes?: boolean }> = {
  instances: { name: "Instances" },
  storageBytes: { name: "Storage", bytes: true },
  cloudEgress: { name: "Cloud egress", bytes: true },
  customDomains: { name: "Custom domains" },
};

type Metric = { used: number; limit: number };
type Day = { day: string; cloudEgress: number; storageBytes: number };
type Billing = {
  plan: string;
  price: { monthlyCents: number; currency: string };
  upgradeOptions: string[];
  autoCharge: boolean;
};

export default function Usage() {
  const [u, setU] = useState<any>(null);
  const [series, setSeries] = useState<Day[]>([]);
  const [billing, setBilling] = useState<Billing | null>(null);

  useEffect(() => {
    api<any>("/usage").then(setU).catch(() => setU(null));
    api<{ series: Day[] }>("/usage/timeseries?days=30")
      .then((r) => setSeries(r.series || []))
      .catch(() => {});
    // /billing is owner-only; silently skip for non-owners (RBAC, UI §7.7).
    api<Billing>("/billing")
      .then(setBilling)
      .catch((e) => {
        if (!(e instanceof ApiException && e.status === 403)) setBilling(null);
      });
  }, []);

  if (!u) {
    return (
      <div>
        <PageHeader title="Usage & Billing" />
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-28 w-full" />
          ))}
        </div>
      </div>
    );
  }

  const metrics: [string, Metric][] = Object.entries(u.metrics);
  const overLimit = metrics.filter(([, m]) => pct(m.used, m.limit) >= 80);

  return (
    <div>
      <PageHeader
        title="Usage & Billing"
        description="Billed by cloud storage, download (egress), and retention."
      />

      {overLimit.length > 0 && (
        <Alert className="mb-6">
          <AlertTriangle className="h-4 w-4" />
          <AlertTitle>Approaching limits</AlertTitle>
          <AlertDescription>
            {overLimit.map(([k]) => labels[k]?.name || k).join(", ")}. Lite prompts an upgrade past
            limits and never auto-bills —{" "}
            <Link href="/pricing" className="underline underline-offset-4">
              see plans
            </Link>
            .
          </AlertDescription>
        </Alert>
      )}

      <Card className="mb-6">
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">Current plan</CardTitle>
          <Badge variant="secondary" className="capitalize">
            {u.plan}
          </Badge>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            Period {absoluteTime(u.periodStart)} → {absoluteTime(u.periodEnd)}. Lite prompts an
            upgrade past limits and never auto-bills.
          </p>
        </CardContent>
      </Card>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {metrics.map(([k, m]) => {
          const meta = labels[k] || { name: k };
          const p = pct(m.used, m.limit);
          const fmt = meta.bytes ? formatBytes : (n: number) => String(n);
          return (
            <Card key={k}>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium text-muted-foreground">
                  {meta.name}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="flex items-baseline justify-between">
                  <span className="text-2xl font-semibold tracking-tight">{fmt(m.used)}</span>
                  <span
                    className={
                      p >= 80
                        ? "text-xs font-medium text-destructive"
                        : "text-xs text-muted-foreground"
                    }
                  >
                    of {fmt(m.limit)}
                  </span>
                </div>
                <div className="mt-3 h-2 rounded-full bg-muted">
                  <div
                    className={
                      p >= 80
                        ? "h-full rounded-full bg-destructive"
                        : "h-full rounded-full bg-primary"
                    }
                    style={{ width: `${Math.min(100, p)}%` }}
                  />
                </div>
              </CardContent>
            </Card>
          );
        })}
      </div>

      <Card className="mt-6">
        <CardHeader>
          <CardTitle>Egress trend (30 days)</CardTitle>
        </CardHeader>
        <CardContent>
          <TrendChart series={series} />
        </CardContent>
      </Card>

      {billing && (
        <Card className="mt-6">
          <CardHeader>
            <CardTitle>Billing</CardTitle>
          </CardHeader>
          <CardContent className="flex items-center justify-between">
            <div>
              <div className="font-medium capitalize">{billing.plan}</div>
              <div className="text-sm text-muted-foreground">
                {billing.price.monthlyCents === 0
                  ? "Free"
                  : `${billing.price.currency} ${(billing.price.monthlyCents / 100).toFixed(2)}/mo`}
                {" · "}over-limit prompts an upgrade, never auto-bills.
              </div>
            </div>
            {billing.upgradeOptions.length > 0 && (
              <Button asChild>
                <Link href="/pricing">Upgrade ({billing.upgradeOptions.join(" / ")})</Link>
              </Button>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}

function TrendChart({ series }: { series: Day[] }) {
  if (!series.length) {
    return (
      <p className="text-sm text-muted-foreground">
        No usage recorded yet — egress appears here as links are viewed.
      </p>
    );
  }
  const vals = series.map((s) => s.cloudEgress || 0);
  const max = Math.max(1, ...vals);
  return (
    <div>
      <div className="flex h-32 items-end gap-px" role="img" aria-label="Daily egress trend">
        {series.map((s) => {
          const v = s.cloudEgress || 0;
          const h = Math.max(2, (v / max) * 100);
          return (
            <div
              key={s.day}
              className="flex-1 rounded-t-sm bg-primary"
              style={{ height: `${h}%` }}
              title={`${s.day}: ${formatBytes(v)}`}
            />
          );
        })}
      </div>
      <p className="mt-2 text-xs text-muted-foreground">
        Peak day: {formatBytes(max)} (cloud egress)
      </p>
    </div>
  );
}
