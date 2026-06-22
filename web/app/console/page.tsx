"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { AlertTriangle } from "lucide-react";
import { api, List } from "@/lib/api";
import { relativeTime, formatBytes, pct } from "@/lib/format";
import { PageHeader, Stat, EmptyState } from "@/components/composites";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";

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
      setUsage(u);
      setInstances((i as List<any>).items || []);
      setLinks((l as List<any>).items || []);
      setLoading(false);
    });
  }, []);

  if (loading) {
    return (
      <div>
        <PageHeader title="Overview" />
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-28 w-full" />
          ))}
        </div>
      </div>
    );
  }

  const storage = usage?.metrics?.storageBytes;
  const cloud = usage?.metrics?.cloudEgress;
  const nearLimit = storage && pct(storage.used, storage.limit) >= 80;

  return (
    <div>
      <PageHeader title="Overview" description="Your cloud sharing at a glance." />

      {nearLimit && (
        <Alert className="mb-6">
          <AlertTriangle className="h-4 w-4" />
          <AlertTitle>Approaching storage limit</AlertTitle>
          <AlertDescription>
            <Link href="/console/usage" className="underline underline-offset-4">
              View usage
            </Link>{" "}
            or upgrade your plan.
          </AlertDescription>
        </Alert>
      )}

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Stat label="Instances" value={instances.length} />
        <Stat label="Recent links" value={links.length} />
        <Stat
          label="Storage used"
          value={storage ? formatBytes(storage.used) : "—"}
          sub={storage ? `of ${formatBytes(storage.limit)}` : undefined}
        />
        <Stat
          label="Cloud egress"
          value={cloud ? formatBytes(cloud.used) : "—"}
          sub={cloud ? `of ${formatBytes(cloud.limit)}` : undefined}
        />
      </div>

      <div className="mt-6 grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Instances</CardTitle>
          </CardHeader>
          <CardContent>
            {instances.length === 0 ? (
              <EmptyState
                title="No instances yet"
                hint="Add an instance, then run apage-cli to let your agent create links."
                action={
                  <Button asChild size="sm">
                    <Link href="/console/instances">Add instance</Link>
                  </Button>
                }
              />
            ) : (
              <div className="divide-y">
                {instances.slice(0, 5).map((i) => (
                  <div key={i.instanceId} className="flex items-center justify-between py-2.5 text-sm">
                    <span className="flex items-center gap-2">
                      {i.agentName}
                      <span className="font-mono text-xs text-muted-foreground">{i.subdomain}</span>
                    </span>
                    <Badge variant="secondary">{i.mode}</Badge>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0">
            <CardTitle>Recent shares</CardTitle>
            <Button asChild variant="ghost" size="sm">
              <Link href="/console/links">View all</Link>
            </Button>
          </CardHeader>
          <CardContent>
            {links.length === 0 ? (
              <EmptyState title="No preview links yet" hint="Links are created by your agent via MCP." />
            ) : (
              <div className="divide-y">
                {links.map((l) => (
                  <div key={l.linkId} className="flex items-center justify-between py-2.5 text-sm">
                    <span className="truncate">{l.displayName || l.linkId}</span>
                    <span className="ml-2 shrink-0 text-xs text-muted-foreground">
                      {relativeTime(l.createdAt)}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
