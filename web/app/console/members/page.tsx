"use client";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { Button, Table, Td, Badge, Drawer, Input, Select, Skeleton, useToast, ConfirmDialog } from "@/components/ui";
import { relativeTime } from "@/lib/format";

export default function Members() {
  const toast = useToast();
  const [items, setItems] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [invite, setInvite] = useState(false);
  const [remove, setRemove] = useState<any>(null);

  const load = () => api<{ items: any[] }>("/members").then((r) => { setItems(r.items || []); setLoading(false); });
  useEffect(() => { load().catch(() => setLoading(false)); }, []);

  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 16 }}>
        <h1>Members</h1>
        <Button onClick={() => setInvite(true)}>Invite member</Button>
      </div>
      {loading ? <Skeleton rows={3} /> : (
        <Table head={["Email", "Role", "Joined", ""]}>
          {items.map((m) => (
            <tr key={m.membershipId}>
              <Td>{m.email}</Td>
              <Td>
                <Select value={m.role} onChange={async (e) => { await api(`/members/${m.membershipId}`, { method: "PATCH", body: { role: e.target.value } }).then(load).catch((err) => toast({ tone: "danger", msg: err.body?.message || "Failed" })); }}>
                  {["owner", "admin", "member", "viewer"].map((r) => <option key={r} value={r}>{r}</option>)}
                </Select>
              </Td>
              <Td>{relativeTime(m.createdAt)}</Td>
              <Td><Button size="sm" variant="ghost" onClick={() => setRemove(m)}>Remove</Button></Td>
            </tr>
          ))}
        </Table>
      )}
      <div style={{ marginTop: 24 }}>
        <h3>Role permissions</h3>
        <Table head={["Role", "Can do"]}>
          <tr><Td><Badge tone="success">owner</Badge></Td><Td>Everything incl. billing, delete tenant, manage members</Td></tr>
          <tr><Td><Badge tone="info">admin</Badge></Td><Td>Manage instances, links, files, domains; view audit; handle abuse</Td></tr>
          <tr><Td><Badge tone="muted">member</Badge></Td><Td>Create/revoke own links; view tenant resources</Td></tr>
          <tr><Td><Badge tone="muted">viewer</Badge></Td><Td>Read-only: instance status and link list</Td></tr>
        </Table>
      </div>

      {invite && <InviteDrawer onClose={() => setInvite(false)} onDone={() => { setInvite(false); toast({ tone: "success", msg: "Invite sent" }); }} />}
      {remove && (
        <ConfirmDialog title="Remove member" danger message={`Remove ${remove.email}? At least one owner must remain.`}
          onCancel={() => setRemove(null)}
          onConfirm={async () => { await api(`/members/${remove.membershipId}`, { method: "DELETE" }).then(() => { setRemove(null); load(); toast({ tone: "success", msg: "Removed" }); }).catch((e) => toast({ tone: "danger", msg: e.body?.message || "Failed" })); }} />
      )}
    </div>
  );
}

function InviteDrawer({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const toast = useToast();
  const [email, setEmail] = useState(""); const [role, setRole] = useState("member"); const [loading, setLoading] = useState(false);
  return (
    <Drawer title="Invite member" onClose={onClose}>
      <Input label="Email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
      <Select label="Role" value={role} onChange={(e) => setRole(e.target.value)}>
        {["admin", "member", "viewer", "owner"].map((r) => <option key={r} value={r}>{r}</option>)}
      </Select>
      <Button loading={loading} onClick={async () => {
        setLoading(true);
        try { await api("/members/invite", { method: "POST", body: { email, role } }); onDone(); }
        catch (e: any) { toast({ tone: "danger", msg: e.body?.message || "Failed" }); }
        finally { setLoading(false); }
      }}>Send invite</Button>
    </Drawer>
  );
}
