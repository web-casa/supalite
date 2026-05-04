"use client";

import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, errorMessage } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { toast } from "sonner";
import {
  Loader2,
  RefreshCw,
  Zap,
  HardDrive,
  Play,
  RotateCcw,
} from "lucide-react";

interface StanzaBackup {
  label: string;
  type: "full" | "diff" | "incr";
  timestamp: { start: number; stop: number };
}

interface Stanza {
  name: string;
  status: { code: number; message: string };
  backup?: StanzaBackup[];
  db?: { id: number; "system-id": number; version: string }[];
}

interface LastRun {
  type?: string;
  started?: string;
  finished?: string;
  ok: boolean;
  exit_code: number;
  stderr?: string;
  duration?: string;
}

interface StatusResponse {
  running: boolean;
  last?: LastRun;
}

interface RestoreState {
  phase: string; // stopping | restoring | starting | done | error
  set?: string;
  started?: string;
  finished?: string;
  ok: boolean;
  error?: string;
  stderr?: string;
  duration?: string;
}

interface RestoreStatusResponse {
  running: boolean;
  state?: RestoreState;
}

type BackupType = "full" | "diff" | "incr";

const phaseLabel: Record<string, string> = {
  stopping: "Stopping db…",
  restoring: "Restoring from backup…",
  starting: "Starting db…",
  done: "Done",
  error: "Error",
};

export function PgbackrestCard() {
  const [confirmRestore, setConfirmRestore] = useState<StanzaBackup | null>(null);
  const [typedLabel, setTypedLabel] = useState("");
  const queryClient = useQueryClient();

  const wasBackupRunning = useRef(false);
  const wasRestoreRunning = useRef(false);

  // info: refetched manually (after init / after a backup or restore
  // transitions to idle). No polling.
  const infoQuery = useQuery({
    queryKey: ["pgbackrest", "info"],
    queryFn: () => api.get<{ ok: boolean; info?: Stanza[] }>("/pgbackrest/info"),
  });
  const stanzas = infoQuery.data?.info ?? null;
  const error = infoQuery.error
    ? errorMessage(infoQuery.error, "info failed")
    : null;
  const loading = infoQuery.isFetching;

  // backup + restore status, polled only while one is running. Each
  // query fetches independently so a single endpoint outage doesn't
  // mask the other's authoritative state (refactor of the previous
  // Promise.allSettled trick).
  const statusQuery = useQuery({
    queryKey: ["pgbackrest", "status"],
    queryFn: () => api.get<StatusResponse>("/pgbackrest/status"),
    refetchInterval: (query) =>
      query.state.data?.running ||
      queryClient.getQueryData<RestoreStatusResponse>([
        "pgbackrest",
        "restore-status",
      ])?.running
        ? 3000
        : false,
  });
  const status = statusQuery.data ?? { running: false };

  const restoreStatusQuery = useQuery({
    queryKey: ["pgbackrest", "restore-status"],
    queryFn: () =>
      api.get<RestoreStatusResponse>("/pgbackrest/restore/status"),
    refetchInterval: (query) =>
      query.state.data?.running ||
      queryClient.getQueryData<StatusResponse>(["pgbackrest", "status"])
        ?.running
        ? 3000
        : false,
  });
  const restoreStatus = restoreStatusQuery.data ?? { running: false };

  // Running→idle transition detection. Same intent as the prior effect:
  // toast result + invalidate info so the new backup appears.
  useEffect(() => {
    const isRunning = status.running || restoreStatus.running;
    if (!isRunning) {
      if (wasBackupRunning.current) {
        wasBackupRunning.current = false;
        queryClient.invalidateQueries({ queryKey: ["pgbackrest", "info"] });
        if (status.last) {
          if (status.last.ok) {
            toast.success(
              `${status.last.type} backup complete (${status.last.duration})`
            );
          } else {
            toast.error(
              `${status.last.type} backup failed (exit ${status.last.exit_code})`
            );
          }
        }
      }
      if (wasRestoreRunning.current) {
        wasRestoreRunning.current = false;
        queryClient.invalidateQueries({ queryKey: ["pgbackrest", "info"] });
        const st = restoreStatus.state;
        if (st) {
          if (st.ok) {
            toast.success(`Restore from ${st.set} complete (${st.duration})`);
          } else {
            toast.error(
              `Restore failed at ${st.phase}: ${st.error || "unknown"}`
            );
          }
        }
      }
      return;
    }
    if (status.running) wasBackupRunning.current = true;
    if (restoreStatus.running) wasRestoreRunning.current = true;
  }, [
    status.running,
    status.last,
    restoreStatus.running,
    restoreStatus.state,
    queryClient,
  ]);

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ["pgbackrest"] });
  };
  // Initialize wizard: kept as plain async to mirror previous UX.
  const [initializing, setInitializing] = useState(false);

  async function initStanza() {
    setInitializing(true);
    try {
      await api.post("/pgbackrest/stanza-create");
      toast.success("Stanza created");
      refresh();
    } catch (e) {
      toast.error(errorMessage(e, "init failed"));
    } finally {
      setInitializing(false);
    }
  }

  // Optimistically flip status to running so destructive buttons
  // disable immediately. The next /status poll (≤3s) corrects the
  // shape with the real server-side state.
  const backupMutation = useMutation({
    mutationFn: (type: BackupType) =>
      api.post("/pgbackrest/backup", { type }),
    onSuccess: (_d, type) => {
      toast.message(`${type} backup started — this may take a while`);
      queryClient.setQueryData<StatusResponse>(
        ["pgbackrest", "status"],
        {
          running: true,
          last: {
            ok: false,
            exit_code: 0,
            type,
            started: new Date().toISOString(),
          },
        },
      );
    },
    onError: (e) => toast.error(errorMessage(e, "backup failed to start")),
  });

  const restoreMutation = useMutation({
    mutationFn: (label: string) =>
      api.post("/pgbackrest/restore", { set: label }),
    onSuccess: (_d, label) => {
      toast.message(`Restore from ${label} started`);
      queryClient.setQueryData<RestoreStatusResponse>(
        ["pgbackrest", "restore-status"],
        {
          running: true,
          state: {
            phase: "stopping",
            set: label,
            started: new Date().toISOString(),
            ok: false,
          },
        },
      );
      setConfirmRestore(null);
      setTypedLabel("");
    },
    onError: (e) => toast.error(errorMessage(e, "restore failed to start")),
  });

  const runBackup = (type: BackupType) => backupMutation.mutate(type);
  const submitRestore = () => {
    if (confirmRestore) restoreMutation.mutate(confirmRestore.label);
  };
  const startingRestore = restoreMutation.isPending;

  const notInitialized = stanzas !== null && stanzas.length === 0;
  const stanza = stanzas && stanzas.length > 0 ? stanzas[0] : null;
  const anyRunning = status.running || restoreStatus.running;
  const canBackup = stanza !== null && !anyRunning;
  const canRestore = stanza !== null && !anyRunning;

  return (
    <div className="rounded-lg border p-4">
      <div className="flex items-start justify-between mb-3">
        <div className="flex items-center gap-2">
          <HardDrive className="h-4 w-4 text-muted-foreground" />
          <div>
            <h2 className="text-sm font-semibold">pgBackRest</h2>
            <p className="text-xs text-muted-foreground">
              Physical incremental backups — opt-in, requires <code>.env</code> config + Postgres restart
            </p>
          </div>
        </div>
        <Button variant="ghost" size="sm" onClick={refresh} disabled={loading}>
          {loading ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <RefreshCw className="h-3.5 w-3.5" />
          )}
        </Button>
      </div>

      {error && (
        <div className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-xs font-mono text-destructive">
          {error}
        </div>
      )}

      {!error && notInitialized && (
        <div className="space-y-2">
          <p className="text-xs text-muted-foreground">
            Stanza not initialized. Set <code>PGBACKREST_ARCHIVE_MODE=on</code>{" "}
            in <code>.env</code>, restart the db container, then click Initialize.
          </p>
          <Button
            size="sm"
            onClick={initStanza}
            disabled={initializing}
            className="bg-brand text-black hover:bg-brand/90"
          >
            {initializing ? (
              <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
            ) : (
              <Zap className="mr-2 h-3.5 w-3.5" />
            )}
            Initialize Stanza
          </Button>
        </div>
      )}

      {!error && stanza && (
        <div className="space-y-3 text-xs">
          <div className="grid grid-cols-[120px_1fr] gap-y-1">
            <span className="text-muted-foreground">Stanza</span>
            <span className="font-mono">{stanza.name}</span>
            <span className="text-muted-foreground">Status</span>
            <span
              className={
                stanza.status.code === 0 ? "text-brand" : "text-destructive"
              }
            >
              {stanza.status.message}
            </span>
            {stanza.db && stanza.db[0] && (
              <>
                <span className="text-muted-foreground">PG version</span>
                <span className="font-mono">{stanza.db[0].version}</span>
              </>
            )}
          </div>

          {/* Running banner for restore — dominates the UI when active */}
          {restoreStatus.running && restoreStatus.state && (
            <div className="rounded-md border border-destructive/50 bg-destructive/10 p-3 text-[11px]">
              <div className="flex items-center gap-2 font-semibold text-destructive">
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                Restore in progress — {phaseLabel[restoreStatus.state.phase] || restoreStatus.state.phase}
              </div>
              <div className="mt-1 font-mono text-muted-foreground">
                from {restoreStatus.state.set}
              </div>
            </div>
          )}

          {/* Backup action buttons */}
          <div className="flex items-center gap-2 pt-2 border-t">
            <span className="text-muted-foreground">Run backup:</span>
            {(["full", "diff", "incr"] as const).map((t) => (
              <Button
                key={t}
                variant="outline"
                size="sm"
                disabled={!canBackup}
                onClick={() => runBackup(t)}
              >
                {status.running && status.last?.type === t ? (
                  <Loader2 className="mr-1 h-3 w-3 animate-spin" />
                ) : (
                  <Play className="mr-1 h-3 w-3" />
                )}
                {t}
              </Button>
            ))}
            {status.running && (
              <span className="text-muted-foreground text-[11px]">
                {status.last?.type} backup running…
              </span>
            )}
          </div>

          {/* Backup list with Restore actions */}
          {stanza.backup && stanza.backup.length > 0 ? (
            <div>
              <p className="text-muted-foreground mb-1">
                Recent backups ({stanza.backup.length}):
              </p>
              <ul className="space-y-0.5 font-mono text-[11px]">
                {stanza.backup
                  .slice(-10)
                  .reverse()
                  .map((b) => (
                    <li
                      key={b.label}
                      className="flex items-center gap-2 rounded hover:bg-muted/30 px-1 py-0.5"
                    >
                      <span className="text-muted-foreground w-12">{b.type}</span>
                      <span className="flex-1">{b.label}</span>
                      <button
                        type="button"
                        disabled={!canRestore}
                        onClick={() => {
                          setConfirmRestore(b);
                          setTypedLabel("");
                        }}
                        className="text-muted-foreground hover:text-destructive disabled:opacity-30 disabled:cursor-not-allowed"
                        title="Restore from this backup (DESTRUCTIVE)"
                      >
                        <RotateCcw className="h-3 w-3" />
                      </button>
                    </li>
                  ))}
              </ul>
            </div>
          ) : (
            <p className="text-muted-foreground">
              Stanza initialized; no backups yet. Click <code>full</code> to create the first one.
            </p>
          )}

          {status.last && !status.running && (
            <div className="pt-2 border-t text-[11px] text-muted-foreground">
              Last backup: {status.last.type}{" "}
              {status.last.ok ? (
                <span className="text-brand">ok</span>
              ) : (
                <span className="text-destructive">
                  failed (exit {status.last.exit_code})
                </span>
              )}
              {status.last.duration && ` · ${status.last.duration}`}
            </div>
          )}

          {restoreStatus.state && !restoreStatus.running && (
            <div className="text-[11px] text-muted-foreground">
              Last restore: {restoreStatus.state.set}{" "}
              {restoreStatus.state.ok ? (
                <span className="text-brand">ok</span>
              ) : (
                <span className="text-destructive">
                  failed at {restoreStatus.state.phase}
                </span>
              )}
              {restoreStatus.state.duration &&
                ` · ${restoreStatus.state.duration}`}
            </div>
          )}
        </div>
      )}

      {/* Restore confirmation dialog */}
      <Dialog
        open={confirmRestore !== null}
        onOpenChange={(o) => {
          if (!o) {
            setConfirmRestore(null);
            setTypedLabel("");
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="text-destructive">
              Restore from {confirmRestore?.label}?
            </DialogTitle>
            <DialogDescription>
              This will STOP the db container, run{" "}
              <code>pgbackrest restore --delta</code> against the live data
              directory, and START the db back up.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2 text-sm">
            <p className="text-destructive font-semibold">
              All data written since this backup will be lost. Expect downtime
              for the duration of the restore.
            </p>
            <p className="text-destructive">
              On failure the db stays stopped.{" "}
              <strong>
                Do NOT start Postgres until you&apos;ve reviewed logs
              </strong>
              {" "}—{" "}
              <code>--delta</code> may leave a partially restored data directory
              that a naive start would crash on.
            </p>
            <p>
              Type the backup label to confirm:{" "}
              <code className="text-foreground">{confirmRestore?.label}</code>
            </p>
          </div>
          <Input
            autoFocus
            value={typedLabel}
            onChange={(e) => setTypedLabel(e.target.value)}
            placeholder={confirmRestore?.label}
            className="font-mono"
          />
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setConfirmRestore(null);
                setTypedLabel("");
              }}
              disabled={startingRestore}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={submitRestore}
              disabled={startingRestore || typedLabel !== confirmRestore?.label}
            >
              {startingRestore && (
                <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
              )}
              Start Restore
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
