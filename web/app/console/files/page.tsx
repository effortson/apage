"use client";
import { useEffect, useRef, useState } from "react";
import { api, List, getTenant } from "@/lib/api";
import { Button, Table, Td, Badge, EmptyState, Skeleton, useToast, ConfirmDialog, statusTone, Select } from "@/components/ui";
import { relativeTime, formatBytes } from "@/lib/format";

export default function Files() {
  const toast = useToast();
  const [items, setItems] = useState<any[]>([]);
  const [instances, setInstances] = useState<any[]>([]);
  const [instanceId, setInstanceId] = useState("");
  const [loading, setLoading] = useState(true);
  const [del, setDel] = useState<any>(null);
  const fileInput = useRef<HTMLInputElement>(null);

  const load = () => api<List<any>>("/files?limit=50").then((r) => { setItems(r.items || []); setLoading(false); });
  useEffect(() => {
    load().catch(() => setLoading(false));
    api<List<any>>("/instances?limit=50").then((r) => { setInstances(r.items || []); if (r.items?.[0]) setInstanceId(r.items[0].instanceId); });
  }, []);

  async function upload(file: File) {
    if (!instanceId) { toast({ tone: "danger", msg: "Create an instance first" }); return; }
    const fd = new FormData();
    fd.append("file", file);
    fd.append("displayName", file.name);
    fd.append("instanceId", instanceId);
    const res = await fetch("/api/v1/files", { method: "POST", credentials: "include", headers: { "X-Tenant-Id": getTenant() || "", "Idempotency-Key": `up-${Date.now()}` }, body: fd });
    if (!res.ok) { toast({ tone: "danger", msg: "Upload failed (size/type limit?)" }); return; }
    toast({ tone: "success", msg: "Uploaded — scanning…" });
    setTimeout(load, 1500);
  }

  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 16 }}>
        <h1>Cloud Files</h1>
        <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
          <Select value={instanceId} onChange={(e) => setInstanceId(e.target.value)}>
            {instances.map((i) => <option key={i.instanceId} value={i.instanceId}>{i.agentName}</option>)}
          </Select>
          <input ref={fileInput} type="file" hidden onChange={(e) => e.target.files?.[0] && upload(e.target.files[0])} />
          <Button onClick={() => fileInput.current?.click()}>Upload file</Button>
        </div>
      </div>
      <p style={{ fontSize: 13, color: "var(--color-text-muted)", marginBottom: 16 }}>
        Direct upload ≤ 8 MiB; larger files use presigned upload. Deleting a file cascades to all its links.
      </p>

      {loading ? <Skeleton rows={4} /> : items.length === 0 ? (
        <EmptyState title="No cloud files" hint="Upload a file, or use Tunnel mode to keep files on your server." action={<Button onClick={() => fileInput.current?.click()}>Upload file</Button>} />
      ) : (
        <Table head={["Name", "File ID", "Status", "Size", "Type", "Expires", ""]}>
          {items.map((f) => (
            <tr key={f.fileId}>
              <Td>{f.displayName}</Td>
              <Td mono>{f.fileId}</Td>
              <Td><Badge tone={statusTone(f.status)}>{f.status}</Badge>{f.rejectReason && <span style={{ fontSize: 11, color: "var(--color-danger)" }}> {f.rejectReason}</span>}</Td>
              <Td>{formatBytes(f.size)}</Td>
              <Td>{f.mimeType}</Td>
              <Td>{relativeTime(f.expiresAt)}</Td>
              <Td><Button size="sm" variant="ghost" onClick={() => setDel(f)}>Delete</Button></Td>
            </tr>
          ))}
        </Table>
      )}

      {del && (
        <ConfirmDialog title="Delete file" danger confirmWord={del.displayName}
          message="Deleting removes the original and all derivatives, and invalidates every link backed by it."
          onCancel={() => setDel(null)}
          onConfirm={async () => { await api(`/files/${del.fileId}`, { method: "DELETE" }); setDel(null); load(); toast({ tone: "success", msg: "Deleted (audit logged)" }); }} />
      )}
    </div>
  );
}
