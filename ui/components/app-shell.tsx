"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { clsx } from "clsx";
import type { ReactNode } from "react";

import { ThemeToggle } from "@/components/theme-toggle";

const navItems = [
  { href: "/launchpad", label: "Launchpad" },
  { href: "/divergence", label: "Divergence" },
  { href: "/gauntlet", label: "Gauntlet" },
  { href: "/shadow-replay", label: "Shadow Replay" },
];

export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();

  return (
    <div className="mission-stage min-h-screen bg-background text-foreground">
      <div className="mission-shell min-h-screen">
      <header className="mission-header sticky top-0 z-50 border-b backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="container mx-auto px-6">
          <div className="flex h-14 items-center gap-10">
            <Link href="/launchpad" className="shrink-0 rounded-md px-2 py-1">
              <div className="flex items-center gap-2">
                <span className="energy-badge rounded-full border border-primary/20 bg-primary/12 px-2.5 py-1 text-[10px] font-semibold uppercase tracking-[0.28em] text-primary">
                  Replay
                </span>
                <span className="text-sm font-medium text-muted-foreground">Mission Control</span>
              </div>
            </Link>

            <nav className="flex items-center gap-1">
              {navItems.map((item) => {
                const isActive =
                  pathname === item.href ||
                  (item.href !== "/shadow-replay" && pathname.startsWith(item.href)) ||
                  (item.href === "/shadow-replay" && pathname === "/shadow-replay");

                return (
                  <Link
                    aria-current={isActive ? "page" : undefined}
                    key={item.href}
                    href={item.href}
                    className={clsx(
                      "relative rounded-md px-3 py-1.5 text-[15px] font-medium transition-colors",
                      isActive
                        ? "text-foreground after:absolute after:bottom-[-13px] after:left-1 after:right-1 after:h-[2px] after:rounded-full after:bg-primary"
                        : "text-muted-foreground hover:text-foreground",
                    )}
                  >
                    {item.label}
                  </Link>
                );
              })}
            </nav>

            <div className="ml-auto flex items-center gap-4">
              <p className="hidden text-sm text-muted-foreground xl:block">
                Shadow Replay evidence, Divergence queues, and Gauntlet verdicts in one Mission Control.
              </p>
              <ThemeToggle />
            </div>
          </div>
        </div>
      </header>

      <main className="container mx-auto min-w-0 px-6 py-8">{children}</main>
      <div className="border-t">
        <div className="container mx-auto flex min-h-12 items-center justify-between px-6 text-sm text-muted-foreground">
          <span>Track Divergences. Separate Gauntlet rejection from system failure.</span>
          <span>Approve only when the evidence is clean.</span>
        </div>
      </div>
      </div>
    </div>
  );
}
