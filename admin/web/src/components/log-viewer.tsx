"use client";

import { useEffect, useRef } from "react";
import { ScrollArea } from "@/components/ui/scroll-area";

interface LogViewerProps {
  logs: string;
}

export function LogViewer({ logs }: LogViewerProps) {
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs]);

  return (
    <ScrollArea className="h-[600px] rounded-md border bg-[hsl(0,0%,6%)]">
      <pre className="p-4 text-xs leading-5 font-mono text-muted-foreground whitespace-pre-wrap break-all">
        {logs || "No logs available"}
      </pre>
      <div ref={bottomRef} />
    </ScrollArea>
  );
}
