"use client";
import { useEffect, useState } from "react";
import { adminApi, ApiException } from "@/lib/api";
import { Card, Banner, Badge, Stat, Button, Input, Table } from "@/components/ui";

type Overview = { tenants: number; onlineInstances: number; activeLinks: number; queues: Record<string, number> };
type Tenant = { tenantId: string; name: string; plan: string; trustLevel: string; status: string };

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
  useEffect(() => { loadOverview(); }, []);

  if (authed === null) return <main style={{ padding: 24 }}>Loading…</main>;
  if (!authed) return <AdminLogin onDone={loadOverview} />;

  return (
    <main style={{ maxWidth: 1100, margin: "0 auto", padding: 24 }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h1>APAGE Admin</h1>
        <div style={{ display: "flex", gap: 12, alignItems: "center" }}>
          <Badge tone="danger">internal · MFA</Badge>
          <Button variant="ghost" onClick={async () => { await adminApi("/auth/logout", { method: "POST" }); setAuthed(false); }}>Sign out</Button>
        </div>
      </div>
      <Banner tone="warning">Internal operations console. IP-allowlisted; all actions are audited and metadata-only.</Banner>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(180px,1fr))", gap: 16, margin: "16px 0 24px" }}>
        <Stat label="Tenants" value={String(overview?.tenants ?? "—")} />
        <Stat label="Online agents" value={String(overview?.onlineInstances ?? "—")} />
        <Stat label="Active links" value={String(overview?.activeLinks ?? "—")} />
        <Stat label="Scan queue" value={String(overview?.queues?.scan ?? "—")} />
      </div>

      <Tenants />
    </main>
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
    e.preventDefault(); setErr("");
    try {
      const r = await adminApi<{ enrolled: boolean; otpauthUri?: string }>("/auth/login", { method: "POST", body: { email, password } });
      if (r.otpauthUri) setOtpauth(r.otpauthUri);
      setStep("mfa");
    } catch (e) { setErr(e instanceof ApiException ? e.body.message : "Login failed"); }
  }
  async function verify(e: React.FormEvent) {
    e.preventDefault(); setErr("");
    try { await adminApi("/auth/mfa", { method: "POST", body: { code } }); onDone(); }
    catch (e) { setErr(e instanceof ApiException ? e.body.message : "Invalid code"); }
  }

  return (
    <main style={{ maxWidth: 380, margin: "12vh auto", padding: 24 }}>
      <h1 style={{ textAlign: "center" }}>APAGE Admin</h1>
      <Badge tone="danger">internal · MFA required</Badge>
      {step === "login" ? (
        <form onSubmit={login} style={{ marginTop: 16 }}>
          <Input label="Email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
          <Input label="Password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} required />
          {err && <div style={{ color: "var(--color-danger)", fontSize: 13, marginBottom: 12 }}>{err}</div>}
          <Button type="submit" style={{ width: "100%" }}>Continue</Button>
        </form>
      ) : (
        <form onSubmit={verify} style={{ marginTop: 16 }}>
          {otpauth && (
            <div style={{ fontSize: 13, color: "var(--color-text-muted)", marginBottom: 12, wordBreak: "break-all" }}>
              First-time setup: add this to your authenticator app, then enter the code.
              <code style={{ display: "block", marginTop: 8 }}>{otpauth}</code>
            </div>
          )}
          <Input label="6-digit code" inputMode="numeric" value={code} onChange={(e) => setCode(e.target.value)} required />
          {err && <div style={{ color: "var(--color-danger)", fontSize: 13, marginBottom: 12 }}>{err}</div>}
          <Button type="submit" style={{ width: "100%" }}>Verify</Button>
        </form>
      )}
    </main>
  );
}

function Tenants() {
  const [items, setItems] = useState<Tenant[]>([]);
  const [q, setQ] = useState("");
  async function load() {
    try { const r = await adminApi<{ items: Tenant[] }>(`/tenants${q ? `?q=${encodeURIComponent(q)}` : ""}`); setItems(r.items || []); }
    catch { setItems([]); }
  }
  useEffect(() => { load(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

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
    <Card title="Tenants">
      <div style={{ display: "flex", gap: 8, marginBottom: 12 }}>
        <Input placeholder="search name / id" value={q} onChange={(e) => setQ(e.target.value)} />
        <Button variant="secondary" onClick={load}>Search</Button>
      </div>
      <Table head={["Name", "Plan", "Trust", "Status", "Actions"]}>
        {items.map((t) => (
          <tr key={t.tenantId}>
            <td>{t.name}</td>
            <td>{t.plan}</td>
            <td>
              <select value={t.trustLevel} onChange={(e) => setTrust(t, e.target.value)}>
                <option value="new">new</option>
                <option value="basic">basic</option>
                <option value="trusted">trusted</option>
              </select>
            </td>
            <td><Badge tone={t.status === "suspended" ? "danger" : "success"}>{t.status}</Badge></td>
            <td>
              {t.status === "suspended"
                ? <Button variant="secondary" onClick={() => act(t, "restore")}>Restore</Button>
                : <Button variant="danger" onClick={() => act(t, "suspend")}>Suspend</Button>}
            </td>
          </tr>
        ))}
      </Table>
    </Card>
  );
}
