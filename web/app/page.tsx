import Link from "next/link";
import {
  Link2,
  Globe,
  ShieldCheck,
  MonitorPlay,
  KeyRound,
  ScrollText,
  ArrowRight,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { CodeBlock } from "@/components/composites";

// Landing page (UI §6.1).
export default function Home() {
  return (
    <div className="flex min-h-screen flex-col">
      <SiteHeader />
      <main className="flex-1">
        <Hero />
        <Audience />
        <Features />
        <CodeSample />
        <Trust />
      </main>
      <SiteFooter />
    </div>
  );
}

function SiteHeader() {
  return (
    <header className="sticky top-0 z-40 border-b bg-background/80 backdrop-blur">
      <div className="container flex h-16 items-center justify-between">
        <Link href="/" className="text-lg font-semibold tracking-tight">
          APAGE
        </Link>
        <nav className="flex items-center gap-1 sm:gap-2">
          <Button asChild variant="ghost" size="sm">
            <Link href="/docs">Docs</Link>
          </Button>
          <Button asChild variant="ghost" size="sm">
            <Link href="/pricing">Pricing</Link>
          </Button>
          <Button asChild variant="ghost" size="sm">
            <Link href="/login">Sign in</Link>
          </Button>
          <Button asChild size="sm">
            <Link href="/register">Get started</Link>
          </Button>
        </nav>
      </div>
    </header>
  );
}

function Hero() {
  return (
    <section className="container py-24 text-center md:py-32">
      <Badge variant="secondary" className="mb-6">
        Preview &amp; share provider for agents
      </Badge>
      <h1 className="mx-auto max-w-3xl text-4xl font-semibold tracking-tight md:text-6xl">
        Preview, share, and serve any file your agent produces.
      </h1>
      <p className="mx-auto mt-6 max-w-2xl text-lg text-muted-foreground">
        Temporary links, subdomains, TLS, revocation, and sandboxed previews.
        Files are hosted in the cloud, and links are created by your agent
        through an MCP tool — no manual steps.
      </p>
      <div className="mt-8 flex flex-wrap items-center justify-center gap-3">
        <Button asChild size="lg">
          <Link href="/register">
            Start free <ArrowRight />
          </Link>
        </Button>
        <Button asChild size="lg" variant="outline">
          <Link href="/docs">Read the docs</Link>
        </Button>
      </div>
    </section>
  );
}

function Audience() {
  const names = ["OpenClaw", "Hermes Agent", "Custom Agents"];
  return (
    <section className="container pb-8">
      <p className="text-center text-xs font-medium uppercase tracking-widest text-muted-foreground">
        Built for the agent ecosystem
      </p>
      <div className="mt-5 flex flex-wrap items-center justify-center gap-x-10 gap-y-3 text-sm font-medium text-foreground/80">
        {names.map((n) => (
          <span key={n}>{n}</span>
        ))}
      </div>
    </section>
  );
}

const features = [
  {
    icon: Link2,
    title: "Temporary links",
    desc: "Short-lived, high-entropy, revocable links — secret never in query strings or logs.",
  },
  {
    icon: Globe,
    title: "Subdomains + TLS",
    desc: "Per-instance subdomains with automatic TLS; bring your own custom domain.",
  },
  {
    icon: ShieldCheck,
    title: "Virus scanning",
    desc: "Cloud uploads are scanned before any preview link goes live.",
  },
  {
    icon: MonitorPlay,
    title: "Sandboxed previews",
    desc: "HTML/SVG render on an isolated domain with strict per-type CSP.",
  },
  {
    icon: KeyRound,
    title: "Access policies",
    desc: "Password, account, IP allowlist, single-use, and max-views — enforced atomically.",
  },
  {
    icon: ScrollText,
    title: "Audit logging",
    desc: "Every access and admin action is recorded; secrets are always redacted.",
  },
];

function Features() {
  return (
    <section className="container py-20">
      <div className="mx-auto max-w-2xl text-center">
        <h2 className="text-3xl font-semibold tracking-tight">
          Everything sharing should be
        </h2>
        <p className="mt-3 text-muted-foreground">
          Secure defaults, cloud hosting, and agent-native delivery in one
          provider.
        </p>
      </div>
      <div className="mt-12 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {features.map(({ icon: Icon, title, desc }) => (
          <Card key={title}>
            <CardHeader>
              <div className="mb-2 flex h-10 w-10 items-center justify-center rounded-lg border bg-muted">
                <Icon className="h-5 w-5" />
              </div>
              <CardTitle>{title}</CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground">{desc}</p>
            </CardContent>
          </Card>
        ))}
      </div>
    </section>
  );
}

function CodeSample() {
  const code = `# Start the local MCP server your agent connects to
apage-cli init --instance alice --workspace ~/outputs --api https://preview.example.com --api-key apage_key_xxx
apage-cli mcp
# Your agent calls the create_preview_link MCP tool:
# -> Preview ready: https://alice.preview.example.com/p/plink_.../aps_...`;
  return (
    <section className="container py-20">
      <div className="mx-auto max-w-3xl">
        <div className="mb-8 text-center">
          <h2 className="text-3xl font-semibold tracking-tight">
            Your agent shares files through one MCP tool
          </h2>
          <p className="mt-3 text-muted-foreground">
            Point the CLI at your instance, run the MCP server, and let the agent
            do the rest.
          </p>
        </div>
        <CodeBlock>{code}</CodeBlock>
      </div>
    </section>
  );
}

function Trust() {
  return (
    <section className="container py-20">
      <Card className="mx-auto max-w-3xl">
        <CardContent className="px-8 py-12 text-center">
          <h2 className="text-2xl font-semibold tracking-tight">
            Security &amp; trust by default
          </h2>
          <p className="mx-auto mt-3 max-w-2xl text-muted-foreground">
            Redaction by default, ≥128-bit secrets, constant-time comparison,
            revocable links, path-traversal and symlink-escape protection, and
            full audit trails.
          </p>
          <Button asChild className="mt-6">
            <Link href="/register">Get started</Link>
          </Button>
        </CardContent>
      </Card>
    </section>
  );
}

function SiteFooter() {
  return (
    <footer className="border-t">
      <div className="container flex flex-col items-center justify-between gap-4 py-8 text-sm text-muted-foreground sm:flex-row">
        <span>© APAGE</span>
        <div className="flex items-center gap-6">
          <Link href="/pricing" className="hover:text-foreground">
            Pricing
          </Link>
          <Separator orientation="vertical" className="h-4" />
          <Link href="/docs" className="hover:text-foreground">
            Docs
          </Link>
        </div>
      </div>
    </footer>
  );
}
