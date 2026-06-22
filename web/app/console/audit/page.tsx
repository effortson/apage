"use client";
import { useEffect, useState } from "react";
import { api, List } from "@/lib/api";
import { absoluteTime, relativeTime } from "@/lib/format";
import { PageHeader, EmptyState } from "@/components/composites";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

export default function Audit() {
  const [items, setItems] = useState<any[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [event, setEvent] = useState("");
  const [loading, setLoading] = useState(true);

  const load = (reset = true) => {
    const q = new URLSearchParams({ limit: "50" });
    if (event) q.set("event", event);
    if (!reset && cursor) q.set("cursor", cursor);
    return api<List<any>>(`/audit-logs?${q}`).then((r) => {
      setItems(reset ? r.items || [] : [...items, ...(r.items || [])]);
      setCursor(r.nextCursor);
      setLoading(false);
    });
  };
  useEffect(() => {
    setLoading(true);
    load(true).catch(() => setLoading(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [event]);

  return (
    <div>
      <PageHeader
        title="Audit Logs"
        description="Secrets are always redacted. Visible to admins and owners."
      />

      <div className="mb-4 max-w-xs space-y-1.5">
        <Label htmlFor="event">Filter by event</Label>
        <Input
          id="event"
          value={event}
          onChange={(e) => setEvent(e.target.value)}
          placeholder="preview_link.accessed"
        />
      </div>

      {loading ? (
        <Skeleton className="h-64 w-full" />
      ) : items.length === 0 ? (
        <EmptyState title="No audit events" hint="Activity in your tenant will appear here." />
      ) : (
        <>
          <div className="rounded-lg border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Time</TableHead>
                  <TableHead>Event</TableHead>
                  <TableHead>Actor</TableHead>
                  <TableHead>Resource</TableHead>
                  <TableHead>IP</TableHead>
                  <TableHead>Reason</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((a) => (
                  <TableRow key={a.eventId}>
                    <TableCell className="text-sm text-muted-foreground" title={absoluteTime(a.createdAt)}>
                      {relativeTime(a.createdAt)}
                    </TableCell>
                    <TableCell>
                      <Badge variant="secondary">{a.event}</Badge>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {a.actorType}
                      {a.actorId ? ` (${a.actorId.slice(0, 10)}…)` : ""}
                    </TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {a.resourceType}
                      {a.resourceId ? `/${a.resourceId.slice(0, 12)}…` : ""}
                    </TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{a.ip || "—"}</TableCell>
                    <TableCell className="text-sm text-muted-foreground">{a.reason || "—"}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          {cursor && (
            <div className="mt-4">
              <Button variant="outline" onClick={() => load(false)}>
                Load more
              </Button>
            </div>
          )}
        </>
      )}
    </div>
  );
}
