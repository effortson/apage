"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { api, ApiException } from "@/lib/api";
import { AuthShell } from "@/components/authshell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";

export default function Reset() {
  const [token, setToken] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);
  const [done, setDone] = useState(false);

  useEffect(() => {
    setToken(new URLSearchParams(window.location.search).get("token") || "");
  }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr("");
    setLoading(true);
    try {
      await api("/auth/reset-password", { method: "POST", tenant: false, body: { token, password } });
      setDone(true);
    } catch (e) {
      setErr(e instanceof ApiException ? e.body.message : "Reset failed — the link may be expired.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <AuthShell
      title="Choose a new password"
      footer={
        <Link href="/login" className="text-foreground underline-offset-4 hover:underline">
          Back to sign in
        </Link>
      }
    >
      {done ? (
        <div className="space-y-4">
          <p className="text-center text-sm text-muted-foreground">Your password has been reset.</p>
          <Button onClick={() => (location.href = "/login")} className="w-full">
            Sign in
          </Button>
        </div>
      ) : (
        <form onSubmit={submit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="password">New password</Label>
            <Input id="password" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
            <p className="text-xs text-muted-foreground">At least 10 characters with letters and digits.</p>
          </div>
          {err && (
            <Alert variant="destructive">
              <AlertDescription>{err}</AlertDescription>
            </Alert>
          )}
          <Button type="submit" disabled={loading} className="w-full">
            {loading ? "Resetting…" : "Reset password"}
          </Button>
        </form>
      )}
    </AuthShell>
  );
}
