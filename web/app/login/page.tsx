"use client";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { api, ApiException } from "@/lib/api";
import { AuthShell } from "@/components/authshell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";

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
    setErr("");
    setLoading(true);
    try {
      await api("/auth/login", { method: "POST", tenant: false, body: { email, password } });
      router.push("/console");
    } catch (e) {
      setErr(e instanceof ApiException ? e.body.message : "Login failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <AuthShell
      title="Sign in to APAGE"
      footer={
        <>
          No account?{" "}
          <Link href="/register" className="text-foreground underline-offset-4 hover:underline">
            Create one
          </Link>
          {" · "}
          <Link href="/forgot" className="text-foreground underline-offset-4 hover:underline">
            Forgot password?
          </Link>
        </>
      }
    >
      <form onSubmit={submit} className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="email">Email</Label>
          <Input id="email" type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
        </div>
        <div className="space-y-2">
          <Label htmlFor="password">Password</Label>
          <Input id="password" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
        </div>
        {err && (
          <Alert variant="destructive">
            <AlertDescription>{err}</AlertDescription>
          </Alert>
        )}
        <Button type="submit" disabled={loading} className="w-full">
          {loading ? "Signing in…" : "Sign in"}
        </Button>
      </form>

      {providers.length > 0 && (
        <div className="mt-4">
          <div className="relative my-4">
            <div className="absolute inset-0 flex items-center">
              <span className="w-full border-t" />
            </div>
            <div className="relative flex justify-center text-xs uppercase">
              <span className="bg-card px-2 text-muted-foreground">or continue with</span>
            </div>
          </div>
          <div className="space-y-2">
            {providers.map((p) => (
              <Button key={p} asChild variant="outline" className="w-full capitalize">
                <a href={`/api/v1/auth/oauth/${p}/start`}>{p}</a>
              </Button>
            ))}
          </div>
        </div>
      )}
    </AuthShell>
  );
}
