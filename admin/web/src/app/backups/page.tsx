"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { api, errorMessage } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { PgbackrestCard } from "@/components/pgbackrest-card";
import { toast } from "sonner";
import {
  Download,
  Loader2,
  Play,
  RefreshCw,
  RotateCcw,
  Trash2,
  AlertTriangle,
} from "lucide-react";

interface Backup {
  name: string;
  size: number;
  last_modified: string;
}

function formatBytes(n: number) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KiB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MiB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GiB`;
}

export default function BackupsPage() {
  const [confirmDelete, setConfirmDelete] = useState<Backup | null>(null);
  const [confirmRestore, setConfirmRestore] = useState<Backup | null>(null);
  const [restoreTyped, setRestoreTyped] = useState("");
  const queryClient = useQueryClient();

  // List of backups. Surface "backup not configured" specially via the
  // hint banner instead of a generic error toast.
  const listQuery = useQuery({
    queryKey: ["backups"],
    queryFn: () => api.get<{ backups: Backup[] }>("/backup/list"),
  });
  const backups = listQuery.data?.backups || [];
  const configErr =
    listQuery.error && errorMessage(listQuery.error, "").includes("backup not configured")
      ? errorMessage(listQuery.error, "list failed")
      : null;

  const runMutation = useMutation({
    mutationFn: () => api.post<{ name: string; duration: string }>("/backup/run"),
    onSuccess: (data) => {
      toast.success(`Backup ${data.name} created in ${data.duration}`);
      queryClient.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: (e) => toast.error(errorMessage(e, "backup failed")),
  });

  const restoreMutation = useMutation({
    mutationFn: (name: string) =>
      api.post<{ duration: string; notices?: string }>("/backup/restore", {
        name,
        clean: true,
      }),
    onSuccess: (data, name) => {
      toast.success(`Restored ${name} in ${data.duration}`);
      if (data.notices && data.notices.trim()) {
        toast.message("pg_restore notices (see console for full output)");
        console.log(`[restore ${name}]\n${data.notices}`);
      }
      setConfirmRestore(null);
      setRestoreTyped("");
    },
    onError: (e) => toast.error(errorMessage(e, "restore failed")),
  });

  const deleteMutation = useMutation({
    mutationFn: (name: string) => api.post("/backup/delete", { name }),
    onSuccess: (_d, name) => {
      toast.success(`Deleted ${name}`);
      setConfirmDelete(null);
      queryClient.invalidateQueries({ queryKey: ["backups"] });
    },
    onError: (e) => toast.error(errorMessage(e, "delete failed")),
  });

  async function downloadBackup(name: string) {
    try {
      const data = await api.get<{ url: string }>(
        `/backup/download?name=${encodeURIComponent(name)}`
      );
      // noopener,noreferrer: don't leak the admin window via opener,
      // and don't send the admin URL in Referer to the S3 endpoint.
      window.open(data.url, "_blank", "noopener,noreferrer");
    } catch (e) {
      toast.error(errorMessage(e, "download failed"));
    }
  }

  // Legacy stand-ins so the JSX below didn't have to change.
  const loading = listQuery.isFetching;
  const running = runMutation.isPending;
  const restoring = restoreMutation.isPending;
  const refresh = () => queryClient.invalidateQueries({ queryKey: ["backups"] });
  const runBackup = () => runMutation.mutate();
  const doRestore = () => {
    if (confirmRestore) restoreMutation.mutate(confirmRestore.name);
  };

  async function deleteBackup() {
    if (!confirmDelete) return;
    try {
      deleteMutation.mutate(confirmDelete.name);
    } catch (e) {
      toast.error(errorMessage(e, "delete failed"));
    }
  }

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold">Backups</h1>
          <p className="text-sm text-muted-foreground">
            Logical backups via <code>pg_dump -Fc</code>, stored on S3-compatible object storage
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={refresh} disabled={loading}>
            {loading ? (
              <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
            ) : (
              <RefreshCw className="mr-2 h-3.5 w-3.5" />
            )}
            Refresh
          </Button>
          <Button
            size="sm"
            onClick={runBackup}
            disabled={running || configErr !== null}
            className="bg-brand text-black hover:bg-brand/90"
          >
            {running ? (
              <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
            ) : (
              <Play className="mr-2 h-3.5 w-3.5" />
            )}
            Run Backup
          </Button>
        </div>
      </div>

      {configErr && (
        <Alert>
          <AlertTriangle className="h-4 w-4" />
          <AlertDescription>
            Backup storage not configured. Set S3 credentials in{" "}
            <Link href="/admin/settings/" className="underline">
              Settings → Backup
            </Link>
            , then save and restart the admin container.
          </AlertDescription>
        </Alert>
      )}

      <div className="rounded-lg border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead className="w-32">Size</TableHead>
              <TableHead className="w-56">Last Modified</TableHead>
              <TableHead className="w-32"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {backups.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={4}
                  className="text-center text-sm text-muted-foreground"
                >
                  {loading ? "Loading…" : configErr ? "—" : "No backups yet"}
                </TableCell>
              </TableRow>
            )}
            {backups.map((b) => (
              <TableRow key={b.name}>
                <TableCell className="font-mono text-xs">{b.name}</TableCell>
                <TableCell className="font-mono text-xs">
                  {formatBytes(b.size)}
                </TableCell>
                <TableCell className="text-xs text-muted-foreground">
                  {new Date(b.last_modified).toLocaleString()}
                </TableCell>
                <TableCell>
                  <div className="flex gap-1">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => downloadBackup(b.name)}
                      title="Download"
                    >
                      <Download className="h-3.5 w-3.5" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => {
                        setConfirmRestore(b);
                        setRestoreTyped("");
                      }}
                      title="Restore (DESTRUCTIVE)"
                    >
                      <RotateCcw className="h-3.5 w-3.5" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setConfirmDelete(b)}
                      title="Delete"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      <PgbackrestCard />

      <Dialog
        open={confirmRestore !== null}
        onOpenChange={(o) => {
          if (!o) {
            setConfirmRestore(null);
            setRestoreTyped("");
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="text-destructive">
              Restore from {confirmRestore?.name}?
            </DialogTitle>
            <DialogDescription>
              Runs <code>pg_restore --clean --if-exists</code> against the live
              database. Objects covered by the backup will be dropped and recreated.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2 text-sm">
            <p className="text-destructive font-semibold">
              Current data will be lost. This cannot be undone.
            </p>
            <p>
              Type the backup name to confirm:{" "}
              <code className="text-foreground">{confirmRestore?.name}</code>
            </p>
          </div>
          <Input
            autoFocus
            value={restoreTyped}
            onChange={(e) => setRestoreTyped(e.target.value)}
            placeholder={confirmRestore?.name}
            className="font-mono"
          />
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setConfirmRestore(null);
                setRestoreTyped("");
              }}
              disabled={restoring}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={doRestore}
              disabled={restoring || restoreTyped !== confirmRestore?.name}
            >
              {restoring && <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />}
              Restore
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={confirmDelete !== null}
        onOpenChange={(o) => !o && setConfirmDelete(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete {confirmDelete?.name}?</DialogTitle>
            <DialogDescription>
              This permanently removes the object from S3. Cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDelete(null)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={deleteBackup}>
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
