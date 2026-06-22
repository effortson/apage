"use client";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import {
  LayoutDashboard,
  Boxes,
  Link2,
  Files,
  Globe,
  ScrollText,
  CreditCard,
  Users,
  Settings,
  LogOut,
  type LucideIcon,
} from "lucide-react";
import { api, setTenant, getTenant } from "@/lib/api";
import { useT, LocaleToggle } from "@/lib/i18n";
import { ThemeToggle } from "@/components/theme";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

type Session = {
  user: { userId: string; email: string; emailVerified: boolean };
  tenants: { tenantId: string; name: string; plan: string; role: string }[];
};

const roleRank: Record<string, number> = { viewer: 0, member: 1, admin: 2, owner: 3 };

// Nav with the minimum role each surface requires (mirrors backend RBAC, UI §7).
const nav: [string, string, string, LucideIcon][] = [
  ["Overview", "/console", "viewer", LayoutDashboard],
  ["Instances", "/console/instances", "viewer", Boxes],
  ["Preview Links", "/console/links", "viewer", Link2],
  ["Cloud Files", "/console/files", "viewer", Files],
  ["Custom Domains", "/console/domains", "admin", Globe],
  ["Audit Logs", "/console/audit", "admin", ScrollText],
  ["Usage & Billing", "/console/usage", "admin", CreditCard],
  ["Members", "/console/members", "member", Users],
  ["Settings", "/console/settings", "member", Settings],
];

export default function ConsoleLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const t = useT();
  const [session, setSession] = useState<Session | null>(null);
  const [tenant, setTenantState] = useState<string | null>(null);

  useEffect(() => {
    api<Session>("/auth/session", { tenant: false })
      .then((s) => {
        setSession(s);
        let cur = getTenant();
        if (!cur || !s.tenants.find((x) => x.tenantId === cur)) {
          cur = s.tenants[0]?.tenantId || null;
          setTenant(cur);
        }
        setTenantState(cur);
      })
      .catch(() => router.push("/login"));
  }, [router]);

  if (!session) {
    return (
      <div className="flex min-h-screen">
        <aside className="hidden w-60 border-r p-4 md:block">
          <Skeleton className="mb-6 h-7 w-24" />
          <div className="space-y-2">
            {Array.from({ length: 7 }).map((_, i) => (
              <Skeleton key={i} className="h-8 w-full" />
            ))}
          </div>
        </aside>
        <main className="flex-1 p-6">
          <Skeleton className="h-8 w-48" />
        </main>
      </div>
    );
  }

  const current = session.tenants.find((x) => x.tenantId === tenant);
  const rank = roleRank[current?.role || "viewer"] ?? 0;
  const visibleNav = nav.filter(([, , min]) => rank >= (roleRank[min] ?? 0));
  const initials = session.user.email.slice(0, 2).toUpperCase();

  return (
    <div className="flex min-h-screen">
      <aside className="sticky top-0 hidden h-screen w-60 shrink-0 flex-col border-r bg-muted/30 md:flex">
        <div className="flex h-14 items-center px-6 text-lg font-semibold tracking-tight">
          APAGE
        </div>
        <nav className="flex-1 space-y-1 px-3 py-2">
          {visibleNav.map(([label, href, , Icon]) => {
            const active = pathname === href;
            return (
              <Link
                key={href}
                href={href}
                className={cn(
                  "flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors",
                  active
                    ? "bg-accent font-medium text-accent-foreground"
                    : "text-muted-foreground hover:bg-accent/60 hover:text-foreground",
                )}
              >
                <Icon className="h-4 w-4" />
                {t(label)}
              </Link>
            );
          })}
        </nav>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-10 flex h-14 items-center justify-between gap-3 border-b bg-background/95 px-4 backdrop-blur supports-[backdrop-filter]:bg-background/60 md:px-6">
          <Select
            value={tenant || ""}
            onValueChange={(v) => {
              setTenant(v);
              setTenantState(v);
              location.reload();
            }}
          >
            <SelectTrigger className="h-8 w-[180px]">
              <SelectValue placeholder="Select tenant" />
            </SelectTrigger>
            <SelectContent>
              {session.tenants.map((tn) => (
                <SelectItem key={tn.tenantId} value={tn.tenantId}>
                  {tn.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <div className="flex items-center gap-1.5">
            {current && (
              <Badge variant="secondary" className="hidden capitalize sm:inline-flex">
                {current.plan}
              </Badge>
            )}
            <LocaleToggle />
            <ThemeToggle />
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon" className="rounded-full">
                  <span className="flex h-7 w-7 items-center justify-center rounded-full bg-primary text-xs font-medium text-primary-foreground">
                    {initials}
                  </span>
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-56">
                <DropdownMenuLabel className="truncate font-normal">
                  <span className="block text-sm font-medium">{session.user.email}</span>
                  {current && (
                    <span className="text-xs capitalize text-muted-foreground">
                      {current.role}
                    </span>
                  )}
                </DropdownMenuLabel>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  onClick={async () => {
                    await api("/auth/logout", { method: "POST", tenant: false });
                    router.push("/login");
                  }}
                >
                  <LogOut className="mr-2 h-4 w-4" />
                  {t("Sign out")}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </header>

        <main className="mx-auto w-full max-w-screen-xl flex-1 p-4 md:p-6">
          {children}
        </main>
      </div>
    </div>
  );
}
