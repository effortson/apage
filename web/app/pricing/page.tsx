import Link from "next/link";

// Pricing (UI §6.2, aligned with spec §20).
const plans = [
  { name: "Lite", price: "Free", feats: ["1 instance", "1GB tunnel / month", "100MB cloud storage", "24h retention", "Platform subdomain only", "No custom domain / SSO / SLA"] },
  { name: "Starter", price: "$—", feats: ["1 instance", "10GB tunnel traffic", "1GB cloud storage", "Longer retention", "1 custom domain"] },
  { name: "Pro", price: "$—", feats: ["5 instances", "100GB traffic", "50GB storage", "5 custom domains", "Audit log"] },
  { name: "Team", price: "Custom", feats: ["Custom domain", "SSO", "Advanced audit", "Longer retention", "SLA"] },
];

export default function Pricing() {
  return (
    <main style={{ maxWidth: 1200, margin: "0 auto", padding: "40px 24px" }}>
      <Link href="/">← Home</Link>
      <h1 style={{ textAlign: "center", margin: "24px 0" }}>Pricing</h1>
      <p style={{ textAlign: "center", color: "var(--color-text-muted)", marginBottom: 32 }}>
        Tunnel billed by instances / forwarded traffic / domains. Cloud billed by storage / download / conversions / retention.
      </p>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(240px,1fr))", gap: 16 }}>
        {plans.map((p) => (
          <div key={p.name} style={{ border: "1px solid var(--color-border)", borderRadius: "var(--radius-lg)", padding: 24, background: "var(--color-bg-subtle)" }}>
            <div style={{ fontWeight: 700, fontSize: 18 }}>{p.name}</div>
            <div style={{ fontSize: 28, fontWeight: 700, margin: "8px 0" }}>{p.price}</div>
            <ul style={{ listStyle: "none", padding: 0, color: "var(--color-text-muted)", fontSize: 14 }}>
              {p.feats.map((f) => <li key={f} style={{ padding: "4px 0" }}>✓ {f}</li>)}
            </ul>
            <Link href="/register" style={{ display: "block", textAlign: "center", marginTop: 16, background: "var(--color-primary)", color: "#fff", padding: "10px", borderRadius: "var(--radius-md)" }}>Choose</Link>
          </div>
        ))}
      </div>
      <div style={{ marginTop: 40, color: "var(--color-text-muted)", fontSize: 14 }}>
        <h3>FAQ</h3>
        <p><b>Are my files uploaded?</b> Not in Tunnel mode — files stay on your server. In Cloud mode they are hosted by the platform.</p>
        <p><b>Does Tunnel relay pass through the platform?</b> Bytes transit the gateway but are never stored.</p>
        <p><b>What happens past the free limit?</b> Lite prompts you to upgrade and never silently bills you.</p>
      </div>
    </main>
  );
}
