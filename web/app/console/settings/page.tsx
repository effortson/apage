"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api";
import { PageHeader, ConfirmDialog } from "@/components/composites";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { toast } from "sonner";

// Settings (UI §7.9): tenant profile, security pointer, compliance/data deletion,
// danger zone. Destructive actions require typed confirmation.
export default function Settings() {
  const [name, setName] = useState("");
  const [saving, setSaving] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);

  useEffect(() => {
    api<{ tenants: { tenantId: string; name: string }[] }>("/auth/session", { tenant: false })
      .then((s) => {
        const t = typeof window !== "undefined" ? localStorage.getItem("apage_tenant") : null;
        setName(s.tenants.find((x) => x.tenantId === t)?.name || s.tenants[0]?.name || "");
      })
      .catch(() => {});
  }, []);

  async function saveName(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    try {
      await api("/tenant", { method: "PATCH", body: { name } });
      toast.success("Tenant name updated");
    } catch (e: any) {
      toast.error(e.body?.message || "Update failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div>
      <PageHeader title="Settings" />

      <Card className="mb-6">
        <CardHeader>
          <CardTitle>Tenant profile</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={saveName} className="flex items-end gap-2">
            <div className="flex-1 space-y-2">
              <Label htmlFor="tenant-name">Tenant name</Label>
              <Input
                id="tenant-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </div>
            <Button type="submit" disabled={saving}>
              {saving ? "Saving…" : "Save"}
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card className="mb-6">
        <CardHeader>
          <CardTitle>Security — credentials</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            Instance API keys and agent tokens are managed per instance. Create, rotate, or revoke
            them from{" "}
            <Link href="/console/instances" className="underline underline-offset-4">
              Instances → Details → Danger zone
            </Link>
            . Rotating disconnects the agent; the old key keeps a short grace period.
          </p>
        </CardContent>
      </Card>

      <Card className="mb-6">
        <CardHeader>
          <CardTitle>Notifications</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            Quota warnings, security events, and abuse-freeze notices are sent to tenant admins by
            email.
          </p>
        </CardContent>
      </Card>

      <Card className="border-destructive/50">
        <CardHeader>
          <CardTitle className="text-destructive">Danger zone — delete tenant data</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">
            Permanently delete all of this tenant&apos;s files, derivatives, preview links, and
            file-ref mappings (GDPR/CCPA). This is irreversible and requires typing{" "}
            <code className="font-mono">DELETE</code> to confirm.
          </p>
          <Alert>
            <AlertDescription>
              Deletion is recorded in the audit log (the confirmation, never the deleted content).
            </AlertDescription>
          </Alert>
          <Button variant="destructive" onClick={() => setConfirmDelete(true)}>
            Delete tenant data
          </Button>
        </CardContent>
      </Card>

      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title="Delete tenant data"
        destructive
        confirmLabel="Delete"
        confirmWord="DELETE"
        description="This permanently removes all files, derivatives, preview links, and file-ref mappings for this tenant. This cannot be undone."
        onConfirm={async () => {
          setConfirmDelete(false);
          try {
            const r = await api<any>("/data-deletion-requests", { method: "POST" });
            toast.success(`Deleted ${r.deleted.files} files, ${r.deleted.links} links`);
          } catch (e: any) {
            toast.error(e.body?.message || "Failed");
          }
        }}
      />
    </div>
  );
}
