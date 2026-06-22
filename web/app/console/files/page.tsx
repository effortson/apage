"use client";
import { useEffect, useRef, useState } from "react";
import { api, List, getTenant } from "@/lib/api";
import { usePoll } from "@/lib/hooks";
import { relativeTime, formatBytes } from "@/lib/format";
import { PageHeader, EmptyState, StatusBadge, ConfirmDialog } from "@/components/composites";
import { Button } from "@/components/ui/button";
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

export default function Files() {
  const [items, setItems] = useState<any[]>([]);
  const [instances, setInstances] = useState<any[]>([]);
  const [instanceId, setInstanceId] = useState("");
  const [loading, setLoading] = useState(true);
  const [del, setDel] = useState<any>(null);
  const [progress, setProgress] = useState<number | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);

  const load = () => api<List<any>>("/files?limit=50").then((r) => { setItems(r.items || []); setLoading(false); });
  useEffect(() => {
    load().catch(() => setLoading(false));
    api<List<any>>("/instances?limit=50").then((r) => { setInstances(r.items || []); if (r.items?.[0]) setInstanceId(r.items[0].instanceId); });
  }, []);
  usePoll(() => { load().catch(() => {}); }, 5000); // scanning → ready reflects without manual refresh

  function upload(file: File) {
    if (!instanceId) { toast.error("Create an instance first"); return; }
    const fd = new FormData();
    fd.append("file", file);
    fd.append("displayName", file.name);
    fd.append("instanceId", instanceId);
    // XHR (not fetch) so we can show real upload progress (UI §7.4).
    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/api/v1/files");
    xhr.withCredentials = true;
    xhr.setRequestHeader("X-Tenant-Id", getTenant() || "");
    xhr.setRequestHeader("Idempotency-Key", `up-${Date.now()}`);
    xhr.upload.onprogress = (e) => { if (e.lengthComputable) setProgress(Math.round((e.loaded / e.total) * 100)); };
    xhr.onload = () => {
      setProgress(null);
      if (xhr.status >= 200 && xhr.status < 300) { toast.success("Uploaded — scanning…"); load().catch(() => {}); }
      else { toast.error("Upload failed (size/type limit?)"); }
    };
    xhr.onerror = () => { setProgress(null); toast.error("Upload failed"); };
    setProgress(0);
    xhr.send(fd);
  }

  return (
    <div>
      <PageHeader
        title="Cloud Files"
        description="Direct upload ≤ 8 MiB; larger files use presigned upload. Deleting a file cascades to all its links."
        actions={
          <div className="flex items-center gap-2">
            <Select value={instanceId} onValueChange={setInstanceId}>
              <SelectTrigger className="h-9 w-[180px]">
                <SelectValue placeholder="Instance" />
              </SelectTrigger>
              <SelectContent>
                {instances.map((i) => (
                  <SelectItem key={i.instanceId} value={i.instanceId}>
                    {i.agentName}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <input ref={fileInput} type="file" hidden onChange={(e) => e.target.files?.[0] && upload(e.target.files[0])} />
            <Button onClick={() => fileInput.current?.click()}>Upload file</Button>
          </div>
        }
      />

      {progress !== null && (
        <div className="mb-4">
          <div className="mb-1 text-xs text-muted-foreground">Uploading… {progress}%</div>
          <div className="h-2 overflow-hidden rounded-full bg-muted">
            <div
              className="h-full bg-primary transition-[width] duration-200"
              style={{ width: `${progress}%` }}
            />
          </div>
        </div>
      )}

      {loading ? (
        <Skeleton className="h-64 w-full" />
      ) : items.length === 0 ? (
        <EmptyState
          title="No cloud files"
          hint="Upload a file to create preview links."
          action={<Button onClick={() => fileInput.current?.click()}>Upload file</Button>}
        />
      ) : (
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>File ID</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="text-right">Size</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Expires</TableHead>
                <TableHead className="w-20" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((f) => (
                <TableRow key={f.fileId}>
                  <TableCell className="font-medium">{f.displayName}</TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">{f.fileId}</TableCell>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <StatusBadge status={f.status} />
                      {f.rejectReason && <span className="text-xs text-destructive">{f.rejectReason}</span>}
                    </div>
                  </TableCell>
                  <TableCell className="text-right tabular-nums">{formatBytes(f.size)}</TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">{f.mimeType}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{relativeTime(f.expiresAt)}</TableCell>
                  <TableCell className="text-right">
                    <Button variant="ghost" size="sm" onClick={() => setDel(f)}>
                      Delete
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <ConfirmDialog
        open={!!del}
        onOpenChange={(o) => !o && setDel(null)}
        title="Delete file"
        destructive
        confirmLabel="Delete"
        confirmWord={del?.displayName}
        description="Deleting removes the original and all derivatives, and invalidates every link backed by it."
        onConfirm={async () => {
          const f = del;
          setDel(null);
          await api(`/files/${f.fileId}`, { method: "DELETE" });
          load();
          toast.success("Deleted", { description: "Audit logged." });
        }}
      />
    </div>
  );
}
