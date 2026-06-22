"use client";
import { useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api";
import { AuthShell } from "@/components/authshell";
import { Button, Input } from "@/components/ui";

export default function Forgot() {
  const [email, setEmail] = useState("");
  const [sent, setSent] = useState(false);

  return (
    <AuthShell title="Reset your password">
      {sent ? (
        <p style={{ color: "var(--color-text-muted)" }}>
          If an account exists for that email, a password-reset link has been sent.
        </p>
      ) : (
        <form
          onSubmit={async (e) => {
            e.preventDefault();
            // Always succeeds (anti-enumeration) — the backend returns 200 regardless.
            await api("/auth/forgot-password", { method: "POST", tenant: false, body: { email } }).catch(() => {});
            setSent(true);
          }}
        >
          <Input label="Email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
          <Button type="submit" style={{ width: "100%" }}>Send reset link</Button>
        </form>
      )}
      <p style={{ fontSize: 13, marginTop: 16, color: "var(--color-text-muted)" }}>
        <Link href="/login">Back to sign in</Link>
      </p>
    </AuthShell>
  );
}
