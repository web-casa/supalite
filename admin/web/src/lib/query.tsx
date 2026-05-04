"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";

// Single QueryClient per browser session. Defaults tuned for an
// admin panel (long-lived single tab, infrequent fetches):
//   - staleTime 5s : cheap re-fetches but not constantly
//   - retry 1     : transient network blip is forgiven; keep noise low
//   - refetchOnWindowFocus false : SSE/poll handles freshness;
//     refocusing the tab shouldn't trigger a thundering-herd refetch
export function QueryProvider({ children }: { children: React.ReactNode }) {
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            staleTime: 5_000,
            retry: 1,
            refetchOnWindowFocus: false,
          },
          mutations: {
            retry: 0,
          },
        },
      }),
  );
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
