"use client";
import { useEffect, useState } from "react";
import { api, List, ApiException, idemKey } from "@/lib/api";
import { Button, Table, Td, Badge, Drawer, Input, Select, EmptyState, Skeleton, SecretReveal, useToast, ConfirmDialog, statusTone } from "@/components/ui";
import { relativeTime, absoluteTime } from "@/lib/format";

function linkStatus(l: any): string {
  if (l.frozenAt) return "frozen";
  if (l.revokedAt) return "revoked";
  if (l.expiresAt && new Date(l.expiresAt) < new Date()) return "expired";
  return "active";
}

export default function Links() {
  const toast = useToast();
  const [items, setItems] = useState<any[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [filter, setFilter] = useState({ status: "", mode: "" });
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [revoke, setRevoke] = useState<any>(null);

  const load = (reset = true) => {
    const q = new URLSearchParams({ limit: "20" });
    if (filter.status) q.set("status", filter.status);
    if (filter.mode) q.set("mode", filter.mode);
    if (!reset && cursor) q.set("cursor", cursor);
    return api<List<any>>(`/preview-links?${q}`).then((r) => {
      setItems(reset ? r.items || [] : [...items, ...(r.items || [])]);
      setCursor(r.nextCursor); setLoading(false);
    });
  };
  useEffect(() => { setLoading(true); load(true).catch(() => setLoading(false)); /* eslint-disable-next-line */ }, [filter]);

  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 16 }}>
        <h1>Preview Links</h1>
        <Button onClick={() => setShowCreate(true)}>Create link</Button>
      </div>
      <div style={{ display: "flex", gap: 8, marginBottom: 16 }}>
        <Select value={filter.status} onChange={(e) => setFilter({ ...filter, status: e.target.value })}>
          <option value="">All statuses</option><option value="active">Active</option><option value="revoked">Revoked</option><option value="expired">Expired</option><option value="frozen">Frozen</option>
        </Select>
        <Select value={filter.mode} onChange={(e) => setFilter({ ...filter, mode: e.target.value })}>
          <option value="">All modes</option><option value="tunnel">Tunnel</option><option value="cloud">Cloud</option>
        </Select>
      </div>

      {loading ? <Skeleton rows={5} /> : items.length === 0 ? (
        <EmptyState title="No preview links" hint="Create a tunnel or cloud preview link." action={<Button onClick={() => setShowCreate(true)}>Create link</Button>} />
      ) : (
        <>
          <Table head={["Name", "Link ID", "Mode", "Policy", "Status", "Expires", "Views", ""]}>
            {items.map((l) => {
              const st = linkStatus(l);
              return (
                <tr key={l.linkId}>
                  <Td>{l.displayName || "—"}</Td>
                  <Td mono>{l.linkId}</Td>
                  <Td><Badge tone={l.mode === "tunnel" ? "info" : "muted"}>{l.mode}</Badge></Td>
                  <Td>{policyLabel(l.accessPolicy)}</Td>
                  <Td><Badge tone={statusTone(st)}>{st}</Badge></Td>
                  <Td title={absoluteTime(l.expiresAt)}>{relativeTime(l.expiresAt)}</Td>
                  <Td>{l.viewCount}</Td>
                  <Td>{st === "active" && <Button size="sm" variant="ghost" onClick={() => setRevoke(l)}>Revoke</Button>}</Td>
                </tr>
              );
            })}
          </Table>
          {cursor && <div style={{ marginTop: 12 }}><Button variant="secondary" onClick={() => load(false)}>Load more</Button></div>}
        </>
      )}

      {showCreate && <CreateLink onClose={() => setShowCreate(false)} onCreated={() => { load(true); toast({ tone: "success", msg: "Link created" }); }} />}
      {revoke && (
        <ConfirmDialog title="Revoke link" danger message={`Revoke "${revoke.displayName || revoke.linkId}"? Visitors will get a 410 within seconds.`}
          onCancel={() => setRevoke(null)}
          onConfirm={async () => {
            await api(`/preview-links/${revoke.linkId}/revoke`, { method: "POST" });
            setRevoke(null); load(true); toast({ tone: "success", msg: "Revoked (audit logged)" });
          }} />
      )}
    </div>
  );
}

function policyLabel(p: any): React.ReactNode {
  if (!p) return "public";
  const parts = [p.type];
  if (p.singleUse) parts.push("single-use");
  else if (p.maxViews) parts.push(`max ${p.maxViews}`);
  if (p.ipAllowlist?.length) parts.push("ip");
  return <span style={{ fontSize: 12 }}>{parts.join(" · ")}</span>;
}

function CreateLink({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const toast = useToast();
  const [instances, setInstances] = useState<any[]>([]);
  const [files, setFiles] = useState<any[]>([]);
  const [created, setCreated] = useState<any>(null);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);
  const [f, setF] = useState<any>({
    mode: "tunnel", instanceId: "", fileRef: "", displayName: "", fileId: "",
    expiresInSeconds: 3600, type: "public_token", allowDownload: true, maxViews: 0, singleUse: false, password: "", ipAllowlist: "",
  });

  useEffect(() => {
    api<List<any>>("/instances?limit=50").then((r) => { setInstances(r.items || []); if (r.items?.[0]) setF((p: any) => ({ ...p, instanceId: r.items[0].instanceId })); });
    api<List<any>>("/files?status=ready&limit=50").then((r) => setFiles(r.items || []));
  }, []);

  async function submit() {
    setErr(""); setLoading(true);
    const body: any = {
      mode: f.mode, expiresInSeconds: Number(f.expiresInSeconds),
      displayName: f.displayName,
      accessPolicy: {
        type: f.type, allowDownload: f.allowDownload,
        maxViews: Number(f.maxViews) || 0, singleUse: f.singleUse,
        ipAllowlist: f.ipAllowlist ? f.ipAllowlist.split(",").map((s: string) => s.trim()).filter(Boolean) : [],
      },
    };
    if (f.password) body.password = f.password;
    if (f.mode === "tunnel") { body.instanceId = f.instanceId; body.fileRef = f.fileRef; }
    else { body.fileId = f.fileId; }
    try {
      const c = await api("/preview-links", { method: "POST", body, idempotencyKey: idemKey("link") });
      setCreated(c); onCreated();
    } catch (e) { setErr(e instanceof ApiException ? e.body.message : "Failed"); }
    finally { setLoading(false); }
  }

  return (
    <Drawer title={created ? "Link created" : "Create preview link"} onClose={onClose}>
      {created ? (
        <div>
          <p>Share this URL. It contains the secret and is shown only once.</p>
          <SecretReveal value={created.url} />
          <p style={{ fontSize: 13, color: "var(--color-text-muted)", marginTop: 12 }}>Expires: {absoluteTime(created.expiresAt)}</p>
          <Button onClick={onClose} style={{ marginTop: 12 }}>Done</Button>
        </div>
      ) : (
        <div>
          <Select label="Mode" value={f.mode} onChange={(e) => setF({ ...f, mode: e.target.value })}>
            <option value="tunnel">Tunnel (file on your server)</option>
            <option value="cloud">Cloud (uploaded file)</option>
          </Select>
          {f.mode === "tunnel" ? (
            <>
              <Select label="Instance" value={f.instanceId} onChange={(e) => setF({ ...f, instanceId: e.target.value })}>
                {instances.map((i) => <option key={i.instanceId} value={i.instanceId}>{i.agentName} ({i.subdomain})</option>)}
              </Select>
              <Input label="File ref (from local register)" value={f.fileRef} onChange={(e) => setF({ ...f, fileRef: e.target.value })} placeholder="fref_..." />
              <Input label="Display name" value={f.displayName} onChange={(e) => setF({ ...f, displayName: e.target.value })} />
            </>
          ) : (
            <Select label="Ready file" value={f.fileId} onChange={(e) => setF({ ...f, fileId: e.target.value })}>
              <option value="">Select a ready file…</option>
              {files.map((x) => <option key={x.fileId} value={x.fileId}>{x.displayName}</option>)}
            </Select>
          )}
          <Input label="Expires in (seconds)" type="number" value={f.expiresInSeconds} onChange={(e) => setF({ ...f, expiresInSeconds: e.target.value })} />
          <Select label="Access policy" value={f.type} onChange={(e) => setF({ ...f, type: e.target.value })}>
            <option value="public_token">Public token</option>
            <option value="password">Password</option>
            <option value="account">Account required</option>
            <option value="ip_allowlist">IP allowlist</option>
          </Select>
          {f.type === "password" && <Input label="Password (stored hashed; attempts are rate-limited)" type="password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} />}
          {f.type === "ip_allowlist" && <Input label="CIDR allowlist (comma-separated)" value={f.ipAllowlist} onChange={(e) => setF({ ...f, ipAllowlist: e.target.value })} placeholder="203.0.113.0/24" />}
          <label style={{ display: "flex", gap: 8, alignItems: "center", marginBottom: 8 }}>
            <input type="checkbox" checked={f.allowDownload} onChange={(e) => setF({ ...f, allowDownload: e.target.checked })} /> Allow download (best-effort)
          </label>
          <label style={{ display: "flex", gap: 8, alignItems: "center", marginBottom: 8 }}>
            <input type="checkbox" checked={f.singleUse} onChange={(e) => setF({ ...f, singleUse: e.target.checked })} /> Single use
          </label>
          {!f.singleUse && <Input label="Max views (0 = unlimited)" type="number" value={f.maxViews} onChange={(e) => setF({ ...f, maxViews: e.target.value })} />}
          {err && <div style={{ color: "var(--color-danger)", fontSize: 13, marginBottom: 12 }}>{err}</div>}
          <Button loading={loading} onClick={submit}>Create link</Button>
        </div>
      )}
    </Drawer>
  );
}
