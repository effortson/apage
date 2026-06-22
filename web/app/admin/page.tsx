"use client";
import { useEffect, useState } from "react";
import { adminApi, ApiException } from "@/lib/api";
import { relativeTime } from "@/lib/format";
import { PageHeader, Stat, EmptyState, StatusBadge } from "@/components/composites";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

type Overview = {
  tenants: number;
  instances: number;
  activeLinks: number;
  queues: Record<string, number>;
};
type Tenant = {
  tenantId: string;
  name: string;
  plan: string;
  trustLevel: string;
  status: string;
};

export default function Admin() {
  const [authed, setAuthed] = useState<boolean | null>(null);
  const [overview, setOverview] = useState<Overview | null>(null);

  async function loadOverview() {
    try {
      setOverview(await adminApi<Overview>("/overview"));
      setAuthed(true);
    } catch (e) {
      if (e instanceof ApiException && e.status === 401) setAuthed(false);
    }
  }
  useEffect(() => {
    loadOverview();
  }, []);

  if (authed === null) {
    return (
      <main className="mx-auto max-w-5xl p-6">
        <Skeleton className="h-8 w-48" />
        <div className="mt-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-28 w-full" />
          ))}
        </div>
      </main>
    );
  }
  if (!authed) return <AdminLogin onDone={loadOverview} />;

  return (
    <main className="mx-auto max-w-5xl p-6">
      <PageHeader
        title="APAGE Admin"
        actions={
          <>
            <Badge variant="destructive">internal · MFA</Badge>
            <Button
              variant="ghost"
              onClick={async () => {
                await adminApi("/auth/logout", { method: "POST" });
                setAuthed(false);
              }}
            >
              Sign out
            </Button>
          </>
        }
      />

      <Alert className="mb-6">
        <AlertTitle>Internal operations console</AlertTitle>
        <AlertDescription>
          IP-allowlisted; all actions are audited and metadata-only.
        </AlertDescription>
      </Alert>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Stat label="Tenants" value={String(overview?.tenants ?? "—")} />
        <Stat label="Instances" value={String(overview?.instances ?? "—")} />
        <Stat label="Active links" value={String(overview?.activeLinks ?? "—")} />
        <Stat label="Scan queue" value={String(overview?.queues?.scan ?? "—")} />
      </div>

      <Tabs defaultValue="tenants" className="mt-6">
        <TabsList>
          <TabsTrigger value="tenants">Tenants</TabsTrigger>
          <TabsTrigger value="abuse">Abuse queue</TabsTrigger>
          <TabsTrigger value="audit">Global audit</TabsTrigger>
        </TabsList>
        <TabsContent value="tenants">
          <Tenants />
        </TabsContent>
        <TabsContent value="abuse">
          <Abuse />
        </TabsContent>
        <TabsContent value="audit">
          <AuditLog />
        </TabsContent>
      </Tabs>
    </main>
  );
}

function Abuse() {
  const [items, setItems] = useState<any[]>([]);
  const [status, setStatus] = useState("open");
  async function load() {
    try {
      const r = await adminApi<{ items: any[] }>(`/abuse-reports?status=${status}`);
      setItems(r.items || []);
    } catch {
      setItems([]);
    }
  }
  useEffect(() => {
    load();
  }, [status]); // eslint-disable-line react-hooks/exhaustive-deps
  async function act(id: string, s: "actioned" | "dismissed") {
    await adminApi(`/abuse-reports/${id}/action`, { method: "POST", body: { status: s } });
    load();
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>Abuse reports</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="mb-4">
          <Select value={status || "all"} onValueChange={(v) => setStatus(v === "all" ? "" : v)}>
            <SelectTrigger className="h-9 w-[160px]">
              <SelectValue placeholder="Status" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="open">open</SelectItem>
              <SelectItem value="actioned">actioned</SelectItem>
              <SelectItem value="dismissed">dismissed</SelectItem>
              <SelectItem value="all">all</SelectItem>
            </SelectContent>
          </Select>
        </div>
        {items.length === 0 ? (
          <EmptyState title="No abuse reports" hint="Reports matching this filter will appear here." />
        ) : (
          <div className="rounded-lg border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Category</TableHead>
                  <TableHead>Link</TableHead>
                  <TableHead>Detail</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>When</TableHead>
                  <TableHead className="w-40" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((a) => (
                  <TableRow key={a.reportId}>
                    <TableCell>
                      <Badge variant="warning">{a.category}</Badge>
                    </TableCell>
                    <TableCell className="font-mono text-xs">{a.linkId || "—"}</TableCell>
                    <TableCell>{(a.detail || "").slice(0, 80)}</TableCell>
                    <TableCell>{a.status}</TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {relativeTime(a.createdAt)}
                    </TableCell>
                    <TableCell className="text-right">
                      {a.status === "open" && (
                        <span className="flex justify-end gap-2">
                          <Button
                            size="sm"
                            variant="destructive"
                            onClick={() => act(a.reportId, "actioned")}
                          >
                            Action
                          </Button>
                          <Button
                            size="sm"
                            variant="secondary"
                            onClick={() => act(a.reportId, "dismissed")}
                          >
                            Dismiss
                          </Button>
                        </span>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function AuditLog() {
  const [items, setItems] = useState<any[]>([]);
  const [event, setEvent] = useState("");
  async function load() {
    try {
      const r = await adminApi<{ items: any[] }>(
        `/audit-logs?limit=50${event ? `&event=${encodeURIComponent(event)}` : ""}`,
      );
      setItems(r.items || []);
    } catch {
      setItems([]);
    }
  }
  useEffect(() => {
    load();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <Card>
      <CardHeader>
        <CardTitle>Global audit (cross-tenant)</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="mb-4 flex gap-2">
          <Input
            placeholder="event filter, e.g. tenant.suspended"
            value={event}
            onChange={(e) => setEvent(e.target.value)}
          />
          <Button variant="secondary" onClick={load}>
            Filter
          </Button>
        </div>
        {items.length === 0 ? (
          <EmptyState title="No audit events" hint="Cross-tenant events will appear here." />
        ) : (
          <div className="rounded-lg border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Event</TableHead>
                  <TableHead>Tenant</TableHead>
                  <TableHead>Actor</TableHead>
                  <TableHead>Resource</TableHead>
                  <TableHead>When</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((a) => (
                  <TableRow key={a.eventId}>
                    <TableCell className="font-mono text-xs">{a.event}</TableCell>
                    <TableCell className="font-mono text-xs">{a.tenantId || "—"}</TableCell>
                    <TableCell>{a.actorType}</TableCell>
                    <TableCell className="font-mono text-xs">{a.resourceId || "—"}</TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {relativeTime(a.createdAt)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function AdminLogin({ onDone }: { onDone: () => void }) {
  const [step, setStep] = useState<"login" | "mfa">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [otpauth, setOtpauth] = useState<string | null>(null);
  const [err, setErr] = useState("");

  async function login(e: React.FormEvent) {
    e.preventDefault();
    setErr("");
    try {
      const r = await adminApi<{ enrolled: boolean; otpauthUri?: string }>("/auth/login", {
        method: "POST",
        body: { email, password },
      });
      if (r.otpauthUri) setOtpauth(r.otpauthUri);
      setStep("mfa");
    } catch (e) {
      setErr(e instanceof ApiException ? e.body.message : "Login failed");
    }
  }
  async function verify(e: React.FormEvent) {
    e.preventDefault();
    setErr("");
    try {
      await adminApi("/auth/mfa", { method: "POST", body: { code } });
      onDone();
    } catch (e) {
      setErr(e instanceof ApiException ? e.body.message : "Invalid code");
    }
  }

  return (
    <main className="mx-auto max-w-sm px-6 py-[12vh]">
      <Card>
        <CardHeader className="text-center">
          <CardTitle>APAGE Admin</CardTitle>
          <div className="flex justify-center pt-1">
            <Badge variant="destructive">internal · MFA required</Badge>
          </div>
        </CardHeader>
        <CardContent>
          {step === "login" ? (
            <form onSubmit={login} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="email">Email</Label>
                <Input
                  id="email"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="password">Password</Label>
                <Input
                  id="password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                />
              </div>
              {err && (
                <Alert variant="destructive">
                  <AlertDescription>{err}</AlertDescription>
                </Alert>
              )}
              <Button type="submit" className="w-full">
                Continue
              </Button>
            </form>
          ) : (
            <form onSubmit={verify} className="space-y-4">
              {otpauth && (
                <div className="space-y-2 text-sm text-muted-foreground">
                  First-time setup: add this to your authenticator app, then enter the code.
                  <code className="block break-all rounded-md border bg-muted px-3 py-2 font-mono text-xs">
                    {otpauth}
                  </code>
                </div>
              )}
              <div className="space-y-2">
                <Label htmlFor="code">6-digit code</Label>
                <Input
                  id="code"
                  inputMode="numeric"
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                  required
                />
              </div>
              {err && (
                <Alert variant="destructive">
                  <AlertDescription>{err}</AlertDescription>
                </Alert>
              )}
              <Button type="submit" className="w-full">
                Verify
              </Button>
            </form>
          )}
        </CardContent>
      </Card>
    </main>
  );
}

function Tenants() {
  const [items, setItems] = useState<Tenant[]>([]);
  const [q, setQ] = useState("");
  async function load() {
    try {
      const r = await adminApi<{ items: Tenant[] }>(`/tenants${q ? `?q=${encodeURIComponent(q)}` : ""}`);
      setItems(r.items || []);
    } catch {
      setItems([]);
    }
  }
  useEffect(() => {
    load();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  async function act(t: Tenant, action: "suspend" | "restore") {
    if (!confirm(`${action} tenant ${t.name}?`)) return;
    await adminApi(`/tenants/${t.tenantId}/${action}`, { method: "POST" });
    load();
  }
  async function setTrust(t: Tenant, trust: string) {
    await adminApi(`/tenants/${t.tenantId}/trust`, { method: "POST", body: { trust } });
    load();
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Tenants</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="mb-4 flex gap-2">
          <Input placeholder="search name / id" value={q} onChange={(e) => setQ(e.target.value)} />
          <Button variant="secondary" onClick={load}>
            Search
          </Button>
        </div>
        {items.length === 0 ? (
          <EmptyState title="No tenants" hint="No tenants match this search." />
        ) : (
          <div className="rounded-lg border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Plan</TableHead>
                  <TableHead>Trust</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="w-28 text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((t) => (
                  <TableRow key={t.tenantId}>
                    <TableCell className="font-medium">{t.name}</TableCell>
                    <TableCell className="capitalize text-muted-foreground">{t.plan}</TableCell>
                    <TableCell>
                      <Select value={t.trustLevel} onValueChange={(v) => setTrust(t, v)}>
                        <SelectTrigger className="h-8 w-[120px]">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="new">new</SelectItem>
                          <SelectItem value="basic">basic</SelectItem>
                          <SelectItem value="trusted">trusted</SelectItem>
                        </SelectContent>
                      </Select>
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={t.status} />
                    </TableCell>
                    <TableCell className="text-right">
                      {t.status === "suspended" ? (
                        <Button variant="secondary" size="sm" onClick={() => act(t, "restore")}>
                          Restore
                        </Button>
                      ) : (
                        <Button variant="destructive" size="sm" onClick={() => act(t, "suspend")}>
                          Suspend
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
