"use client";
import { useEffect, useState } from "react";
import { api, List, ApiException, idemKey } from "@/lib/api";
import { usePoll } from "@/lib/hooks";
import {
  PageHeader,
  EmptyState,
  SecretReveal,
  CodeBlock,
} from "@/components/composites";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Separator } from "@/components/ui/separator";
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
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

const API_BASE = "https://preview.example.com";

export default function Instances() {
  const [items, setItems] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAdd, setShowAdd] = useState(false);
  const [detail, setDetail] = useState<any>(null);

  const load = () =>
    api<List<any>>("/instances?limit=50").then((r) => {
      setItems(r.items || []);
      setLoading(false);
    });
  useEffect(() => {
    load().catch(() => setLoading(false));
  }, []);
  usePoll(() => {
    load().catch(() => {});
  }, 5000);

  return (
    <div>
      <PageHeader
        title="Instances"
        description="Each instance is a subdomain + API key your agent uses to share files."
        actions={<Button onClick={() => setShowAdd(true)}>Add instance</Button>}
      />

      {loading ? (
        <Skeleton className="h-48 w-full" />
      ) : items.length === 0 ? (
        <EmptyState
          title="No instances"
          hint="Add an instance, then run apage-cli to connect your agent."
          action={<Button onClick={() => setShowAdd(true)}>Add instance</Button>}
        />
      ) : (
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Subdomain</TableHead>
                <TableHead>Mode</TableHead>
                <TableHead className="w-24" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((i) => (
                <TableRow key={i.instanceId}>
                  <TableCell className="font-medium">{i.agentName}</TableCell>
                  <TableCell className="capitalize text-muted-foreground">{i.agentType}</TableCell>
                  <TableCell className="font-mono text-xs">{i.subdomain}</TableCell>
                  <TableCell>
                    <Badge variant="secondary">{i.mode}</Badge>
                  </TableCell>
                  <TableCell className="text-right">
                    <Button variant="ghost" size="sm" onClick={() => setDetail(i)}>
                      Details
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <AddInstance open={showAdd} onOpenChange={setShowAdd} onCreated={() => load()} />
      <InstanceDetail instance={detail} onOpenChange={(o) => !o && setDetail(null)} />
    </div>
  );
}

function AddInstance({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  onCreated: () => void;
}) {
  const [form, setForm] = useState({ agentName: "", subdomain: "", agentType: "openclaw" });
  const [created, setCreated] = useState<any>(null);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);

  // Reset the dialog each time it opens.
  useEffect(() => {
    if (open) {
      setForm({ agentName: "", subdomain: "", agentType: "openclaw" });
      setCreated(null);
      setErr("");
    }
  }, [open]);

  async function submit() {
    setErr("");
    setLoading(true);
    try {
      const c = await api<any>("/instances", {
        method: "POST",
        body: form,
        idempotencyKey: idemKey("inst"),
      });
      setCreated(c);
      onCreated();
      toast.success("Instance created");
    } catch (e) {
      setErr(e instanceof ApiException ? e.body.message : "Failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        {created ? (
          <>
            <DialogHeader>
              <DialogTitle>Instance created</DialogTitle>
              <DialogDescription>
                Save the API key now — it configures apage-cli and is shown only once.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4">
              <div className="space-y-1.5">
                <Label>Instance API key</Label>
                <SecretReveal value={created.instanceApiKey} />
              </div>
              <div className="space-y-1.5">
                <Label>Connect your agent</Label>
                <CodeBlock>{`apage-cli init --instance ${created.subdomain || form.subdomain} --workspace ~/outputs --api ${API_BASE} --api-key ${created.instanceApiKey}
apage-cli mcp`}</CodeBlock>
                <p className="text-sm text-muted-foreground">
                  <code className="font-mono">apage-cli mcp</code> starts a local MCP server on{" "}
                  <code className="font-mono">127.0.0.1:7777</code>; your agent calls its tools to
                  upload files and create preview links.
                </p>
              </div>
            </div>
            <DialogFooter>
              <Button onClick={() => onOpenChange(false)}>Done</Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>Add instance</DialogTitle>
              <DialogDescription>
                Pick a subdomain; we&apos;ll issue an instance API key for your agent.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="agentName">Agent name</Label>
                <Input
                  id="agentName"
                  value={form.agentName}
                  onChange={(e) => setForm({ ...form, agentName: e.target.value })}
                  placeholder="Alice's agent"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="subdomain">Subdomain</Label>
                <Input
                  id="subdomain"
                  value={form.subdomain}
                  onChange={(e) => setForm({ ...form, subdomain: e.target.value })}
                  placeholder="alice"
                />
              </div>
              <div className="space-y-2">
                <Label>Agent type</Label>
                <Select
                  value={form.agentType}
                  onValueChange={(v) => setForm({ ...form, agentType: v })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="openclaw">OpenClaw</SelectItem>
                    <SelectItem value="hermes">Hermes</SelectItem>
                    <SelectItem value="custom">Custom</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              {err && (
                <Alert variant="destructive">
                  <AlertDescription>{err}</AlertDescription>
                </Alert>
              )}
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button onClick={submit} disabled={loading}>
                {loading ? "Creating…" : "Create instance"}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

function InstanceDetail({
  instance,
  onOpenChange,
}: {
  instance: any;
  onOpenChange: (o: boolean) => void;
}) {
  const [rotated, setRotated] = useState<string | null>(null);

  useEffect(() => {
    setRotated(null);
  }, [instance]);

  if (!instance) return null;
  return (
    <Sheet open={!!instance} onOpenChange={onOpenChange}>
      <SheetContent className="w-full sm:max-w-md">
        <SheetHeader>
          <SheetTitle>{instance.agentName}</SheetTitle>
          <SheetDescription className="font-mono text-xs">{instance.instanceId}</SheetDescription>
        </SheetHeader>

        <div className="mt-6 space-y-3 text-sm">
          <Row k="Subdomain" v={instance.subdomain} mono />
          <Row k="Type" v={instance.agentType} />
          <Row k="Mode" v={instance.mode} />
        </div>

        <Separator className="my-6" />

        <h3 className="text-sm font-semibold text-destructive">Danger zone</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Rotating issues a new instance API key; the previous key keeps working briefly during a
          grace window.
        </p>
        <Button
          variant="outline"
          size="sm"
          className="mt-3"
          onClick={async () => {
            const c = await api<any>(`/instances/${instance.instanceId}/rotate-credentials`, {
              method: "POST",
            });
            setRotated(c.instanceApiKey);
            toast.success("Rotated — new API key issued");
          }}
        >
          Rotate credentials
        </Button>
        {rotated && (
          <div className="mt-4">
            <SecretReveal value={rotated} />
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}

function Row({ k, v, mono }: { k: string; v: string; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between">
      <span className="text-muted-foreground">{k}</span>
      <span className={mono ? "font-mono text-xs" : "capitalize"}>{v}</span>
    </div>
  );
}
