"use client";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { Button, Table, Td, Badge, Drawer, Input, Skeleton, useToast, CopyField, statusTone } from "@/components/ui";
import { relativeTime } from "@/lib/format";

export default function Domains() {
  const toast = useToast();
  const [items, setItems] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [add, setAdd] = useState(false);

  const load = () => api<{ items: any[] }>("/custom-domains").then((r) => { setItems(r.items || []); setLoading(false); });
  useEffect(() => { load().catch(() => setLoading(false)); }, []);

  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 16 }}>
        <h1>Custom Domains</h1>
        <Button onClick={() => setAdd(true)}>Add domain</Button>
      </div>
      {loading ? <Skeleton rows={3} /> : items.length === 0 ? (
        <p style={{ color: "var(--color-text-muted)" }}>No custom domains. Available on paid plans; subject to your plan&apos;s domain limit.</p>
      ) : (
        <Table head={["Domain", "Status", "Certificate", "Last checked", ""]}>
          {items.map((d) => (
            <tr key={d.domainId}>
              <Td mono>{d.domain}</Td>
              <Td><Badge tone={statusTone(d.status)}>{d.status}</Badge></Td>
              <Td>{d.certStatus}</Td>
              <Td>{relativeTime(d.lastCheckedAt)}</Td>
              <Td>
                <Button size="sm" variant="ghost" onClick={async () => { const r = await api<any>(`/custom-domains/${d.domainId}/verify`, { method: "POST" }); toast({ tone: r.status === "verified" ? "success" : "danger", msg: `Status: ${r.status}` }); load(); }}>Check DNS</Button>
              </Td>
            </tr>
          ))}
        </Table>
      )}
      {add && <AddDomain onClose={() => setAdd(false)} onDone={() => { setAdd(false); load(); }} />}
    </div>
  );
}

function AddDomain({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const toast = useToast();
  const [domain, setDomain] = useState(""); const [res, setRes] = useState<any>(null);
  return (
    <Drawer title={res ? "DNS records" : "Add custom domain"} onClose={onClose}>
      {res ? (
        <div>
          <p>Add these records at your DNS provider, then click Check DNS.</p>
          <h3 style={{ marginTop: 12 }}>TXT (ownership)</h3>
          <CopyField value={`${res.dns.txt.name}  TXT  ${res.dns.txt.value}`} />
          <h3 style={{ marginTop: 12 }}>CNAME</h3>
          <CopyField value={`${res.dns.cname.name}  CNAME  ${res.dns.cname.value}`} />
          <Button style={{ marginTop: 16 }} onClick={onDone}>Done</Button>
        </div>
      ) : (
        <div>
          <Input label="Domain" value={domain} onChange={(e) => setDomain(e.target.value)} placeholder="preview.customer.com" />
          <Button onClick={async () => {
            try { const r = await api<any>("/custom-domains", { method: "POST", body: { domain } }); setRes(r); }
            catch (e: any) { toast({ tone: "danger", msg: e.body?.message || "Failed" }); }
          }}>Add domain</Button>
        </div>
      )}
    </Drawer>
  );
}
