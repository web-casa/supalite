"use client";

import "./globals.css";
import { Toaster } from "@/components/ui/sonner";
import { AppShell } from "@/components/app-shell";
import { QueryProvider } from "@/lib/query";

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" className="dark h-full">
      <head>
        <title>SupaLite admin</title>
      </head>
      <body className="h-full bg-background text-foreground antialiased font-sans">
        <QueryProvider>
          <AppShell>{children}</AppShell>
          <Toaster theme="dark" position="bottom-right" />
        </QueryProvider>
      </body>
    </html>
  );
}
