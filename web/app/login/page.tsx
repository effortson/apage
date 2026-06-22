"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { api, ApiException } from "@/lib/api";
import { Button, Input } from "@/components/ui";
import { AuthShell } from "@/components/authshell";

export default function Login() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(""); setLoading(true);
    try {
      await api("/auth/login", { method: "POST", tenant: false, body: { email, password } });
      router.push("/console");
    } catch (e) {
      setErr(e instanceof ApiException ? e.body.message : "Login failed");
    } finally { setLoading(false); }
  }

  return (
    <AuthShell title="Sign in to APAGE">
      <form onSubmit={submit}>
        <Input label="Email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
        <Input label="Password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} required />
        {err && <div style={{ color: "var(--color-danger)", fontSize: 13, marginBottom: 12 }}>{err}</div>}
        <Button type="submit" loading={loading} style={{ width: "100%" }}>Sign in</Button>
      </form>
      <p style={{ fontSize: 13, marginTop: 16, color: "var(--color-text-muted)" }}>
        No account? <Link href="/register">Create one</Link>
      </p>
    </AuthShell>
  );
}
