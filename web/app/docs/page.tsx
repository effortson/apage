import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { CodeBlock } from "@/components/composites";

// Docs (UI §6.3) — condensed quick-start + integration + security model.
export default function Docs() {
  return (
    <div className="flex min-h-screen flex-col">
      <SiteHeader />
      <main className="flex-1">
        <article className="container max-w-3xl py-16 md:py-20">
          <h1 className="text-4xl font-semibold tracking-tight md:text-5xl">
            Documentation
          </h1>
          <p className="mt-4 text-lg text-muted-foreground">
            A condensed quick-start, the integration model, and how APAGE keeps
            shared files safe.
          </p>

          <section className="mt-12 space-y-4">
            <h2 className="text-xl font-semibold tracking-tight">Quick start</h2>
            <ol className="ml-5 list-decimal space-y-4 text-muted-foreground marker:text-muted-foreground">
              <li>
                Create a tenant and an instance in the console; copy the{" "}
                <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-sm text-foreground">
                  instanceApiKey
                </code>{" "}
                (shown once).
              </li>
              <li>
                Install the CLI (verify the checksum):
                <CodeBlock className="mt-2">
                  curl -fsSL https://preview.example.com/install.sh | sh
                </CodeBlock>
              </li>
              <li>
                Point it at your instance:
                <CodeBlock className="mt-2">
                  apage-cli init --instance alice --workspace ~/outputs --api
                  https://preview.example.com --api-key apage_key_xxx
                </CodeBlock>
              </li>
              <li>
                Start the local MCP server your agent connects to:
                <CodeBlock className="mt-2">
                  apage-cli mcp # serves MCP on 127.0.0.1:7777
                </CodeBlock>
              </li>
            </ol>
          </section>

          <Separator className="my-12" />

          <section className="space-y-4">
            <h2 className="text-xl font-semibold tracking-tight">Integration</h2>
            <p className="leading-relaxed text-muted-foreground">
              Your agent connects to the local MCP server and calls its tools.
              The flow is:{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-sm text-foreground">
                create_preview_link
              </code>{" "}
              uploads a file from the workspace, waits for scanning to finish,
              and returns the share URL. Other tools cover{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-sm text-foreground">
                list_links
              </code>
              ,{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-sm text-foreground">
                revoke_link
              </code>
              , and{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-sm text-foreground">
                modify_link
              </code>
              . Files are hosted in the cloud; the console is for viewing and
              revoking links, not creating them.
            </p>
          </section>

          <Separator className="my-12" />

          <section className="space-y-4">
            <h2 className="text-xl font-semibold tracking-tight">
              API reference
            </h2>
            <p className="leading-relaxed text-muted-foreground">
              All write endpoints accept{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-sm text-foreground">
                Idempotency-Key
              </code>
              . Lists use cursor pagination (
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-sm text-foreground">
                limit
              </code>
              ,{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-sm text-foreground">
                cursor
              </code>
              ,{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-sm text-foreground">
                order
              </code>
              ) and return{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-sm text-foreground">
                {`{ items, nextCursor, hasMore }`}
              </code>
              . Errors use{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-sm text-foreground">
                {`{ error: { code, message, requestId, retryable } }`}
              </code>
              .
            </p>
          </section>

          <Separator className="my-12" />

          <section className="space-y-4">
            <h2 className="text-xl font-semibold tracking-tight">
              Security model
            </h2>
            <ul className="ml-5 list-disc space-y-2.5 leading-relaxed text-muted-foreground marker:text-muted-foreground">
              <li>
                Files are hosted in the cloud and scanned before any preview link
                goes live.
              </li>
              <li>
                The CLI uploads only from an allowlisted workspace root; paths are
                normalized, symlink-escapes and traversal blocked, hidden/executable
                files rejected.
              </li>
              <li>
                Untrusted HTML/SVG render only on the isolated render domain with
                sandbox + strict CSP.
              </li>
            </ul>
          </section>

          <Separator className="my-12" />

          <section className="space-y-4">
            <h2 className="text-xl font-semibold tracking-tight">
              Custom domains
            </h2>
            <p className="leading-relaxed text-muted-foreground">
              Add a TXT record for ownership and a CNAME to your tenant subdomain;
              the platform issues a certificate via ACME once DNS verifies.
            </p>
          </section>

          <div className="mt-14 flex flex-wrap gap-3">
            <Button asChild>
              <Link href="/register">Start free</Link>
            </Button>
            <Button asChild variant="outline">
              <Link href="/pricing">View pricing</Link>
            </Button>
          </div>
        </article>
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
