"use client";

import { useState } from "react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { useAuthStore } from "@/lib/store";
import { Database, Loader2 } from "lucide-react";

export function LoginPage() {
  const [token, setToken] = useState("");
  const [error, setError] = useState(false);
  const [loading, setLoading] = useState(false);
  const login = useAuthStore((s) => s.login);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(false);
    setLoading(true);
    const ok = await login(token.trim());
    setLoading(false);
    if (!ok) setError(true);
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <div className="w-full max-w-sm">
        <div className="mb-8 text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-lg bg-brand/10">
            <Database className="h-6 w-6 text-brand" />
          </div>
          <h1 className="text-xl font-semibold">SupaLite</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Enter your admin token to continue
          </p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <Input
            type="password"
            placeholder="Admin token"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            autoFocus
            className="bg-card"
          />
          {error && (
            <p className="text-sm text-destructive">Invalid token</p>
          )}
          <Button
            type="submit"
            className="w-full bg-brand text-black hover:bg-brand/90"
            disabled={loading || !token.trim()}
          >
            {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : "Login"}
          </Button>
        </form>
      </div>
    </div>
  );
}
