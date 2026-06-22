import "./globals.css";
import type { Metadata } from "next";
import { ToastProvider } from "@/components/ui";

export const metadata: Metadata = {
  title: "APAGE — Preview & Share Provider for Agents",
  description: "File preview, temporary sharing, and subdomain access for agents. DNS + Tunnel + Cloud.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        <ToastProvider>{children}</ToastProvider>
      </body>
    </html>
  );
}
