"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { api, ApiException } from "@/lib/api";
import { AuthShell } from "@/components/authshell";
import { Button, Input } from "@/components/ui";

export default function Reset() {
  const [token, setToken] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState("");
  const [done, setDone] = useState(false);

  useEffect(() => {
    setToken(new URLSearchParams(window.location.search).get("token") || "");
  }, []);

  return (
    <AuthShell title="Choose a new password">
      {done ? (
        <>
          <p>Your password has been reset.</p>
          <Button onClick={() => (location.href = "/login")} style={{ width: "100%", marginTop: 8 }}>Sign in</Button>
        </>
      ) : (
        <form
          onSubmit={async (e) => {
            e.preventDefault();
            setErr("");
            try {
              await api("/auth/reset-password", { method: "POST", tenant: false, body: { token, password } });
              setDone(true);
            } catch (e) {
              setErr(e instanceof ApiException ? e.body.message : "Reset failed — the link may be expired.");
            }
          }}
        >
          <Input label="New password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} required />
          <p style={{ fontSize: 12, color: "var(--color-text-muted)", marginBottom: 8 }}>At least 10 characters with letters and digits.</p>
          {err && <div style={{ color: "var(--color-danger)", fontSize: 13, marginBottom: 12 }}>{err}</div>}
          <Button type="submit" style={{ width: "100%" }}>Reset password</Button>
        </form>
      )}
      <p style={{ fontSize: 13, marginTop: 16, color: "var(--color-text-muted)" }}>
        <Link href="/login">Back to sign in</Link>
      </p>
    </AuthShell>
  );
}
