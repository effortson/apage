import Link from "next/link";

// Docs (UI §6.3) — condensed quick-start + integration + security model.
export default function Docs() {
  return (
    <main style={{ maxWidth: 860, margin: "0 auto", padding: "40px 24px", lineHeight: 1.6 }}>
      <Link href="/">← Home</Link>
      <h1 style={{ margin: "16px 0" }}>Documentation</h1>

      <h2 style={{ marginTop: 24 }}>Quick start</h2>
      <ol>
        <li>Create a tenant and an instance in the console; copy the <code>agentToken</code> and <code>instanceApiKey</code> (shown once).</li>
        <li>Install the agent (verify the checksum):<pre style={pre}>curl -fsSL https://preview.example.com/install.sh | sh</pre></li>
        <li>Initialize and start it:<pre style={pre}>apage-agent init --instance alice --agent-type openclaw --workspace ~/.openclaw/workspace/outputs
apage-agent start --token apage_agt_xxx</pre></li>
        <li>Create your first link:<pre style={pre}>apage-agent share --instance alice --path outputs/report.pdf --expires 3600</pre></li>
      </ol>

      <h2 style={{ marginTop: 24 }}>Integration</h2>
      <p>OpenClaw: CLI helper + MCP tool. Hermes: SDK adapter + CLI helper. Custom agents: local HTTP API + REST + SDK. The tool flow is: receive path → register locally → get fileRef + metadata → create preview link → return URL.</p>

      <h2 style={{ marginTop: 24 }}>API reference</h2>
      <p>All write endpoints accept <code>Idempotency-Key</code>. Lists use cursor pagination (<code>limit</code>, <code>cursor</code>, <code>order</code>) and return <code>{`{ items, nextCursor, hasMore }`}</code>. Errors use <code>{`{ error: { code, message, requestId, retryable } }`}</code>.</p>

      <h2 style={{ marginTop: 24 }}>Security model</h2>
      <ul>
        <li>Three data-flow modes: DNS-only / Tunnel relay / Cloud.</li>
        <li>The agent serves only an allowlist root; paths are normalized, symlink-escapes and traversal blocked, hidden/executable files rejected.</li>
        <li>Untrusted HTML/SVG render only on the isolated render domain with sandbox + strict CSP.</li>
      </ul>

      <h2 style={{ marginTop: 24 }}>Custom domains</h2>
      <p>Add a TXT record for ownership and a CNAME to your tenant subdomain; the platform issues a certificate via ACME once DNS verifies.</p>
    </main>
  );
}

const pre: React.CSSProperties = { background: "var(--color-bg-muted)", padding: 12, borderRadius: 8, fontFamily: "var(--font-mono)", fontSize: 13, overflowX: "auto" };
