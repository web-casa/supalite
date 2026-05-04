"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useAuthStore } from "@/lib/store";
import { api } from "@/lib/api";
import { LoginPage } from "./login-page";
import {
  LayoutDashboard,
  ScrollText,
  Settings,
  LogOut,
  Database,
  ExternalLink,
  Wrench,
  ShieldUser,
  Archive,
  Gauge,
  Activity,
  KeyRound,
} from "lucide-react";

// Admin sidebar — Phase 3 focus: DBA / ops.
// Table editor, SQL editor, auth users management → Studio (via /studio/).
// DB Ops / Roles / Backups / Performance → Phase 5 & 6 will fill these.
const navItems = [
  { href: "/admin/", icon: LayoutDashboard, label: "Dashboard" },
  { href: "/admin/logs/", icon: ScrollText, label: "Logs" },
  { href: "/admin/dbops/", icon: Wrench, label: "DB Ops" },
  { href: "/admin/sessions/", icon: Activity, label: "Sessions" },
  // Placeholders for later phases.
  { href: "/admin/roles/", icon: ShieldUser, label: "DB Roles", disabled: true },
  { href: "/admin/backups/", icon: Archive, label: "Backups" },
  { href: "/admin/secrets/", icon: KeyRound, label: "Secrets" },
  { href: "/admin/performance/", icon: Gauge, label: "Performance", disabled: true },
  { href: "/admin/settings/", icon: Settings, label: "Settings" },
];

interface ServiceStatus {
  name: string;
  state: string;
}

export function AppShell({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, checkAuth, logout } = useAuthStore();
  const pathname = usePathname();
  const [services, setServices] = useState<ServiceStatus[]>([]);

  useEffect(() => {
    checkAuth();
  }, [checkAuth]);

  useEffect(() => {
    if (!isAuthenticated) return;
    const es = api.sse("/status/stream");
    es.addEventListener("snapshot", (e: MessageEvent) => {
      try {
        const data = JSON.parse(e.data) as { services: ServiceStatus[] };
        setServices(data.services || []);
      } catch {
        /* ignore malformed payload */
      }
    });
    return () => es.close();
  }, [isAuthenticated]);

  if (!isAuthenticated) {
    return <LoginPage />;
  }

  return (
    <div className="flex h-full">
      {/* Sidebar */}
      <aside className="flex w-56 flex-col border-r border-sidebar-border bg-sidebar">
        {/* Logo */}
        <div className="flex h-14 items-center gap-2 border-b border-sidebar-border px-4">
          <Database className="h-5 w-5 text-brand" />
          <span className="text-sm font-semibold text-foreground">
            SupaLite
          </span>
        </div>

        {/* Navigation */}
        <nav className="flex-1 space-y-0.5 px-2 py-3">
          {navItems.map((item) => {
            const active = pathname === item.href || pathname === item.href.slice(0, -1);
            if (item.disabled) {
              return (
                <div
                  key={item.href}
                  className="flex items-center gap-3 rounded-md px-3 py-2 text-sm text-sidebar-foreground/40 cursor-not-allowed"
                  title="Coming in a future release"
                >
                  <item.icon className="h-4 w-4" />
                  {item.label}
                  <span className="ml-auto text-[9px] uppercase tracking-wide">
                    soon
                  </span>
                </div>
              );
            }
            return (
              <Link
                key={item.href}
                href={item.href}
                className={`flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors ${
                  active
                    ? "border-l-2 border-brand bg-sidebar-accent text-foreground"
                    : "text-sidebar-foreground hover:bg-sidebar-accent hover:text-foreground"
                }`}
              >
                <item.icon className="h-4 w-4" />
                {item.label}
              </Link>
            );
          })}

          {/* Switch to Studio — for application-dev tasks like table editing */}
          <a
            href="/studio/"
            className="mt-4 flex items-center gap-3 rounded-md border border-brand/30 px-3 py-2 text-sm text-brand hover:bg-brand/10"
          >
            <ExternalLink className="h-4 w-4" />
            Open Studio
          </a>
        </nav>

        {/* Status dots */}
        <div className="border-t border-sidebar-border px-4 py-3">
          <p className="mb-2 text-xs font-medium text-muted-foreground">
            Services
          </p>
          <div className="space-y-1.5">
            {services.map((svc) => (
              <div key={svc.name} className="flex items-center gap-2 text-xs">
                <span
                  className={`h-1.5 w-1.5 rounded-full ${
                    svc.state === "running"
                      ? "bg-brand"
                      : svc.state === "exited"
                        ? "bg-destructive"
                        : "bg-yellow-500"
                  }`}
                />
                <span className="text-sidebar-foreground">{svc.name}</span>
              </div>
            ))}
          </div>
        </div>

        {/* Logout */}
        <div className="border-t border-sidebar-border p-2">
          <button
            onClick={logout}
            className="flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm text-sidebar-foreground hover:bg-sidebar-accent hover:text-foreground"
          >
            <LogOut className="h-4 w-4" />
            Logout
          </button>
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 overflow-auto">{children}</main>
    </div>
  );
}
