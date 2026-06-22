"use client";
import { useEffect, useState } from "react";
import { api, List } from "@/lib/api";
import { Table, Td, Skeleton, Input, Button, Badge } from "@/components/ui";
import { absoluteTime, relativeTime } from "@/lib/format";

export default function Audit() {
  const [items, setItems] = useState<any[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [event, setEvent] = useState("");
  const [loading, setLoading] = useState(true);

  const load = (reset = true) => {
    const q = new URLSearchParams({ limit: "50" });
    if (event) q.set("event", event);
    if (!reset && cursor) q.set("cursor", cursor);
    return api<List<any>>(`/audit-logs?${q}`).then((r) => { setItems(reset ? r.items || [] : [...items, ...(r.items || [])]); setCursor(r.nextCursor); setLoading(false); });
  };
  useEffect(() => { setLoading(true); load(true).catch(() => setLoading(false)); /* eslint-disable-next-line */ }, [event]);

  return (
    <div>
      <h1 style={{ marginBottom: 16 }}>Audit Logs</h1>
      <div style={{ maxWidth: 320 }}>
        <Input label="Filter by event" value={event} onChange={(e) => setEvent(e.target.value)} placeholder="preview_link.accessed" />
      </div>
      <p style={{ fontSize: 13, color: "var(--color-text-muted)", marginBottom: 12 }}>Secrets are always redacted. Visible to admins and owners.</p>
      {loading ? <Skeleton rows={6} /> : (
        <>
          <Table head={["Time", "Event", "Actor", "Resource", "IP", "Reason"]}>
            {items.map((a) => (
              <tr key={a.eventId}>
                <Td title={absoluteTime(a.createdAt)}>{relativeTime(a.createdAt)}</Td>
                <Td><Badge tone="muted">{a.event}</Badge></Td>
                <Td>{a.actorType}{a.actorId ? ` (${a.actorId.slice(0, 10)}…)` : ""}</Td>
                <Td mono>{a.resourceType}{a.resourceId ? `/${a.resourceId.slice(0, 12)}…` : ""}</Td>
                <Td mono>{a.ip || "—"}</Td>
                <Td>{a.reason || "—"}</Td>
              </tr>
            ))}
          </Table>
          {cursor && <div style={{ marginTop: 12 }}><Button variant="secondary" onClick={() => load(false)}>Load more</Button></div>}
        </>
      )}
    </div>
  );
}
