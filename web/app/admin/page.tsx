"use client";
import { Card, Banner, Badge, Stat } from "@/components/ui";

// Admin console shell (UI §8). The platform admin console is internal-only with
// independent auth (forced SSO + MFA, network-isolated). This shell documents
// the operational surfaces; wiring to platform-admin endpoints is production
// hardening (spec §21: admin is post-MVP).
export default function Admin() {
  return (
    <main style={{ maxWidth: 1100, margin: "0 auto", padding: 24 }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h1>APAGE Admin</h1>
        <Badge tone="danger">internal · SSO + MFA required</Badge>
      </div>
      <Banner tone="warning">
        Internal operations console. Access is network-isolated / IP-allowlisted; all actions are audited and default-redacted.
      </Banner>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(180px,1fr))", gap: 16, marginBottom: 24 }}>
        <Stat label="Online agents" value="—" />
        <Stat label="Active links" value="—" />
        <Stat label="Abuse tickets" value="—" />
        <Stat label="System alerts" value="—" />
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16 }}>
        <Card title="Tenants">Plan / trust level / quota / suspend-resume / freeze. Cross-tenant queries are audited; file contents are never shown (metadata only).</Card>
        <Card title="Abuse & takedown">Queue by source (report / scanner / blacklist), severity, SLA. Tiered actions: freeze link → instance → tenant → ban. CSAM follows legal reporting.</Card>
        <Card title="System health">Component status, per-gateway connections/streams/egress, queue depths, storage delete backlog, capacity vs scale thresholds.</Card>
        <Card title="SLO panel">Preview API availability, tunnel first-byte P95, revoke effectiveness, scanner queue latency.</Card>
        <Card title="Global audit">Cross-tenant search; admin-action audit is independently traceable; 90-day retention (compliance-adjustable).</Card>
        <Card title="Domains & certs">Verification + ACME issuance/renewal ops; render-domain reputation monitoring (Safe Browsing / blacklist).</Card>
      </div>
    </main>
  );
}
