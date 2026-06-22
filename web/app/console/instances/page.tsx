"use client";
import { useEffect, useState } from "react";
import { api, List, ApiException, idemKey } from "@/lib/api";
import { usePoll } from "@/lib/hooks";
import { Button, Table, Td, StatusDot, Badge, Drawer, Input, Select, EmptyState, Skeleton, SecretReveal, CopyField, CodeBlock, useToast, ConfirmDialog } from "@/components/ui";
import { relativeTime } from "@/lib/format";

export default function Instances() {
  const toast = useToast();
  const [items, setItems] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAdd, setShowAdd] = useState(false);
  const [created, setCreated] = useState<any>(null);
  const [detail, setDetail] = useState<any>(null);

  const load = () => api<List<any>>("/instances?limit=50").then((r) => { setItems(r.items || []); setLoading(false); });
  useEffect(() => { load().catch(() => setLoading(false)); }, []);
  usePoll(() => { load().catch(() => {}); }, 5000); // reflect online status ≤5s (UI §4.5)

  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 16 }}>
        <h1>Instances</h1>
        <Button onClick={() => { setCreated(null); setShowAdd(true); }}>Add instance</Button>
      </div>

      {loading ? <Skeleton rows={4} /> : items.length === 0 ? (
        <EmptyState title="No instances" hint="Add an instance to connect a preview agent." action={<Button onClick={() => setShowAdd(true)}>Add instance</Button>} />
      ) : (
        <Table head={["Name", "Type", "Subdomain", "Mode", "Status", "Version", "Last seen", ""]}>
          {items.map((i) => (
            <tr key={i.instanceId}>
              <Td>{i.agentName}</Td>
              <Td>{i.agentType}</Td>
              <Td mono>{i.subdomain}</Td>
              <Td><Badge tone={i.mode === "tunnel" ? "info" : "muted"}>{i.mode}</Badge></Td>
              <Td><StatusDot online={i.status === "online"} /></Td>
              <Td mono>{i.agentVersion || "—"}</Td>
              <Td>{relativeTime(i.lastSeenAt)}</Td>
              <Td><Button size="sm" variant="ghost" onClick={() => setDetail(i)}>Details</Button></Td>
            </tr>
          ))}
        </Table>
      )}

      {showAdd && (
        <AddInstance onClose={() => setShowAdd(false)} created={created}
          onCreated={(c) => { setCreated(c); load(); toast({ tone: "success", msg: "Instance created" }); }} />
      )}
      {detail && <InstanceDetail instance={detail} onClose={() => setDetail(null)} onChange={load} />}
    </div>
  );
}

function AddInstance({ onClose, onCreated, created }: { onClose: () => void; onCreated: (c: any) => void; created: any }) {
  const toast = useToast();
  const [form, setForm] = useState({ agentName: "", subdomain: "", agentType: "openclaw", mode: "tunnel" });
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);

  async function submit() {
    setErr(""); setLoading(true);
    try {
      const c = await api("/instances", { method: "POST", body: form, idempotencyKey: idemKey("inst") });
      onCreated(c);
    } catch (e) { setErr(e instanceof ApiException ? e.body.message : "Failed"); toast({ tone: "danger", msg: "Create failed" }); }
    finally { setLoading(false); }
  }

  return (
    <Drawer title={created ? "Instance created" : "Add instance"} onClose={onClose}>
      {created ? (
        <div>
          <p>Save these credentials now — they are shown only once.</p>
          <h3 style={{ marginTop: 16 }}>Agent token</h3>
          <SecretReveal value={created.agentToken} />
          <h3 style={{ marginTop: 16 }}>Instance API key</h3>
          <SecretReveal value={created.instanceApiKey} />
          <h3 style={{ marginTop: 16 }}>Install & start</h3>
          <CodeBlock>{`apage-agent init --instance ${form.subdomain || created.subdomain} --agent-type ${form.agentType} --workspace ~/outputs
apage-agent start --token ${created.agentToken}`}</CodeBlock>
          <div style={{ marginTop: 16 }}><Button onClick={onClose}>Done</Button></div>
        </div>
      ) : (
        <div>
          <Input label="Agent name" value={form.agentName} onChange={(e) => setForm({ ...form, agentName: e.target.value })} />
          <Input label="Subdomain" value={form.subdomain} onChange={(e) => setForm({ ...form, subdomain: e.target.value })} placeholder="alice" />
          <Select label="Agent type" value={form.agentType} onChange={(e) => setForm({ ...form, agentType: e.target.value })}>
            <option value="openclaw">OpenClaw</option>
            <option value="hermes">Hermes</option>
            <option value="custom">Custom</option>
          </Select>
          <Select label="Mode" value={form.mode} onChange={(e) => setForm({ ...form, mode: e.target.value })}>
            <option value="tunnel">Tunnel</option>
            <option value="cloud">Cloud</option>
            <option value="hybrid">Hybrid</option>
          </Select>
          {err && <div style={{ color: "var(--color-danger)", fontSize: 13, marginBottom: 12 }}>{err}</div>}
          <Button loading={loading} onClick={submit}>Create instance</Button>
        </div>
      )}
    </Drawer>
  );
}

function InstanceDetail({ instance, onClose, onChange }: { instance: any; onClose: () => void; onChange: () => void }) {
  const toast = useToast();
  const [detail, setDetail] = useState<any>(null);
  const [confirm, setConfirm] = useState(false);
  useEffect(() => { api<any>(`/instances/${instance.instanceId}`).then(setDetail).catch(() => {}); }, [instance.instanceId]);

  return (
    <Drawer title={instance.agentName} onClose={onClose}>
      <div style={{ display: "grid", gap: 8 }}>
        <Field k="Instance ID" v={instance.instanceId} mono />
        <Field k="Subdomain" v={instance.subdomain} mono />
        <Field k="Mode" v={instance.mode} />
        <Field k="Version" v={instance.agentVersion || "—"} />
        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <span style={{ color: "var(--color-text-muted)" }}>Connection</span>
          <StatusDot online={detail?.connection?.online} />
        </div>
      </div>
      <h3 style={{ marginTop: 16 }}>Allowlist</h3>
      <p style={{ fontSize: 13, color: "var(--color-text-muted)" }}>
        The allowlist is configured on the customer server and reported by the agent; the console cannot widen it remotely.
      </p>
      <Button size="sm" variant="secondary" onClick={async () => {
        await api(`/instances/${instance.instanceId}/allowlist-change-request`, { method: "POST" });
        toast({ tone: "info", msg: "Change request generated — confirm on the host" });
      }}>Request allowlist change</Button>

      <h3 style={{ marginTop: 24, color: "var(--color-danger)" }}>Danger zone</h3>
      <div style={{ display: "flex", gap: 8 }}>
        <Button size="sm" variant="secondary" onClick={async () => {
          const c = await api<any>(`/instances/${instance.instanceId}/rotate-credentials`, { method: "POST" });
          toast({ tone: "success", msg: "Rotated — new credentials issued" });
          alert("New agent token:\n" + c.agentToken + "\n\nNew API key:\n" + c.instanceApiKey);
        }}>Rotate credentials</Button>
        <Button size="sm" variant="danger" onClick={() => setConfirm(true)}>Revoke token</Button>
      </div>
      {confirm && (
        <ConfirmDialog title="Revoke agent token" danger confirmWord={instance.subdomain}
          message="This immediately disconnects the agent and invalidates its token."
          onCancel={() => setConfirm(false)}
          onConfirm={async () => {
            await api(`/instances/${instance.instanceId}/revoke-token`, { method: "POST" });
            setConfirm(false); onChange(); toast({ tone: "success", msg: "Token revoked (audit logged)" });
          }} />
      )}
    </Drawer>
  );
}

function Field({ k, v, mono }: { k: string; v: string; mono?: boolean }) {
  return (
    <div style={{ display: "flex", justifyContent: "space-between" }}>
      <span style={{ color: "var(--color-text-muted)" }}>{k}</span>
      <span className={mono ? "mono" : ""}>{v}</span>
    </div>
  );
}
