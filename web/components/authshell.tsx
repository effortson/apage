import { Card } from "@/components/ui";

export function AuthShell({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <main style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", padding: 24 }}>
      <div style={{ width: 380 }}>
        <div style={{ textAlign: "center", marginBottom: 24, fontWeight: 700, fontSize: 20 }}>APAGE</div>
        <Card>
          <h2 style={{ marginBottom: 16 }}>{title}</h2>
          {children}
        </Card>
      </div>
    </main>
  );
}
