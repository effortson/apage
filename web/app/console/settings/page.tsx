"use client";
import { useState } from "react";
import { Card, Banner, Button, ConfirmDialog, useToast } from "@/components/ui";
import { api } from "@/lib/api";

// Settings (UI §7.9). MVP shows profile/security/compliance/danger sections;
// destructive actions require typed confirmation (wired where backend exists).
export default function Settings() {
  const toast = useToast();
  const [confirmDelete, setConfirmDelete] = useState(false);
  return (
    <div>
      <h1 style={{ marginBottom: 16 }}>Settings</h1>

      <Card title="Security — credentials" style={{ marginBottom: 16 }}>
        <p style={{ color: "var(--color-text-muted)", fontSize: 14 }}>
          Instance API keys and agent tokens are managed per instance. Create, rotate, or revoke them from
          <a href="/console/instances"> Instances → Details → Danger zone</a>. Rotating disconnects the agent;
          the old key keeps a short grace period.
        </p>
      </Card>

      <Card title="Data & compliance" style={{ marginBottom: 16 }}>
        <p style={{ color: "var(--color-text-muted)", fontSize: 14 }}>
          Tenant data is stored in your home region. You may request data deletion (GDPR/CCPA); deletion removes
          original files, derivatives, preview links, and file-ref mappings, then issues a deletion confirmation.
        </p>
        <Banner tone="info">Deletion is recorded in the audit log (the confirmation, never the deleted content).</Banner>
        <Button variant="danger" onClick={() => setConfirmDelete(true)}>Request data deletion</Button>
      </Card>

      <Card title="Notifications" style={{ marginBottom: 16 }}>
        <p style={{ color: "var(--color-text-muted)", fontSize: 14 }}>
          Quota warnings, security events, and abuse-freeze notices are sent to tenant admins by email.
        </p>
      </Card>

      <Card title={<span style={{ color: "var(--color-danger)" }}>Danger zone</span>}>
        <p style={{ color: "var(--color-text-muted)", fontSize: 14 }}>
          Deleting a tenant is irreversible and requires typing the tenant name to confirm (owner only).
        </p>
      </Card>

      {confirmDelete && (
        <ConfirmDialog title="Request data deletion" danger confirmWord="DELETE"
          message="This permanently removes all files, derivatives, preview links, and file-ref mappings for this tenant. This cannot be undone."
          onCancel={() => setConfirmDelete(false)}
          onConfirm={async () => {
            try {
              const r = await api<any>("/data-deletion-requests", { method: "POST" });
              toast({ tone: "success", msg: `Deleted ${r.deleted.files} files, ${r.deleted.links} links` });
            } catch (e: any) { toast({ tone: "danger", msg: e.body?.message || "Failed" }); }
            setConfirmDelete(false);
          }} />
      )}
    </div>
  );
}
