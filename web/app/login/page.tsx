"use client";
import { useEffect, useState } from "react";
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
  const [providers, setProviders] = useState<string[]>([]);

  useEffect(() => {
    api<{ providers: string[] }>("/auth/providers", { tenant: false })
      .then((r) => setProviders(r.providers || []))
      .catch(() => {});
  }, []);

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
      {providers.length > 0 && (
        <div style={{ marginTop: 16 }}>
          <div style={{ textAlign: "center", fontSize: 12, color: "var(--color-text-muted)", margin: "8px 0" }}>
            or continue with
          </div>
          {providers.map((p) => (
            <a
              key={p}
              href={`/api/v1/auth/oauth/${p}/start`}
              style={{
                display: "block", textAlign: "center", padding: "9px 12px", marginTop: 8,
                border: "1px solid var(--color-border)", borderRadius: "var(--radius-md)",
                textDecoration: "none", color: "var(--color-text)", textTransform: "capitalize",
              }}
            >
              {p}
            </a>
          ))}
        </div>
      )}
      <p style={{ fontSize: 13, marginTop: 16, color: "var(--color-text-muted)" }}>
        No account? <Link href="/register">Create one</Link> · <Link href="/forgot">Forgot password?</Link>
      </p>
    </AuthShell>
  );
}
