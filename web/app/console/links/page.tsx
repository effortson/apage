"use client";
import { useEffect, useState } from "react";
import { api, List } from "@/lib/api";
import { usePoll } from "@/lib/hooks";
import { relativeTime, absoluteTime } from "@/lib/format";
import { PageHeader, EmptyState, StatusBadge, ConfirmDialog } from "@/components/composites";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Skeleton } from "@/components/ui/skeleton";
import { toast } from "sonner";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

function linkStatus(l: any): string {
  if (l.frozenAt) return "frozen";
  if (l.revokedAt) return "revoked";
  if (l.expiresAt && new Date(l.expiresAt) < new Date()) return "expired";
  return "active";
}

function policyLabel(p: any): string {
  if (!p) return "public";
  const parts = [p.type];
  if (p.singleUse) parts.push("single-use");
  else if (p.maxViews) parts.push(`max ${p.maxViews}`);
  if (p.ipAllowlist?.length) parts.push("ip");
  return parts.join(" · ");
}

export default function Links() {
  const [items, setItems] = useState<any[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [status, setStatus] = useState("");
  const [loading, setLoading] = useState(true);
  const [revoke, setRevoke] = useState<any>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [batchRevoke, setBatchRevoke] = useState(false);

  const toggle = (id: string) =>
    setSelected((s) => {
      const n = new Set(s);
      n.has(id) ? n.delete(id) : n.add(id);
      return n;
    });

  const load = (reset = true) => {
    const q = new URLSearchParams({ limit: "20" });
    if (status) q.set("status", status);
    if (!reset && cursor) q.set("cursor", cursor);
    return api<List<any>>(`/preview-links?${q}`).then((r) => {
      setItems(reset ? r.items || [] : [...items, ...(r.items || [])]);
      setCursor(r.nextCursor);
      setLoading(false);
    });
  };
  useEffect(() => {
    setLoading(true);
    load(true).catch(() => setLoading(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [status]);
  usePoll(() => {
    load(true).catch(() => {});
  }, 5000);

  return (
    <div>
      <PageHeader
        title="Preview Links"
        description="Created by your agent via MCP. View, revoke, or freeze them here."
      />

      <div className="mb-4 flex items-center gap-2">
        <Select value={status || "all"} onValueChange={(v) => setStatus(v === "all" ? "" : v)}>
          <SelectTrigger className="h-9 w-[160px]">
            <SelectValue placeholder="All statuses" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All statuses</SelectItem>
            <SelectItem value="active">Active</SelectItem>
            <SelectItem value="revoked">Revoked</SelectItem>
            <SelectItem value="expired">Expired</SelectItem>
            <SelectItem value="frozen">Frozen</SelectItem>
          </SelectContent>
        </Select>
        {selected.size > 0 && (
          <Button variant="destructive" onClick={() => setBatchRevoke(true)}>
            Revoke selected ({selected.size})
          </Button>
        )}
      </div>

      {loading ? (
        <Skeleton className="h-64 w-full" />
      ) : items.length === 0 ? (
        <EmptyState title="No preview links" hint="Links are created by your agent via MCP." />
      ) : (
        <>
          <div className="rounded-lg border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-10" />
                  <TableHead>Name</TableHead>
                  <TableHead>Link ID</TableHead>
                  <TableHead>Policy</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Expires</TableHead>
                  <TableHead className="text-right">Views</TableHead>
                  <TableHead className="w-20" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((l) => {
                  const st = linkStatus(l);
                  return (
                    <TableRow key={l.linkId}>
                      <TableCell>
                        {st === "active" && (
                          <Checkbox
                            aria-label={`select ${l.linkId}`}
                            checked={selected.has(l.linkId)}
                            onCheckedChange={() => toggle(l.linkId)}
                          />
                        )}
                      </TableCell>
                      <TableCell className="font-medium">{l.displayName || "—"}</TableCell>
                      <TableCell className="font-mono text-xs text-muted-foreground">{l.linkId}</TableCell>
                      <TableCell className="text-sm text-muted-foreground">{policyLabel(l.accessPolicy)}</TableCell>
                      <TableCell>
                        <StatusBadge status={st} />
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground" title={absoluteTime(l.expiresAt)}>
                        {relativeTime(l.expiresAt)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">{l.viewCount}</TableCell>
                      <TableCell className="text-right">
                        {st === "active" && (
                          <Button variant="ghost" size="sm" onClick={() => setRevoke(l)}>
                            Revoke
                          </Button>
                        )}
                      </TableCell>
                    </TableRow>
                  );
                })}
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

      <ConfirmDialog
        open={!!revoke}
        onOpenChange={(o) => !o && setRevoke(null)}
        title="Revoke link"
        destructive
        confirmLabel="Revoke"
        description={`Revoke "${revoke?.displayName || revoke?.linkId}"? Visitors will get a 410 within seconds.`}
        onConfirm={async () => {
          const l = revoke;
          setRevoke(null);
          await api(`/preview-links/${l.linkId}/revoke`, { method: "POST" });
          load(true);
          toast.success("Link revoked", { description: "Audit logged." });
        }}
      />
      <ConfirmDialog
        open={batchRevoke}
        onOpenChange={setBatchRevoke}
        title="Revoke selected links"
        destructive
        confirmLabel="Revoke all"
        confirmWord="REVOKE"
        description={`Revoke ${selected.size} selected link(s)? Visitors will get a 410 within seconds. This cannot be undone.`}
        onConfirm={async () => {
          const ids = Array.from(selected);
          setBatchRevoke(false);
          await Promise.allSettled(ids.map((id) => api(`/preview-links/${id}/revoke`, { method: "POST" })));
          setSelected(new Set());
          load(true);
          toast.success(`Revoked ${ids.length} link(s)`, { description: "Audit logged." });
        }}
      />
    </div>
  );
}
