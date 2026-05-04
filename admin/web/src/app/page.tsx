"use client";

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, errorMessage } from "@/lib/api";
import { useConfigStore } from "@/lib/store";
import { SetupWizard } from "@/components/setup-wizard";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { toast } from "sonner";
import { Copy, Activity, Key } from "lucide-react";

interface ServiceInfo {
  name: string;
  state: string;
  status: string;
}

interface KeysData {
  anon_key: string;
  api_url: string;
}

export default function DashboardPage() {
  const { config, fetchConfig } = useConfigStore();
  const [showWizard, setShowWizard] = useState(false);
  const [serviceKey, setServiceKey] = useState<string | null>(null);

  const servicesQuery = useQuery({
    queryKey: ["dashboard", "status"],
    queryFn: () => api.get<{ services: ServiceInfo[] }>("/status"),
  });
  const services = servicesQuery.data?.services || [];

  const keysQuery = useQuery({
    queryKey: ["dashboard", "keys"],
    queryFn: () => api.get<KeysData>("/keys"),
  });
  const keys = keysQuery.data ?? null;

  async function revealServiceKey() {
    try {
      const data = await api.post<{ service_role_key: string }>(
        "/keys/service_role"
      );
      setServiceKey(data.service_role_key);
    } catch (e) {
      toast.error(errorMessage(e, "Failed to fetch key"));
    }
  }

  function hideServiceKey() {
    setServiceKey(null);
  }

  useEffect(() => {
    fetchConfig();
  }, [fetchConfig]);

  useEffect(() => {
    if (config && config.meta?.SETUP_COMPLETE !== "true") {
      setShowWizard(true);
    }
  }, [config]);

  function copy(text: string) {
    navigator.clipboard.writeText(text);
    toast.success("Copied!");
  }

  const snippets = {
    javascript: `import { createClient } from '@supabase/supabase-js'

const supabase = createClient(
  '${keys?.api_url || "http://localhost:8000"}',
  '${keys?.anon_key || "<ANON_KEY>"}'
)

const { data } = await supabase.from('your_table').select('*')`,
    python: `from supabase import create_client

supabase = create_client(
    "${keys?.api_url || "http://localhost:8000"}",
    "${keys?.anon_key || "<ANON_KEY>"}"
)

data = supabase.table("your_table").select("*").execute()`,
    curl: `curl '${keys?.api_url || "http://localhost:8000"}/rest/v1/your_table?select=*' \\
  -H 'apikey: ${keys?.anon_key || "<ANON_KEY>"}' \\
  -H 'Authorization: Bearer ${keys?.anon_key || "<ANON_KEY>"}'`,
  };

  return (
    <div className="p-6 space-y-6">
      {showWizard && keys && (
        <SetupWizard
          keys={keys}
          onComplete={() => {
            setShowWizard(false);
            fetchConfig();
            keysQuery.refetch();
          }}
        />
      )}

      <div>
        <h1 className="text-lg font-semibold">Dashboard</h1>
        <p className="text-sm text-muted-foreground">Overview of your SupaLite instance</p>
      </div>

      {/* Service Status */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-sm font-medium">
            <Activity className="h-4 w-4" />
            Services
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap gap-3">
            {services.map((svc) => (
              <div
                key={svc.name}
                className="flex items-center gap-2 rounded-md border bg-background px-3 py-2"
              >
                <span
                  className={`h-2 w-2 rounded-full ${
                    svc.state === "running" ? "bg-brand" : "bg-destructive"
                  }`}
                />
                <span className="text-sm">{svc.name}</span>
                <Badge variant="secondary" className="text-xs">
                  {svc.state}
                </Badge>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* API Keys */}
      {keys && (
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-sm font-medium">
              <Key className="h-4 w-4" />
              API Keys
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <div>
              <p className="text-xs text-muted-foreground mb-1">API URL</p>
              <div
                onClick={() => copy(keys.api_url)}
                className="cursor-pointer rounded border bg-background p-2.5 font-mono text-xs hover:border-brand/50 transition-colors"
              >
                {keys.api_url}
                <Copy className="inline ml-2 h-3 w-3 text-muted-foreground" />
              </div>
            </div>
            <div>
              <p className="text-xs text-muted-foreground mb-1">ANON KEY</p>
              <div
                onClick={() => copy(keys.anon_key)}
                className="cursor-pointer rounded border bg-background p-2.5 font-mono text-xs break-all hover:border-brand/50 transition-colors"
              >
                {keys.anon_key}
                <Copy className="inline ml-2 h-3 w-3 text-muted-foreground" />
              </div>
            </div>
            <div>
              <p className="text-xs text-destructive mb-1">
                SERVICE ROLE KEY{" "}
                <button
                  onClick={serviceKey ? hideServiceKey : revealServiceKey}
                  className="ml-2 underline text-muted-foreground hover:text-foreground"
                >
                  {serviceKey ? "Hide" : "Reveal"}
                </button>
              </p>
              {serviceKey ? (
                <div
                  onClick={() => copy(serviceKey)}
                  className="cursor-pointer rounded border border-destructive/20 bg-background p-2.5 font-mono text-xs break-all hover:border-destructive/40 transition-colors"
                >
                  {serviceKey}
                  <Copy className="inline ml-2 h-3 w-3 text-muted-foreground" />
                </div>
              ) : (
                <div className="rounded border border-destructive/20 bg-background p-2.5 font-mono text-xs text-muted-foreground">
                  Click &quot;Reveal&quot; to load and view the service role key
                </div>
              )}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Quick Start */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium">Quick Start</CardTitle>
        </CardHeader>
        <CardContent>
          <Tabs defaultValue="javascript">
            <TabsList className="bg-background">
              <TabsTrigger value="javascript">JavaScript</TabsTrigger>
              <TabsTrigger value="python">Python</TabsTrigger>
              <TabsTrigger value="curl">curl</TabsTrigger>
            </TabsList>
            {Object.entries(snippets).map(([lang, code]) => (
              <TabsContent key={lang} value={lang}>
                <div className="relative">
                  <pre className="rounded-md border bg-[hsl(0,0%,6%)] p-4 text-xs font-mono overflow-x-auto">
                    {code}
                  </pre>
                  <button
                    onClick={() => copy(code)}
                    className="absolute top-2 right-2 rounded p-1 hover:bg-muted"
                  >
                    <Copy className="h-3.5 w-3.5 text-muted-foreground" />
                  </button>
                </div>
              </TabsContent>
            ))}
          </Tabs>
        </CardContent>
      </Card>
    </div>
  );
}
