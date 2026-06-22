"use client";
import { useEffect, useState } from "react";
import { Card, Banner, Button, Input, ConfirmDialog, useToast } from "@/components/ui";
import { api } from "@/lib/api";

// Settings (UI §7.9): tenant profile, security pointer, compliance/data deletion,
// danger zone. Destructive actions require typed confirmation.
export default function Settings() {
  const toast = useToast();
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
      toast({ tone: "success", msg: "Tenant name updated" });
    } catch (e: any) {
      toast({ tone: "danger", msg: e.body?.message || "Update failed" });
    } finally {
      setSaving(false);
    }
  }

  return (
    <div>
      <h1 style={{ marginBottom: 16 }}>Settings</h1>

      <Card title="Tenant profile" style={{ marginBottom: 16 }}>
        <form onSubmit={saveName} style={{ display: "flex", gap: 8, alignItems: "flex-end" }}>
          <div style={{ flex: 1 }}>
            <Input label="Tenant name" value={name} onChange={(e) => setName(e.target.value)} required />
          </div>
          <Button type="submit" loading={saving}>Save</Button>
        </form>
      </Card>

      <Card title="Security — credentials" style={{ marginBottom: 16 }}>
        <p style={{ color: "var(--color-text-muted)", fontSize: 14 }}>
          Instance API keys and agent tokens are managed per instance. Create, rotate, or revoke them from
          <a href="/console/instances"> Instances → Details → Danger zone</a>. Rotating disconnects the agent;
          the old key keeps a short grace period.
        </p>
      </Card>

      <Card title="Notifications" style={{ marginBottom: 16 }}>
        <p style={{ color: "var(--color-text-muted)", fontSize: 14 }}>
          Quota warnings, security events, and abuse-freeze notices are sent to tenant admins by email.
        </p>
      </Card>

      <Card title={<span style={{ color: "var(--color-danger)" }}>Danger zone — delete tenant data</span>}>
        <p style={{ color: "var(--color-text-muted)", fontSize: 14 }}>
          Permanently delete all of this tenant&apos;s files, derivatives, preview links, and file-ref mappings
          (GDPR/CCPA). This is irreversible and requires typing <code>DELETE</code> to confirm.
        </p>
        <Banner tone="info">Deletion is recorded in the audit log (the confirmation, never the deleted content).</Banner>
        <Button variant="danger" onClick={() => setConfirmDelete(true)}>Delete tenant data</Button>
      </Card>

      {confirmDelete && (
        <ConfirmDialog
          title="Delete tenant data"
          danger
          confirmWord="DELETE"
          message="This permanently removes all files, derivatives, preview links, and file-ref mappings for this tenant. This cannot be undone."
          onCancel={() => setConfirmDelete(false)}
          onConfirm={async () => {
            try {
              const r = await api<any>("/data-deletion-requests", { method: "POST" });
              toast({ tone: "success", msg: `Deleted ${r.deleted.files} files, ${r.deleted.links} links` });
            } catch (e: any) {
              toast({ tone: "danger", msg: e.body?.message || "Failed" });
            }
            setConfirmDelete(false);
          }}
        />
      )}
    </div>
  );
}
