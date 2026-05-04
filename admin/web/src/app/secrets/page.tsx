"use client";

import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api, errorMessage } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { toast } from "sonner";
import { AlertTriangle, KeyRound, Loader2, RefreshCw } from "lucide-react";

interface CatalogEntry {
  key: string;
  label: string;
  description: string;
  consequence: string;
  restart: string[];
}

interface RotateResponse {
  ok: boolean;
  updated: string[];
  restart: string[];
  notes?: string;
}

export default function SecretsPage() {
  const [catalog, setCatalog] = useState<CatalogEntry[]>([]);
  const [confirm, setConfirm] = useState<CatalogEntry | null>(null);
  const [typed, setTyped] = useState("");
  const [rotating, setRotating] = useState(false);
  const [lastResult, setLastResult] = useState<{
    key: string;
    res: RotateResponse;
  } | null>(null);
  const queryClient = useQueryClient();

  useEffect(() => {
    api
      .get<{ secrets: CatalogEntry[] }>("/secrets")
      .then((d) => setCatalog(d.secrets))
      .catch((e) => toast.error(errorMessage(e, "load failed")));
  }, []);

  async function rotate() {
    if (!confirm) return;
    setRotating(true);
    try {
      const res = await api.post<RotateResponse>("/secrets/rotate", {
        key: confirm.key,
      });
      toast.success(`${confirm.key} rotated`);
      setLastResult({ key: confirm.key, res });
      // Bust cached values that include rotated secrets — most
      // notably the Dashboard's keys query, which surfaces ANON_KEY.
      // Without this, navigating to the Dashboard immediately after
      // a JWT rotation can show the old ANON_KEY for up to staleTime
      // (5s) before refetch.
      queryClient.invalidateQueries({ queryKey: ["dashboard"] });
      setConfirm(null);
      setTyped("");
    } catch (e) {
      toast.error(errorMessage(e, "rotation failed"));
    } finally {
      setRotating(false);
    }
  }

  return (
    <div className="p-6 space-y-4 max-w-3xl">
      <div>
        <h1 className="text-lg font-semibold">Secrets</h1>
        <p className="text-sm text-muted-foreground">
          Rotate credentials. Each rotation atomically updates{" "}
          <code>.env</code>; affected services need a restart for the new
          value to take effect.
        </p>
      </div>

      <Alert>
        <AlertTriangle className="h-4 w-4" />
        <AlertDescription>
          <strong>POSTGRES_PASSWORD is not rotatable here.</strong> The
          required <code>ALTER USER</code> for 12 supabase roles has too many
          partial-failure modes for a one-click wizard. See the README for
          the manual procedure.
        </AlertDescription>
      </Alert>

      {lastResult && (
        <div className="rounded-lg border border-brand/30 bg-brand/5 p-4 text-sm space-y-2">
          <div className="flex items-center gap-2 font-semibold text-brand">
            <RefreshCw className="h-4 w-4" />
            {lastResult.key} rotated
          </div>
          <p className="text-xs text-muted-foreground">
            Updated env keys:{" "}
            {lastResult.res.updated.map((k) => (
              <code key={k} className="mr-1">
                {k}
              </code>
            ))}
          </p>
          {lastResult.res.restart.length > 0 ? (
            <p className="text-xs">
              <span className="font-semibold">Restart these services:</span>{" "}
              {lastResult.res.restart.map((s) => (
                <code key={s} className="mr-1">
                  {s}
                </code>
              ))}
              <br />
              <span className="text-muted-foreground">
                Use Settings → Restart Services, or run the equivalent{" "}
                <code>docker compose up -d --no-deps {lastResult.res.restart.join(" ")}</code>{" "}
                on the host.
              </span>
            </p>
          ) : (
            <p className="text-xs text-muted-foreground">
              No restart required — admin re-reads <code>.env</code> per request.
            </p>
          )}
          {lastResult.res.notes && (
            <p className="text-xs text-muted-foreground">{lastResult.res.notes}</p>
          )}
        </div>
      )}

      <div className="grid gap-3">
        {catalog.map((s) => (
          <div key={s.key} className="rounded-lg border p-4 space-y-2">
            <div className="flex items-start justify-between gap-3">
              <div className="flex-1">
                <div className="flex items-center gap-2 mb-1">
                  <KeyRound className="h-3.5 w-3.5 text-muted-foreground" />
                  <code className="text-xs">{s.key}</code>
                </div>
                <p className="text-sm font-semibold">{s.label}</p>
                <p className="text-xs text-muted-foreground mt-1">
                  {s.description}
                </p>
                <p className="text-xs text-destructive mt-1.5">
                  <strong>Consequence:</strong> {s.consequence}
                </p>
                {s.restart.length > 0 && (
                  <p className="text-[11px] text-muted-foreground mt-1.5">
                    Requires restart of:{" "}
                    {s.restart.map((r) => (
                      <code key={r} className="mr-1">
                        {r}
                      </code>
                    ))}
                  </p>
                )}
              </div>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setConfirm(s);
                  setTyped("");
                }}
              >
                <RefreshCw className="mr-2 h-3.5 w-3.5" />
                Rotate
              </Button>
            </div>
          </div>
        ))}
      </div>

      <Dialog
        open={confirm !== null}
        onOpenChange={(o) => {
          if (!o) {
            setConfirm(null);
            setTyped("");
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="text-destructive">
              Rotate {confirm?.key}?
            </DialogTitle>
            <DialogDescription>{confirm?.consequence}</DialogDescription>
          </DialogHeader>
          <div className="space-y-2 text-sm">
            <p>
              Type the secret key to confirm:{" "}
              <code className="text-foreground">{confirm?.key}</code>
            </p>
          </div>
          <Input
            autoFocus
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            placeholder={confirm?.key}
            className="font-mono"
          />
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setConfirm(null);
                setTyped("");
              }}
              disabled={rotating}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={rotate}
              disabled={rotating || typed !== confirm?.key}
            >
              {rotating && (
                <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
              )}
              Rotate
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
