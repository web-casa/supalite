"use client";

import { useState } from "react";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ChevronDown, Loader2, Play, AlertTriangle } from "lucide-react";

type Op = "vacuum" | "analyze" | "reindex";

const opLabels: Record<Op, string> = {
  vacuum: "VACUUM",
  analyze: "ANALYZE",
  reindex: "REINDEX",
};

type Result = {
  ok: boolean;
  duration?: string;
  sql?: string;
  error?: string;
};

export default function DBOpsPage() {
  const [op, setOp] = useState<Op>("analyze");
  const [full, setFull] = useState(false);
  const [target, setTarget] = useState("");
  const [running, setRunning] = useState(false);
  const [result, setResult] = useState<Result | null>(null);

  async function run() {
    setRunning(true);
    setResult(null);
    try {
      const data = await api.post<Result>("/dbops/maintenance", {
        op,
        target: target.trim(),
        full: op === "vacuum" && full,
      });
      setResult(data);
    } catch (err) {
      setResult({
        ok: false,
        error: err instanceof Error ? err.message : "unknown error",
      });
    } finally {
      setRunning(false);
    }
  }

  return (
    <div className="p-6 space-y-4 max-w-3xl">
      <div>
        <h1 className="text-lg font-semibold">DB Ops</h1>
        <p className="text-sm text-muted-foreground">
          Run maintenance operations against the postgres database
        </p>
      </div>

      <Alert>
        <AlertTriangle className="h-4 w-4" />
        <AlertDescription>
          VACUUM FULL and REINDEX take an AccessExclusiveLock. Run during low traffic.
          Requests time out after 10 minutes.
        </AlertDescription>
      </Alert>

      <div className="space-y-3 rounded-lg border p-4">
        <div className="grid grid-cols-[140px_1fr] items-center gap-3">
          <Label>Operation</Label>
          <DropdownMenu>
            <DropdownMenuTrigger className="inline-flex w-fit items-center gap-2 rounded-md border border-input bg-background px-3 py-1.5 text-sm hover:bg-accent">
              {opLabels[op]} <ChevronDown className="h-3.5 w-3.5" />
            </DropdownMenuTrigger>
            <DropdownMenuContent>
              {(Object.keys(opLabels) as Op[]).map((k) => (
                <DropdownMenuItem key={k} onClick={() => setOp(k)}>
                  {opLabels[k]}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>

        {op === "vacuum" && (
          <div className="grid grid-cols-[140px_1fr] items-center gap-3">
            <Label>Mode</Label>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={full}
                onChange={(e) => setFull(e.target.checked)}
              />
              FULL (rewrites table, locks it exclusively)
            </label>
          </div>
        )}

        <div className="grid grid-cols-[140px_1fr] items-start gap-3">
          <Label className="pt-1.5">Target</Label>
          <div>
            <Input
              placeholder="schema.table  (leave empty for whole database)"
              value={target}
              onChange={(e) => setTarget(e.target.value)}
              className="font-mono text-sm"
            />
            <p className="mt-1 text-xs text-muted-foreground">
              Examples: <code>public.users</code>, <code>auth.refresh_tokens</code>.
              {op === "reindex" && " REINDEX requires a specific table."}
            </p>
          </div>
        </div>

        <div className="pt-2">
          <Button onClick={run} disabled={running}>
            {running ? (
              <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
            ) : (
              <Play className="mr-2 h-3.5 w-3.5" />
            )}
            Run
          </Button>
        </div>
      </div>

      {result && (
        <div
          className={`rounded-lg border p-4 ${result.ok ? "" : "border-destructive/50 bg-destructive/5"}`}
        >
          <div className="flex items-center justify-between text-sm">
            <span className={result.ok ? "text-brand" : "text-destructive"}>
              {result.ok ? "Success" : "Failed"}
            </span>
            {result.duration && (
              <span className="font-mono text-xs text-muted-foreground">
                {result.duration}
              </span>
            )}
          </div>
          {result.sql && (
            <pre className="mt-2 overflow-x-auto rounded bg-muted/50 p-2 font-mono text-xs">
              {result.sql}
            </pre>
          )}
          {result.error && (
            <pre className="mt-2 whitespace-pre-wrap font-mono text-xs text-destructive">
              {result.error}
            </pre>
          )}
        </div>
      )}
    </div>
  );
}
