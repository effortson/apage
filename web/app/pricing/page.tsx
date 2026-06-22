import Link from "next/link";
import { Check } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";

// Pricing (UI §6.2, aligned with spec §20).
const plans = [
  {
    name: "Lite",
    price: "Free",
    feats: [
      "1 instance",
      "1GB cloud egress / month",
      "100MB cloud storage",
      "24h retention",
      "Platform subdomain only",
      "No custom domain / SSO / SLA",
    ],
  },
  {
    name: "Starter",
    price: "$—",
    feats: [
      "1 instance",
      "10GB cloud egress",
      "1GB cloud storage",
      "Longer retention",
      "1 custom domain",
    ],
  },
  {
    name: "Pro",
    price: "$—",
    feats: [
      "5 instances",
      "100GB egress",
      "50GB storage",
      "5 custom domains",
      "Audit log",
    ],
    featured: true,
  },
  {
    name: "Team",
    price: "Custom",
    feats: [
      "Custom domain",
      "SSO",
      "Advanced audit",
      "Longer retention",
      "SLA",
    ],
  },
];

const faqs = [
  {
    q: "Are my files uploaded?",
    a: "Yes — files are hosted by the platform in the cloud, scanned, and served over temporary preview links.",
  },
  {
    q: "Who creates the links?",
    a: "Your agent does, through the apage-cli MCP tool. The console is for viewing and revoking links.",
  },
  {
    q: "What happens past the free limit?",
    a: "Lite prompts you to upgrade and never silently bills you.",
  },
];

export default function Pricing() {
  return (
    <div className="flex min-h-screen flex-col">
      <SiteHeader />
      <main className="flex-1">
        <section className="container py-16 md:py-20">
          <div className="mx-auto max-w-2xl text-center">
            <h1 className="text-4xl font-semibold tracking-tight md:text-5xl">
              Pricing
            </h1>
            <p className="mt-4 text-lg text-muted-foreground">
              Billed by cloud storage, download (egress), and retention.
            </p>
          </div>

          <div className="mt-14 grid gap-6 md:grid-cols-2 lg:grid-cols-4">
            {plans.map((p) => (
              <Card
                key={p.name}
                className={p.featured ? "border-primary shadow-md" : undefined}
              >
                <CardHeader>
                  <div className="flex items-center justify-between">
                    <CardTitle className="text-lg">{p.name}</CardTitle>
                    {p.featured && <Badge>Popular</Badge>}
                  </div>
                  <CardDescription>
                    <span className="text-3xl font-semibold tracking-tight text-foreground">
                      {p.price}
                    </span>
                  </CardDescription>
                </CardHeader>
                <CardContent className="flex flex-1 flex-col">
                  <ul className="space-y-2.5 text-sm">
                    {p.feats.map((f) => (
                      <li key={f} className="flex items-start gap-2">
                        <Check className="mt-0.5 h-4 w-4 shrink-0 text-foreground/70" />
                        <span className="text-muted-foreground">{f}</span>
                      </li>
                    ))}
                  </ul>
                  <Button
                    asChild
                    variant={p.featured ? "default" : "outline"}
                    className="mt-6 w-full"
                  >
                    <Link href="/register">Choose</Link>
                  </Button>
                </CardContent>
              </Card>
            ))}
          </div>
        </section>

        <Separator className="container" />

        <section className="container py-16 md:py-20">
          <div className="mx-auto max-w-3xl">
            <h2 className="text-2xl font-semibold tracking-tight">
              Frequently asked questions
            </h2>
            <div className="mt-8 grid gap-4">
              {faqs.map((f) => (
                <Card key={f.q}>
                  <CardHeader className="pb-2">
                    <CardTitle className="text-base">{f.q}</CardTitle>
                  </CardHeader>
                  <CardContent>
                    <p className="text-sm text-muted-foreground">{f.a}</p>
                  </CardContent>
                </Card>
              ))}
            </div>
          </div>
        </section>
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
