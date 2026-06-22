"use client";
import { useEffect, useState } from "react";
import { api, ApiException } from "@/lib/api";
import { relativeTime } from "@/lib/format";
import { PageHeader, EmptyState, ConfirmDialog } from "@/components/composites";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";

export default function Members() {
  const [items, setItems] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [invite, setInvite] = useState(false);
  const [remove, setRemove] = useState<any>(null);

  const load = () => api<{ items: any[] }>("/members").then((r) => { setItems(r.items || []); setLoading(false); });
  useEffect(() => { load().catch(() => setLoading(false)); }, []);

  return (
    <div>
      <PageHeader
        title="Members"
        description="Invite teammates and manage their roles."
        actions={<Button onClick={() => setInvite(true)}>Invite member</Button>}
      />

      {loading ? (
        <Skeleton className="h-48 w-full" />
      ) : items.length === 0 ? (
        <EmptyState
          title="No members"
          hint="Invite a teammate to collaborate in this tenant."
          action={<Button onClick={() => setInvite(true)}>Invite member</Button>}
        />
      ) : (
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Email</TableHead>
                <TableHead>Role</TableHead>
                <TableHead>Joined</TableHead>
                <TableHead className="w-24" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((m) => (
                <TableRow key={m.membershipId}>
                  <TableCell className="font-medium">{m.email}</TableCell>
                  <TableCell>
                    <Select
                      value={m.role}
                      onValueChange={async (v) => {
                        await api(`/members/${m.membershipId}`, { method: "PATCH", body: { role: v } })
                          .then(load)
                          .catch((err) => toast.error(err instanceof ApiException ? err.body.message : "Failed"));
                      }}
                    >
                      <SelectTrigger className="h-9 w-[140px]">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {["owner", "admin", "member", "viewer"].map((r) => (
                          <SelectItem key={r} value={r}>
                            {r}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">{relativeTime(m.createdAt)}</TableCell>
                  <TableCell className="text-right">
                    <Button variant="ghost" size="sm" onClick={() => setRemove(m)}>
                      Remove
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <div className="mt-8">
        <h3 className="mb-3 text-sm font-semibold">Role permissions</h3>
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-32">Role</TableHead>
                <TableHead>Can do</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow>
                <TableCell>
                  <Badge variant="success">owner</Badge>
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">Everything incl. billing, delete tenant, manage members</TableCell>
              </TableRow>
              <TableRow>
                <TableCell>
                  <Badge variant="info">admin</Badge>
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">Manage instances, links, files, domains; view audit; handle abuse</TableCell>
              </TableRow>
              <TableRow>
                <TableCell>
                  <Badge variant="secondary">member</Badge>
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">Create/revoke own links; view tenant resources</TableCell>
              </TableRow>
              <TableRow>
                <TableCell>
                  <Badge variant="secondary">viewer</Badge>
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">Read-only: instance status and link list</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>
      </div>

      <InviteSheet
        open={invite}
        onOpenChange={setInvite}
        onDone={() => { setInvite(false); toast.success("Invite sent"); }}
      />

      <ConfirmDialog
        open={!!remove}
        onOpenChange={(o) => !o && setRemove(null)}
        title="Remove member"
        destructive
        confirmLabel="Remove"
        description={`Remove ${remove?.email}? At least one owner must remain.`}
        onConfirm={async () => {
          const m = remove;
          await api(`/members/${m.membershipId}`, { method: "DELETE" })
            .then(() => { setRemove(null); load(); toast.success("Removed"); })
            .catch((e) => toast.error(e instanceof ApiException ? e.body.message : "Failed"));
        }}
      />
    </div>
  );
}

function InviteSheet({
  open,
  onOpenChange,
  onDone,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  onDone: () => void;
}) {
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("member");
  const [loading, setLoading] = useState(false);

  // Reset the sheet each time it opens.
  useEffect(() => {
    if (open) {
      setEmail("");
      setRole("member");
    }
  }, [open]);

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full sm:max-w-md">
        <SheetHeader>
          <SheetTitle>Invite member</SheetTitle>
          <SheetDescription>Send an invite by email and assign a role.</SheetDescription>
        </SheetHeader>
        <div className="mt-6 space-y-4">
          <div className="space-y-2">
            <Label htmlFor="email">Email</Label>
            <Input
              id="email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label>Role</Label>
            <Select value={role} onValueChange={setRole}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {["admin", "member", "viewer", "owner"].map((r) => (
                  <SelectItem key={r} value={r}>
                    {r}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <Button
            disabled={loading}
            onClick={async () => {
              setLoading(true);
              try {
                await api("/members/invite", { method: "POST", body: { email, role } });
                onDone();
              } catch (e) {
                toast.error(e instanceof ApiException ? e.body.message : "Failed");
              } finally {
                setLoading(false);
              }
            }}
          >
            {loading ? "Sending…" : "Send invite"}
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}
