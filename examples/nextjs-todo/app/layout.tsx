import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "SupaLite — todo example",
  description: "Minimal Next.js + supabase-js + RLS demo",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
