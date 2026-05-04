"use client";

import { useEffect, useState } from "react";
import { api, errorMessage } from "@/lib/api";
import { useConfigStore } from "@/lib/store";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { toast } from "sonner";
import { Save, RotateCcw, Loader2, Mail } from "lucide-react";

export default function SettingsPage() {
  const { config, fetchConfig, saveConfig } = useConfigStore();
  const [form, setForm] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const [restartDialog, setRestartDialog] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const [smtpTestTo, setSmtpTestTo] = useState("");
  const [smtpTesting, setSmtpTesting] = useState(false);

  useEffect(() => {
    fetchConfig();
  }, [fetchConfig]);

  useEffect(() => {
    if (!config) return;
    const flat: Record<string, string> = {};
    for (const section of Object.values(config)) {
      Object.assign(flat, section);
    }
    setForm(flat);
  }, [config]);

  function setField(key: string, value: string) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }

  function toggle(key: string) {
    setForm((prev) => ({
      ...prev,
      [key]: prev[key] === "true" ? "false" : "true",
    }));
  }

  async function handleSave() {
    setSaving(true);
    try {
      const updated = await saveConfig(form);
      toast.success(`Saved ${updated.length} fields. Restart to apply.`);
      fetchConfig();
    } catch (e) {
      toast.error(errorMessage(e, "Save failed"));
    } finally {
      setSaving(false);
    }
  }

  async function handleSmtpTest() {
    const to = smtpTestTo.trim() || form["GOTRUE_SMTP_ADMIN_EMAIL"] || "";
    if (!to) {
      toast.error("Enter a recipient or set Sender Email first");
      return;
    }
    setSmtpTesting(true);
    try {
      await api.post("/auth/smtp-test", { to });
      toast.success(`Test email sent to ${to}`);
    } catch (e) {
      toast.error(errorMessage(e, "SMTP test failed"));
    } finally {
      setSmtpTesting(false);
    }
  }

  async function handleRestart() {
    setRestarting(true);
    try {
      await api.post("/restart");
      toast.success("Services restarting in background — refresh in a few seconds");
      setRestartDialog(false);
    } catch (e) {
      // The restart may still succeed even if this call fails — the
      // gateway container is restarting and may drop the response.
      toast.message(errorMessage(e, "Restart signal sent"));
    } finally {
      setRestarting(false);
    }
  }

  function Field({
    label,
    field,
    type = "text",
    hint,
  }: {
    label: string;
    field: string;
    type?: string;
    hint?: string;
  }) {
    return (
      <div className="space-y-1.5">
        <Label className="text-xs">{label}</Label>
        <Input
          type={type}
          value={form[field] || ""}
          onChange={(e) => setField(field, e.target.value)}
          className="bg-background"
        />
        {hint && <p className="text-[11px] text-muted-foreground">{hint}</p>}
      </div>
    );
  }

  function Toggle({
    label,
    field,
    invert = false,
  }: {
    label: string;
    field: string;
    invert?: boolean;
  }) {
    const raw = form[field] === "true";
    const checked = invert ? !raw : raw;
    return (
      <div className="flex items-center justify-between py-2">
        <Label className="text-sm">{label}</Label>
        <Switch checked={checked} onCheckedChange={() => toggle(field)} />
      </div>
    );
  }

  function OAuthSection({
    provider,
    prefix,
  }: {
    provider: string;
    prefix: string;
  }) {
    const providerKey = provider.toLowerCase();
    // Apple's client_secret is a short-lived JWT, not a static string;
    // the /oauth-test endpoint doesn't cover it.
    const supportsTest = providerKey === "github" || providerKey === "google";
    const [testing, setTesting] = useState(false);

    async function runTest() {
      setTesting(true);
      try {
        const res = await api.post<{ ok: boolean; message: string }>(
          "/auth/oauth-test",
          { provider: providerKey }
        );
        toast.success(`${provider}: ${res.message}`);
      } catch (e) {
        toast.error(errorMessage(e, "test failed"));
      } finally {
        setTesting(false);
      }
    }

    return (
      <div className="space-y-4">
        <Toggle label={`Enable ${provider}`} field={`${prefix}_ENABLED`} />
        <Field label="Client ID" field={`${prefix}_CLIENT_ID`} />
        <Field
          label="Client Secret"
          field={`${prefix}_SECRET`}
          type="password"
        />
        <Field
          label="Redirect URI"
          field={`${prefix}_REDIRECT_URI`}
          hint="Usually: {API_URL}/auth/v1/callback"
        />
        {supportsTest && (
          <div className="pt-2 border-t">
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={runTest}
                disabled={testing}
              >
                {testing ? (
                  <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
                ) : null}
                Test credentials
              </Button>
              <span className="text-[11px] text-muted-foreground">
                Validates saved values against {provider} — Save first if
                you&apos;ve edited above.
              </span>
            </div>
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold">Settings</h1>
          <p className="text-sm text-muted-foreground">
            Configure auth, email, and OAuth providers
          </p>
        </div>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setRestartDialog(true)}
          >
            <RotateCcw className="mr-2 h-3.5 w-3.5" />
            Restart Services
          </Button>
          <Button
            size="sm"
            onClick={handleSave}
            disabled={saving}
            className="bg-brand text-black hover:bg-brand/90"
          >
            {saving ? (
              <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
            ) : (
              <Save className="mr-2 h-3.5 w-3.5" />
            )}
            Save All
          </Button>
        </div>
      </div>

      <Tabs defaultValue="general">
        <TabsList className="bg-card">
          <TabsTrigger value="general">General</TabsTrigger>
          <TabsTrigger value="smtp">SMTP</TabsTrigger>
          <TabsTrigger value="github">GitHub</TabsTrigger>
          <TabsTrigger value="google">Google</TabsTrigger>
          <TabsTrigger value="apple">Apple</TabsTrigger>
          <TabsTrigger value="backup">Backup</TabsTrigger>
        </TabsList>

        <TabsContent value="general">
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">General Settings</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <Field
                label="Site URL"
                field="SITE_URL"
                hint="Your frontend application URL"
              />
              <Field
                label="API External URL"
                field="API_EXTERNAL_URL"
                hint="Public URL of this API gateway"
              />
              <Field
                label="Additional CORS origins (regex)"
                field="CORS_ALLOWED_ORIGINS_REGEX"
                hint={`Regex alternation of ALL allowed frontend origins. Empty = fall back to SITE_URL. Example for mobile+web+marketing: https://app\\.com|https://admin\\.app\\.com|https://marketing\\.app\\.com`}
              />
              <Field
                label="Additional OAuth redirect URIs"
                field="GOTRUE_URI_ALLOW_LIST"
                hint="Comma-separated list of redirect URIs GoTrue will accept (beyond SITE_URL). Useful for mobile deep links or extra frontends."
              />
              <Toggle
                label="Allow public signup"
                field="GOTRUE_DISABLE_SIGNUP"
                invert
              />
              <Toggle
                label="Allow anonymous access"
                field="GOTRUE_EXTERNAL_ANONYMOUS_USERS_ENABLED"
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="smtp">
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">SMTP / Email</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <Toggle
                label="Auto-confirm new users"
                field="GOTRUE_MAILER_AUTOCONFIRM"
              />
              <Field label="SMTP Host" field="GOTRUE_SMTP_HOST" />
              <Field label="SMTP Port" field="GOTRUE_SMTP_PORT" />
              <Field label="SMTP User" field="GOTRUE_SMTP_USER" />
              <Field
                label="SMTP Password"
                field="GOTRUE_SMTP_PASS"
                type="password"
              />
              <Field label="Sender Email" field="GOTRUE_SMTP_ADMIN_EMAIL" />

              <div className="pt-2 border-t">
                <Label className="text-xs">Send test email to</Label>
                <div className="mt-1.5 flex gap-2">
                  <Input
                    type="email"
                    value={smtpTestTo}
                    onChange={(e) => setSmtpTestTo(e.target.value)}
                    placeholder={form["GOTRUE_SMTP_ADMIN_EMAIL"] || "you@example.com"}
                    className="bg-background"
                  />
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleSmtpTest}
                    disabled={smtpTesting}
                  >
                    {smtpTesting ? (
                      <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
                    ) : (
                      <Mail className="mr-2 h-3.5 w-3.5" />
                    )}
                    Send Test
                  </Button>
                </div>
                <p className="mt-1 text-[11px] text-muted-foreground">
                  Uses the currently saved SMTP settings. Save first, then test.
                </p>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="github">
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">GitHub OAuth</CardTitle>
            </CardHeader>
            <CardContent>
              <OAuthSection
                provider="GitHub"
                prefix="GOTRUE_EXTERNAL_GITHUB"
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="google">
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">Google OAuth</CardTitle>
            </CardHeader>
            <CardContent>
              <OAuthSection
                provider="Google"
                prefix="GOTRUE_EXTERNAL_GOOGLE"
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="apple">
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">Apple OAuth</CardTitle>
            </CardHeader>
            <CardContent>
              <OAuthSection
                provider="Apple"
                prefix="GOTRUE_EXTERNAL_APPLE"
              />
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="backup">
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">S3-Compatible Backup Storage</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <p className="text-xs text-muted-foreground">
                Used by <code>pg_dump</code> backups. Supports AWS S3, Cloudflare R2,
                MinIO, Backblaze B2, and similar. Changes take effect after the
                admin container is restarted.
              </p>
              <Field
                label="Endpoint"
                field="BACKUP_S3_ENDPOINT"
                hint="Leave empty for AWS; e.g. https://minio.example.com for MinIO, https://<account>.r2.cloudflarestorage.com for R2"
              />
              <Field label="Bucket" field="BACKUP_S3_BUCKET" />
              <Field
                label="Region"
                field="BACKUP_S3_REGION"
                hint="Defaults to us-east-1. For R2 use 'auto'."
              />
              <Field label="Access Key" field="BACKUP_S3_ACCESS_KEY" />
              <Field
                label="Secret Key"
                field="BACKUP_S3_SECRET_KEY"
                type="password"
              />
              <Toggle
                label="Path-style addressing (required for MinIO/Ceph)"
                field="BACKUP_S3_PATH_STYLE"
              />
              <Field
                label="Prefix"
                field="BACKUP_S3_PREFIX"
                hint="Objects are stored and listed under this prefix. Defaults to backup/"
              />
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <Dialog open={restartDialog} onOpenChange={setRestartDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Restart Services</DialogTitle>
            <DialogDescription>
              This will restart GoTrue and the API gateway. The API will be
              briefly unavailable.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setRestartDialog(false)}>
              Cancel
            </Button>
            <Button onClick={handleRestart} disabled={restarting}>
              {restarting ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : null}
              Restart
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
