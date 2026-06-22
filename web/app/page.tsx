import Link from "next/link";
import { ThemeToggle } from "@/components/theme";

// Landing page (UI §6.1).
export default function Home() {
  return (
    <main style={{ maxWidth: 1200, margin: "0 auto", padding: "0 24px" }}>
      <Nav />
      <Hero />
      <Modes />
      <Audience />
      <Features />
      <CodeSample />
      <Trust />
      <Footer />
    </main>
  );
}

function Nav() {
  return (
    <nav style={{ display: "flex", justifyContent: "space-between", alignItems: "center", padding: "20px 0" }}>
      <div style={{ fontWeight: 700, fontSize: 18 }}>APAGE</div>
      <div style={{ display: "flex", gap: 16, alignItems: "center" }}>
        <Link href="/pricing">Pricing</Link>
        <Link href="/docs">Docs</Link>
        <Link href="/login">Sign in</Link>
        <Link href="/register" style={{ background: "var(--color-primary)", color: "#fff", padding: "8px 14px", borderRadius: "var(--radius-md)" }}>Get started</Link>
        <ThemeToggle />
      </div>
    </nav>
  );
}

function Hero() {
  return (
    <section style={{ padding: "80px 0", textAlign: "center" }}>
      <div style={{ fontSize: 13, color: "var(--color-primary)", fontWeight: 600, marginBottom: 12 }}>PREVIEW &amp; SHARE PROVIDER FOR AGENTS</div>
      <h1 style={{ fontSize: 44, lineHeight: 1.1, maxWidth: 760, margin: "0 auto", fontWeight: 700 }}>
        Preview, share, and serve any file your agent produces — without exposing your server.
      </h1>
      <p style={{ fontSize: 18, color: "var(--color-text-muted)", maxWidth: 600, margin: "20px auto" }}>
        Temporary links, subdomains, TLS, revocation, and sandboxed previews. Files stay on your server over a reverse tunnel, or get hosted in the cloud — your choice.
      </p>
      <div style={{ display: "flex", gap: 12, justifyContent: "center", marginTop: 24 }}>
        <Link href="/register" style={{ background: "var(--color-primary)", color: "#fff", padding: "12px 24px", borderRadius: "var(--radius-md)", fontWeight: 600 }}>Start free</Link>
        <Link href="/docs" style={{ border: "1px solid var(--color-border-strong)", padding: "12px 24px", borderRadius: "var(--radius-md)", color: "var(--color-text)" }}>Read the docs</Link>
      </div>
    </section>
  );
}

function Modes() {
  const modes = [
    { t: "DNS-only", d: "Neither files nor traffic pass through the platform — you expose your own service.", tone: "var(--color-text-muted)" },
    { t: "Tunnel relay", d: "Files stay on your server. Bytes pass through the platform gateway but are never stored.", tone: "var(--color-primary)" },
    { t: "Cloud", d: "Files are uploaded and hosted by the platform, which handles preview, scanning, and sharing.", tone: "var(--color-info)" },
  ];
  return (
    <section style={{ padding: "40px 0" }}>
      <h2 style={{ textAlign: "center", marginBottom: 24 }}>Three data-flow modes — pick your trust boundary</h2>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(260px,1fr))", gap: 16 }}>
        {modes.map((m) => (
          <div key={m.t} style={{ border: "1px solid var(--color-border)", borderRadius: "var(--radius-lg)", padding: 24, background: "var(--color-bg-subtle)" }}>
            <div style={{ fontWeight: 700, color: m.tone, marginBottom: 8 }}>{m.t}</div>
            <div style={{ color: "var(--color-text-muted)", fontSize: 14 }}>{m.d}</div>
          </div>
        ))}
      </div>
    </section>
  );
}

function Audience() {
  return (
    <section style={{ padding: "40px 0", textAlign: "center", color: "var(--color-text-muted)" }}>
      <div style={{ fontSize: 13, letterSpacing: 1 }}>BUILT FOR THE AGENT ECOSYSTEM</div>
      <div style={{ display: "flex", gap: 32, justifyContent: "center", marginTop: 16, fontWeight: 600, color: "var(--color-text)" }}>
        <span>OpenClaw</span><span>Hermes Agent</span><span>Custom Agents</span>
      </div>
    </section>
  );
}

function Features() {
  const items = [
    ["Temporary links", "Short-lived, high-entropy, revocable links — secret never in query strings or logs."],
    ["Subdomains + TLS", "Per-instance subdomains with automatic TLS; bring your own custom domain."],
    ["Virus scanning", "Cloud uploads are scanned before any preview link goes live."],
    ["Sandboxed previews", "HTML/SVG render on an isolated domain with strict per-type CSP."],
    ["Access policies", "Password, account, IP allowlist, single-use, and max-views — enforced atomically."],
    ["Audit logging", "Every access and admin action is recorded; secrets are always redacted."],
  ];
  return (
    <section style={{ padding: "40px 0" }}>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(280px,1fr))", gap: 16 }}>
        {items.map(([t, d]) => (
          <div key={t} style={{ padding: 20 }}>
            <div style={{ fontWeight: 600, marginBottom: 6 }}>{t}</div>
            <div style={{ color: "var(--color-text-muted)", fontSize: 14 }}>{d}</div>
          </div>
        ))}
      </div>
    </section>
  );
}

function CodeSample() {
  const code = `# Register a file locally and share it over the tunnel
apage-agent share --instance alice --path outputs/report.pdf --expires 3600
# -> Preview ready: https://alice.preview.example.com/p/plink_.../aps_...`;
  return (
    <section style={{ padding: "40px 0" }}>
      <h2 style={{ textAlign: "center", marginBottom: 24 }}>One command from file to shareable link</h2>
      <pre style={{ background: "var(--color-bg-muted)", borderRadius: "var(--radius-lg)", padding: 24, overflowX: "auto", fontFamily: "var(--font-mono)", fontSize: 13, maxWidth: 760, margin: "0 auto" }}>{code}</pre>
    </section>
  );
}

function Trust() {
  return (
    <section style={{ padding: "60px 0", textAlign: "center" }}>
      <h2>Security &amp; trust by default</h2>
      <p style={{ color: "var(--color-text-muted)", maxWidth: 600, margin: "12px auto" }}>
        Redaction by default, ≥128-bit secrets, constant-time comparison, revocable links, path-traversal and symlink-escape protection, and full audit trails.
      </p>
    </section>
  );
}

function Footer() {
  return (
    <footer style={{ borderTop: "1px solid var(--color-border)", padding: "32px 0", color: "var(--color-text-muted)", fontSize: 13, display: "flex", justifyContent: "space-between" }}>
      <span>© APAGE</span>
      <span style={{ display: "flex", gap: 16 }}><Link href="/pricing">Pricing</Link><Link href="/docs">Docs</Link></span>
    </footer>
  );
}
