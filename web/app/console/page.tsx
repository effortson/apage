"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { api, List } from "@/lib/api";
import { Stat, Card, Badge, StatusDot, EmptyState, Skeleton, Banner } from "@/components/ui";
import { relativeTime, formatBytes, pct } from "@/lib/format";

export default function Overview() {
  const [usage, setUsage] = useState<any>(null);
  const [instances, setInstances] = useState<any[]>([]);
  const [links, setLinks] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([
      api<any>("/usage").catch(() => null),
      api<List<any>>("/instances?limit=50").catch(() => ({ items: [] })),
      api<List<any>>("/preview-links?limit=5").catch(() => ({ items: [] })),
    ]).then(([u, i, l]) => {
      setUsage(u); setInstances(i.items || []); setLinks(l.items || []); setLoading(false);
    });
  }, []);

  if (loading) return <Skeleton rows={6} />;
  const online = instances.filter((i) => i.status === "online").length;
  const activeLinks = links.length;
  const storage = usage?.metrics?.storageBytes;
  const tunnel = usage?.metrics?.tunnelEgress;
  const nearLimit = storage && pct(storage.used, storage.limit) >= 80;

  return (
    <div>
      <h1 style={{ marginBottom: "var(--space-4)" }}>Overview</h1>
      {nearLimit && <Banner tone="warning">You&apos;re approaching your storage limit. <Link href="/console/usage">View usage</Link> or upgrade.</Banner>}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(200px,1fr))", gap: 16, marginBottom: 24 }}>
        <Stat label="Online instances" value={`${online}/${instances.length}`} />
        <Stat label="Recent links" value={activeLinks} />
        <Stat label="Storage used" value={storage ? formatBytes(storage.used) : "—"} sub={storage ? `of ${formatBytes(storage.limit)}` : undefined} />
        <Stat label="Tunnel egress" value={tunnel ? formatBytes(tunnel.used) : "—"} sub={tunnel ? `of ${formatBytes(tunnel.limit)}` : undefined} />
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16 }}>
        <Card title="Instances">
          {instances.length === 0 ? (
            <EmptyState title="No instances yet" hint="Install the agent and add an instance to start." action={<Link href="/console/instances">Add instance →</Link>} />
          ) : instances.slice(0, 5).map((i) => (
            <div key={i.instanceId} style={{ display: "flex", justifyContent: "space-between", padding: "8px 0", borderBottom: "1px solid var(--color-border)" }}>
              <span>{i.agentName} <span className="mono" style={{ color: "var(--color-text-subtle)" }}>{i.subdomain}</span></span>
              <StatusDot online={i.status === "online"} />
            </div>
          ))}
        </Card>
        <Card title="Recent shares" action={<Link href="/console/links">View all</Link>}>
          {links.length === 0 ? (
            <EmptyState title="No preview links yet" hint="Create your first preview link." action={<Link href="/console/links">Create link →</Link>} />
          ) : links.map((l) => (
            <div key={l.linkId} style={{ display: "flex", justifyContent: "space-between", padding: "8px 0", borderBottom: "1px solid var(--color-border)" }}>
              <span>{l.displayName || l.linkId}</span>
              <span style={{ display: "flex", gap: 8, alignItems: "center" }}>
                <Badge tone={l.mode === "tunnel" ? "info" : "muted"}>{l.mode}</Badge>
                <span style={{ fontSize: 12, color: "var(--color-text-subtle)" }}>{relativeTime(l.createdAt)}</span>
              </span>
            </div>
          ))}
        </Card>
      </div>
    </div>
  );
}
