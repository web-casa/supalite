"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, errorMessage } from "@/lib/api";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Loader2, RefreshCw, X } from "lucide-react";

interface Session {
  pid: number;
  username: string;
  application_name: string;
  client_addr: string | null;
  database_name: string;
  state: string;
  wait_event_type: string | null;
  wait_event: string | null;
  backend_start: string;
  query_start: string | null;
  query: string;
  backend_type: string;
}

function stateBadge(state: string) {
  if (state === "active") return "default" as const;
  if (state === "idle") return "secondary" as const;
  if (state.startsWith("idle in transaction")) return "destructive" as const;
  return "outline" as const;
}

function shorten(q: string, n = 80) {
  const trimmed = q.replace(/\s+/g, " ").trim();
  return trimmed.length > n ? trimmed.slice(0, n) + "…" : trimmed;
}

export default function SessionsPage() {
  const [includeSystem, setIncludeSystem] = useState(false);
  const [selected, setSelected] = useState<Session | null>(null);
  const queryClient = useQueryClient();

  // refetchInterval=5000 + react-query's default refetchIntervalInBackground=false
  // means polling pauses while the tab is hidden — same behavior as the
  // previous useVisiblePoll hook, but via react-query's built-in machinery.
  const sessionsQuery = useQuery({
    queryKey: ["sessions", includeSystem],
    queryFn: () =>
      api.get<{ sessions: Session[] }>(`/sessions?system=${includeSystem}`),
    refetchInterval: 5000,
  });
  const sessions = sessionsQuery.data?.sessions || [];

  const terminateMutation = useMutation({
    mutationFn: (pid: number) =>
      api.post("/sessions/terminate", { pid }),
    onSuccess: () => {
      setSelected(null);
      queryClient.invalidateQueries({ queryKey: ["sessions"] });
    },
    onError: (e) => toast.error(errorMessage(e, "terminate failed")),
  });

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold">Sessions</h1>
          <p className="text-sm text-muted-foreground">
            Active backends from <code>pg_stat_activity</code>. Auto-refreshes every 5s.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <label className="flex items-center gap-1.5 text-sm text-muted-foreground">
            <input
              type="checkbox"
              checked={includeSystem}
              onChange={(e) => setIncludeSystem(e.target.checked)}
            />
            include system
          </label>
          <Button
            variant="outline"
            size="sm"
            onClick={() => sessionsQuery.refetch()}
            disabled={sessionsQuery.isFetching}
          >
            {sessionsQuery.isFetching ? (
              <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
            ) : (
              <RefreshCw className="mr-2 h-3.5 w-3.5" />
            )}
            Refresh
          </Button>
        </div>
      </div>

      <div className="rounded-lg border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-20">PID</TableHead>
              <TableHead>User</TableHead>
              <TableHead>DB</TableHead>
              <TableHead>State</TableHead>
              <TableHead>App / Client</TableHead>
              <TableHead>Wait</TableHead>
              <TableHead>Query</TableHead>
              <TableHead className="w-24"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {sessions.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={8}
                  className="text-center text-sm text-muted-foreground"
                >
                  {sessionsQuery.isLoading ? "Loading…" : "No sessions"}
                </TableCell>
              </TableRow>
            )}
            {sessions.map((s) => (
              <TableRow key={s.pid}>
                <TableCell className="font-mono">{s.pid}</TableCell>
                <TableCell>{s.username || "—"}</TableCell>
                <TableCell>{s.database_name || "—"}</TableCell>
                <TableCell>
                  <Badge variant={stateBadge(s.state)}>{s.state || s.backend_type}</Badge>
                </TableCell>
                <TableCell className="text-xs">
                  <div>{s.application_name || "—"}</div>
                  <div className="text-muted-foreground">{s.client_addr || "local"}</div>
                </TableCell>
                <TableCell className="text-xs">
                  {s.wait_event_type
                    ? `${s.wait_event_type}: ${s.wait_event}`
                    : "—"}
                </TableCell>
                <TableCell
                  className="font-mono text-xs max-w-md truncate"
                  title={s.query}
                >
                  {shorten(s.query) || "—"}
                </TableCell>
                <TableCell>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setSelected(s)}
                    title="Terminate this backend"
                  >
                    <X className="h-3.5 w-3.5" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      <Dialog open={selected !== null} onOpenChange={(o) => !o && setSelected(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Terminate backend {selected?.pid}?</DialogTitle>
            <DialogDescription>
              This sends <code>pg_terminate_backend({selected?.pid})</code> and will
              roll back any in-flight transaction. The client connection will see
              a fatal error.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setSelected(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => selected && terminateMutation.mutate(selected.pid)}
              disabled={terminateMutation.isPending}
            >
              {terminateMutation.isPending && (
                <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
              )}
              Terminate
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
