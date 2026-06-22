"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { api, ApiException } from "@/lib/api";
import { AuthShell } from "@/components/authshell";
import { Button, Input } from "@/components/ui";

export default function Verify() {
  const [status, setStatus] = useState<"working" | "ok" | "needsResend">("working");
  const [msg, setMsg] = useState("");
  const [email, setEmail] = useState("");

  useEffect(() => {
    const token = new URLSearchParams(window.location.search).get("token");
    if (!token) {
      setStatus("needsResend");
      return;
    }
    api("/auth/verify-email", { method: "POST", tenant: false, body: { token } })
      .then(() => setStatus("ok"))
      .catch((e) => {
        setStatus("needsResend");
        setMsg(e instanceof ApiException ? e.body.message : "This verification link is invalid or expired.");
      });
  }, []);

  return (
    <AuthShell title="Verify your email">
      {status === "working" && <p style={{ color: "var(--color-text-muted)" }}>Verifying…</p>}
      {status === "ok" && (
        <>
          <p>Your email is verified.</p>
          <Button onClick={() => (location.href = "/login")} style={{ width: "100%", marginTop: 8 }}>Continue to sign in</Button>
        </>
      )}
      {status === "needsResend" && (
        <>
          {msg && <p style={{ color: "var(--color-danger)", fontSize: 13 }}>{msg}</p>}
          <p style={{ fontSize: 13, color: "var(--color-text-muted)" }}>Enter your email to receive a new verification link.</p>
          <form
            onSubmit={async (e) => {
              e.preventDefault();
              await api("/auth/resend-verification", { method: "POST", tenant: false, body: { email } }).catch(() => {});
              setMsg("If that account still needs verification, a new link has been sent.");
            }}
          >
            <Input label="Email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
            <Button type="submit" style={{ width: "100%" }}>Resend verification</Button>
          </form>
        </>
      )}
      <p style={{ fontSize: 13, marginTop: 16, color: "var(--color-text-muted)" }}>
        <Link href="/login">Back to sign in</Link>
      </p>
    </AuthShell>
  );
}
