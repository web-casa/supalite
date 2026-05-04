"use client";

import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import { LogViewer } from "@/components/log-viewer";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ChevronDown, Pause, Play, Circle } from "lucide-react";

const services = ["db", "rest", "gotrue", "gateway", "admin"];
const tailOptions = [50, 100, 500];

// Ring-buffer cap on client-side lines. Past ~2000 the DOM gets sluggish
// and scrolling starts to hitch; the server keeps streaming regardless.
const MAX_LINES = 2000;

export default function LogsPage() {
  const [service, setService] = useState("gotrue");
  const [tail, setTail] = useState(100);
  const [paused, setPaused] = useState(false);
  const [lines, setLines] = useState<string[]>([]);
  const [connected, setConnected] = useState(false);
  const esRef = useRef<EventSource | null>(null);

  useEffect(() => {
    if (paused) {
      setConnected(false);
      return;
    }
    setLines([]);
    const es = api.sse(`/logs/stream?service=${service}&tail=${tail}`);
    esRef.current = es;
    es.addEventListener("open", () => setConnected(true));
    es.addEventListener("log", (e: MessageEvent) => {
      setLines((prev) => {
        const next = prev.length >= MAX_LINES ? prev.slice(-MAX_LINES + 1) : prev;
        return [...next, e.data];
      });
    });
    es.addEventListener("error", () => setConnected(false));
    return () => {
      es.close();
      esRef.current = null;
    };
  }, [service, tail, paused]);

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold">Logs</h1>
          <p className="text-sm text-muted-foreground">
            Live tail of service container logs
          </p>
        </div>
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <Circle
            className={`h-2 w-2 ${
              connected
                ? "fill-brand text-brand"
                : paused
                  ? "fill-yellow-500 text-yellow-500"
                  : "fill-destructive text-destructive"
            }`}
          />
          {connected ? "live" : paused ? "paused" : "disconnected"}
        </div>
      </div>

      <div className="flex items-center gap-3">
        <DropdownMenu>
          <DropdownMenuTrigger className="inline-flex items-center justify-center gap-2 rounded-md border border-input bg-background px-3 py-1.5 text-sm hover:bg-accent">
            {service} <ChevronDown className="h-3.5 w-3.5" />
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            {services.map((s) => (
              <DropdownMenuItem key={s} onClick={() => setService(s)}>
                {s}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>

        <DropdownMenu>
          <DropdownMenuTrigger className="inline-flex items-center justify-center gap-2 rounded-md border border-input bg-background px-3 py-1.5 text-sm hover:bg-accent">
            {tail} tail <ChevronDown className="h-3.5 w-3.5" />
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            {tailOptions.map((n) => (
              <DropdownMenuItem key={n} onClick={() => setTail(n)}>
                {n} lines
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>

        <Button
          variant="outline"
          size="sm"
          onClick={() => setPaused((p) => !p)}
        >
          {paused ? (
            <>
              <Play className="mr-2 h-3.5 w-3.5" />
              Resume
            </>
          ) : (
            <>
              <Pause className="mr-2 h-3.5 w-3.5" />
              Pause
            </>
          )}
        </Button>
      </div>

      <LogViewer logs={lines.join("\n")} />
    </div>
  );
}
