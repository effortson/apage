"use client";
import { useEffect, useState } from "react";
import { api, ApiException } from "@/lib/api";
import { relativeTime } from "@/lib/format";
import {
  PageHeader,
  EmptyState,
  StatusBadge,
  CopyField,
} from "@/components/composites";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { toast } from "sonner";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";

export default function Domains() {
  const [items, setItems] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [add, setAdd] = useState(false);
  const [diag, setDiag] = useState<any>(null);

  const load = () => api<{ items: any[] }>("/custom-domains").then((r) => { setItems(r.items || []); setLoading(false); });
  useEffect(() => { load().catch(() => setLoading(false)); }, []);

  return (
    <div>
      <PageHeader
        title="Custom Domains"
        description="Serve previews from your own domain. Available on paid plans; subject to your plan's domain limit."
        actions={<Button onClick={() => setAdd(true)}>Add domain</Button>}
      />

      {loading ? (
        <Skeleton className="h-48 w-full" />
      ) : items.length === 0 ? (
        <EmptyState
          title="No custom domains"
          hint="Available on paid plans; subject to your plan's domain limit."
          action={<Button onClick={() => setAdd(true)}>Add domain</Button>}
        />
      ) : (
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Domain</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Certificate</TableHead>
                <TableHead>Last checked</TableHead>
                <TableHead className="w-28" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((d) => (
                <TableRow key={d.domainId}>
                  <TableCell className="font-mono text-xs">{d.domain}</TableCell>
                  <TableCell>
                    <StatusBadge status={d.status} />
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">{d.certStatus}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{relativeTime(d.lastCheckedAt)}</TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={async () => {
                        const r = await api<any>(`/custom-domains/${d.domainId}/verify`, { method: "POST" });
                        if (r.status === "verified") toast.success(`Status: ${r.status}`);
                        else toast.error(`Status: ${r.status}`);
                        if (r.checks) setDiag({ domain: d.domain, ...r });
                        load();
                      }}
                    >
                      Check DNS
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <AddDomain open={add} onOpenChange={setAdd} onDone={() => { setAdd(false); load(); }} />

      <Dialog open={!!diag} onOpenChange={(o) => !o && setDiag(null)}>
        <DialogContent className="sm:max-w-lg">
          {diag && (
            <>
              <DialogHeader>
                <DialogTitle>DNS check — {diag.domain}</DialogTitle>
              </DialogHeader>
              <div className="space-y-3">
                <p className="flex items-center gap-2 text-sm text-muted-foreground">
                  Status: <StatusBadge status={diag.status} /> · cert {diag.certStatus}
                </p>
                <DnsCheck label="TXT (ownership)" name={diag.checks.txt.name} expected={diag.checks.txt.expected} observed={undefined} ok={diag.checks.txt.ok} />
                <DnsCheck label="CNAME (routing)" name={diag.checks.cname.name} expected={diag.checks.cname.expected} observed={diag.checks.cname.observed} ok={diag.checks.cname.ok} />
              </div>
              <DialogFooter>
                <Button onClick={() => setDiag(null)}>Close</Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  );
}

function DnsCheck({ label, name, expected, observed, ok }: { label: string; name: string; expected: string; observed?: string; ok: boolean }) {
  return (
    <div className="rounded-md border p-3">
      <div className="text-sm font-semibold">
        {label}{" "}
        <span className={ok ? "text-green-600 dark:text-green-500" : "text-destructive"}>
          {ok ? "✓ ok" : "✗ not found"}
        </span>
      </div>
      <div className="mt-1 text-xs text-muted-foreground">
        Record: <code className="font-mono">{name}</code>
      </div>
      <div className="mt-1 text-xs">
        Expected: <code className="font-mono">{expected}</code>
      </div>
      {observed !== undefined && (
        <div className="mt-0.5 text-xs">
          Observed: <code className="font-mono">{observed || "(none)"}</code>
        </div>
      )}
    </div>
  );
}

function AddDomain({
  open,
  onOpenChange,
  onDone,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  onDone: () => void;
}) {
  const [domain, setDomain] = useState("");
  const [res, setRes] = useState<any>(null);
  const [loading, setLoading] = useState(false);

  // Reset the sheet each time it opens.
  useEffect(() => {
    if (open) {
      setDomain("");
      setRes(null);
    }
  }, [open]);

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full sm:max-w-md">
        {res ? (
          <>
            <SheetHeader>
              <SheetTitle>DNS records</SheetTitle>
              <SheetDescription>
                Add these records at your DNS provider, then click Check DNS.
              </SheetDescription>
            </SheetHeader>
            <div className="mt-6 space-y-4">
              <div className="space-y-1.5">
                <Label>TXT (ownership)</Label>
                <CopyField value={`${res.dns.txt.name}  TXT  ${res.dns.txt.value}`} />
              </div>
              <div className="space-y-1.5">
                <Label>CNAME</Label>
                <CopyField value={`${res.dns.cname.name}  CNAME  ${res.dns.cname.value}`} />
              </div>
              <Button onClick={onDone}>Done</Button>
            </div>
          </>
        ) : (
          <>
            <SheetHeader>
              <SheetTitle>Add custom domain</SheetTitle>
              <SheetDescription>
                Enter your domain; we&apos;ll issue the DNS records to verify ownership.
              </SheetDescription>
            </SheetHeader>
            <div className="mt-6 space-y-4">
              <div className="space-y-2">
                <Label htmlFor="domain">Domain</Label>
                <Input
                  id="domain"
                  value={domain}
                  onChange={(e) => setDomain(e.target.value)}
                  placeholder="preview.customer.com"
                />
              </div>
              <Button
                disabled={loading}
                onClick={async () => {
                  setLoading(true);
                  try {
                    const r = await api<any>("/custom-domains", { method: "POST", body: { domain } });
                    setRes(r);
                  } catch (e) {
                    toast.error(e instanceof ApiException ? e.body.message : "Failed");
                  } finally {
                    setLoading(false);
                  }
                }}
              >
                {loading ? "Adding…" : "Add domain"}
              </Button>
            </div>
          </>
        )}
      </SheetContent>
    </Sheet>
  );
}
