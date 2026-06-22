"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { api, ApiException, setTenant } from "@/lib/api";
import { Button, Input } from "@/components/ui";
import { AuthShell } from "@/components/authshell";

export default function Register() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [tenantName, setTenantName] = useState("");
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(""); setLoading(true);
    try {
      const res = await api<{ tenantId: string }>("/auth/register", {
        method: "POST", tenant: false, body: { email, password, tenantName },
      });
      setTenant(res.tenantId);
      router.push("/console");
    } catch (e) {
      setErr(e instanceof ApiException ? e.body.message : "Registration failed");
    } finally { setLoading(false); }
  }

  return (
    <AuthShell title="Create your APAGE account">
      <form onSubmit={submit}>
        <Input label="Work email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
        <Input label="Organization name" value={tenantName} onChange={(e) => setTenantName(e.target.value)} placeholder="Acme Inc" />
        <Input label="Password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} required placeholder="≥10 chars, letters + digits" />
        {err && <div style={{ color: "var(--color-danger)", fontSize: 13, marginBottom: 12 }}>{err}</div>}
        <Button type="submit" loading={loading} style={{ width: "100%" }}>Create account</Button>
      </form>
      <p style={{ fontSize: 12, color: "var(--color-text-subtle)", marginTop: 12 }}>
        You become the owner of a new tenant on the free Lite plan. A verification email is sent.
      </p>
      <p style={{ fontSize: 13, marginTop: 8, color: "var(--color-text-muted)" }}>
        Already have an account? <Link href="/login">Sign in</Link>
      </p>
    </AuthShell>
  );
}
