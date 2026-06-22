"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { api, ApiException } from "@/lib/api";
import { AuthShell } from "@/components/authshell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";

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

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    await api("/auth/resend-verification", { method: "POST", tenant: false, body: { email } }).catch(() => {});
    setMsg("If that account still needs verification, a new link has been sent.");
  }

  return (
    <AuthShell
      title="Verify your email"
      footer={
        <Link href="/login" className="text-foreground underline-offset-4 hover:underline">
          Back to sign in
        </Link>
      }
    >
      {status === "working" && (
        <p className="text-center text-sm text-muted-foreground">Verifying…</p>
      )}
      {status === "ok" && (
        <div className="space-y-4">
          <p className="text-center text-sm text-muted-foreground">Your email is verified.</p>
          <Button onClick={() => (location.href = "/login")} className="w-full">
            Continue to sign in
          </Button>
        </div>
      )}
      {status === "needsResend" && (
        <div className="space-y-4">
          {msg && (
            <Alert>
              <AlertDescription>{msg}</AlertDescription>
            </Alert>
          )}
          <p className="text-sm text-muted-foreground">Enter your email to receive a new verification link.</p>
          <form onSubmit={submit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="email">Email</Label>
              <Input id="email" type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
            </div>
            <Button type="submit" className="w-full">
              Resend verification
            </Button>
          </form>
        </div>
      )}
    </AuthShell>
  );
}
