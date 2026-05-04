"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api, errorMessage } from "@/lib/api";
import { toast } from "sonner";
import {
  Rocket,
  Globe,
  Mail,
  Key,
  CheckCircle2,
  Copy,
  ArrowRight,
  ArrowLeft,
  SkipForward,
  Send,
  Loader2,
} from "lucide-react";

interface SetupWizardProps {
  keys: { anon_key: string; api_url: string };
  onComplete: () => void;
}

export function SetupWizard({ keys, onComplete }: SetupWizardProps) {
  const [step, setStep] = useState(0);
  const [serviceKey, setServiceKey] = useState<string | null>(null);

  async function revealServiceKey() {
    try {
      const data = await api.post<{ service_role_key: string }>(
        "/keys/service_role"
      );
      setServiceKey(data.service_role_key);
    } catch {
      /* ignore */
    }
  }
  const [siteUrl, setSiteUrl] = useState("http://localhost:3000");
  const [apiUrl, setApiUrl] = useState("http://localhost:8000");
  const [smtpHost, setSmtpHost] = useState("");
  const [smtpPort, setSmtpPort] = useState("587");
  const [smtpUser, setSmtpUser] = useState("");
  const [smtpPass, setSmtpPass] = useState("");
  const [smtpEmail, setSmtpEmail] = useState("");

  const steps = [
    { icon: Rocket, title: "Welcome" },
    { icon: Globe, title: "URLs" },
    { icon: Mail, title: "Email" },
    { icon: Key, title: "API Keys" },
    { icon: CheckCircle2, title: "Done" },
  ];

  async function saveUrls() {
    await api.post("/config", {
      SITE_URL: siteUrl,
      API_EXTERNAL_URL: apiUrl,
    });
    setStep(2);
  }

  const [smtpTesting, setSmtpTesting] = useState(false);

  async function saveSmtpConfig() {
    await api.post("/config", {
      GOTRUE_SMTP_HOST: smtpHost,
      GOTRUE_SMTP_PORT: smtpPort,
      GOTRUE_SMTP_USER: smtpUser,
      GOTRUE_SMTP_PASS: smtpPass,
      GOTRUE_SMTP_ADMIN_EMAIL: smtpEmail,
      GOTRUE_MAILER_AUTOCONFIRM: smtpHost ? "false" : "true",
    });
  }

  async function saveSmtp() {
    await saveSmtpConfig();
    setStep(3);
  }

  // Save current SMTP fields, then ping the SMTP server with a real
  // probe message. Catches typos / wrong port / bad password BEFORE
  // a real user signup ends up in spam.
  //
  // Note: the /auth/smtp-test endpoint reads .env, so we MUST save
  // first. If the test then fails, the (possibly bad) values are
  // already on disk. The toast makes that explicit so the user knows
  // to re-edit + re-test, not assume their config is untouched.
  async function testSmtp() {
    if (!smtpHost) {
      toast.error("Enter SMTP host first");
      return;
    }
    if (!smtpEmail) {
      toast.error("Enter sender email first (used as test recipient)");
      return;
    }
    setSmtpTesting(true);
    let saved = false;
    try {
      await saveSmtpConfig();
      saved = true;
      await api.post("/auth/smtp-test", { to: smtpEmail });
      toast.success(`Test email sent to ${smtpEmail}`);
    } catch (e) {
      const msg = errorMessage(e, "SMTP test failed");
      if (saved) {
        toast.error(`Saved, but test failed: ${msg}`);
      } else {
        toast.error(`Save failed: ${msg}`);
      }
    } finally {
      setSmtpTesting(false);
    }
  }

  async function finish() {
    await api.post("/config", { SETUP_COMPLETE: "true" });
    // Restart services so config changes take effect
    await api.post("/restart").catch(() => {});
    onComplete();
  }

  function copyToClipboard(text: string) {
    navigator.clipboard.writeText(text);
    toast.success("Copied!");
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-background/95 backdrop-blur-sm">
      <div className="w-full max-w-lg rounded-lg border bg-card p-8">
        {/* Step indicators */}
        <div className="mb-8 flex items-center justify-center gap-2">
          {steps.map((s, i) => (
            <div key={i} className="flex items-center gap-2">
              <div
                className={`flex h-8 w-8 items-center justify-center rounded-full text-xs ${
                  i === step
                    ? "bg-brand text-black"
                    : i < step
                      ? "bg-brand/20 text-brand"
                      : "bg-muted text-muted-foreground"
                }`}
              >
                <s.icon className="h-4 w-4" />
              </div>
              {i < steps.length - 1 && (
                <div
                  className={`h-px w-8 ${
                    i < step ? "bg-brand" : "bg-muted"
                  }`}
                />
              )}
            </div>
          ))}
        </div>

        {/* Step 0: Welcome */}
        {step === 0 && (
          <div className="text-center">
            <h2 className="text-lg font-semibold">
              Welcome to SupaLite
            </h2>
            <p className="mt-2 text-sm text-muted-foreground">
              Let&apos;s configure your instance in a few steps. You can always
              change these settings later.
            </p>
            <Button
              onClick={() => setStep(1)}
              className="mt-6 bg-brand text-black hover:bg-brand/90"
            >
              Get Started <ArrowRight className="ml-2 h-4 w-4" />
            </Button>
          </div>
        )}

        {/* Step 1: URLs */}
        {step === 1 && (
          <div className="space-y-4">
            <h2 className="text-lg font-semibold">Configure URLs</h2>
            <p className="text-sm text-muted-foreground">
              Set the public URLs for your frontend and API gateway.
            </p>
            <div className="space-y-3">
              <div>
                <Label className="text-xs">Frontend URL (SITE_URL)</Label>
                <Input
                  value={siteUrl}
                  onChange={(e) => setSiteUrl(e.target.value)}
                  className="mt-1 bg-background"
                />
              </div>
              <div>
                <Label className="text-xs">API URL (API_EXTERNAL_URL)</Label>
                <Input
                  value={apiUrl}
                  onChange={(e) => setApiUrl(e.target.value)}
                  className="mt-1 bg-background"
                />
              </div>
            </div>
            <div className="flex justify-between pt-2">
              <Button variant="ghost" size="sm" onClick={() => setStep(0)}>
                <ArrowLeft className="mr-2 h-4 w-4" /> Back
              </Button>
              <Button
                onClick={saveUrls}
                size="sm"
                className="bg-brand text-black hover:bg-brand/90"
              >
                Next <ArrowRight className="ml-2 h-4 w-4" />
              </Button>
            </div>
          </div>
        )}

        {/* Step 2: SMTP */}
        {step === 2 && (
          <div className="space-y-4">
            <h2 className="text-lg font-semibold">Email Configuration</h2>
            <p className="text-sm text-muted-foreground">
              Configure SMTP for email verification. Skip to auto-confirm all
              users.
            </p>
            <div className="space-y-3">
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <Label className="text-xs">SMTP Host</Label>
                  <Input
                    value={smtpHost}
                    onChange={(e) => setSmtpHost(e.target.value)}
                    placeholder="smtp.gmail.com"
                    className="mt-1 bg-background"
                  />
                </div>
                <div>
                  <Label className="text-xs">Port</Label>
                  <Input
                    value={smtpPort}
                    onChange={(e) => setSmtpPort(e.target.value)}
                    className="mt-1 bg-background"
                  />
                </div>
              </div>
              <div>
                <Label className="text-xs">Username</Label>
                <Input
                  value={smtpUser}
                  onChange={(e) => setSmtpUser(e.target.value)}
                  className="mt-1 bg-background"
                />
              </div>
              <div>
                <Label className="text-xs">Password</Label>
                <Input
                  type="password"
                  value={smtpPass}
                  onChange={(e) => setSmtpPass(e.target.value)}
                  className="mt-1 bg-background"
                />
              </div>
              <div>
                <Label className="text-xs">Sender Email</Label>
                <Input
                  value={smtpEmail}
                  onChange={(e) => setSmtpEmail(e.target.value)}
                  placeholder="noreply@example.com"
                  className="mt-1 bg-background"
                />
              </div>
            </div>
            <div className="flex justify-between pt-2">
              <Button variant="ghost" size="sm" onClick={() => setStep(1)}>
                <ArrowLeft className="mr-2 h-4 w-4" /> Back
              </Button>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={testSmtp}
                  disabled={smtpTesting || !smtpHost}
                  title="Save these values and send a test email to the sender address"
                >
                  {smtpTesting ? (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  ) : (
                    <Send className="mr-2 h-4 w-4" />
                  )}
                  Send Test
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setStep(3)}
                >
                  <SkipForward className="mr-2 h-4 w-4" /> Skip
                </Button>
                <Button
                  onClick={saveSmtp}
                  size="sm"
                  className="bg-brand text-black hover:bg-brand/90"
                >
                  Next <ArrowRight className="ml-2 h-4 w-4" />
                </Button>
              </div>
            </div>
          </div>
        )}

        {/* Step 3: API Keys */}
        {step === 3 && (
          <div className="space-y-4">
            <h2 className="text-lg font-semibold">Your API Keys</h2>
            <p className="text-sm text-muted-foreground">
              Use these with the supabase-js client library.
            </p>
            <div className="space-y-3">
              <div>
                <Label className="text-xs text-muted-foreground">
                  ANON KEY (safe for frontend)
                </Label>
                <div
                  onClick={() => copyToClipboard(keys.anon_key)}
                  className="mt-1 cursor-pointer rounded-md border bg-background p-3 font-mono text-xs break-all hover:border-brand/50 transition-colors"
                >
                  {keys.anon_key}
                  <Copy className="mt-1 inline h-3 w-3 ml-1 text-muted-foreground" />
                </div>
              </div>
              <div>
                <Label className="text-xs text-destructive">
                  SERVICE ROLE KEY (keep secret!){" "}
                  {!serviceKey && (
                    <button
                      type="button"
                      onClick={revealServiceKey}
                      className="ml-1 underline text-muted-foreground hover:text-foreground"
                    >
                      Reveal
                    </button>
                  )}
                </Label>
                {serviceKey ? (
                  <div
                    onClick={() => copyToClipboard(serviceKey)}
                    className="mt-1 cursor-pointer rounded-md border border-destructive/30 bg-background p-3 font-mono text-xs break-all hover:border-destructive/50 transition-colors"
                  >
                    {serviceKey}
                    <Copy className="mt-1 inline h-3 w-3 ml-1 text-muted-foreground" />
                  </div>
                ) : (
                  <div className="mt-1 rounded-md border border-destructive/30 bg-background p-3 font-mono text-xs text-muted-foreground">
                    Click Reveal to show the service role key
                  </div>
                )}
              </div>
            </div>
            <div className="flex justify-between pt-2">
              <Button variant="ghost" size="sm" onClick={() => setStep(2)}>
                <ArrowLeft className="mr-2 h-4 w-4" /> Back
              </Button>
              <Button
                onClick={() => setStep(4)}
                size="sm"
                className="bg-brand text-black hover:bg-brand/90"
              >
                Next <ArrowRight className="ml-2 h-4 w-4" />
              </Button>
            </div>
          </div>
        )}

        {/* Step 4: Done */}
        {step === 4 && (
          <div className="text-center">
            <CheckCircle2 className="mx-auto h-12 w-12 text-brand" />
            <h2 className="mt-4 text-lg font-semibold">You&apos;re all set!</h2>
            <p className="mt-2 text-sm text-muted-foreground">
              Configure OAuth providers in Settings whenever you&apos;re ready.
            </p>
            <Button
              onClick={finish}
              className="mt-6 bg-brand text-black hover:bg-brand/90"
            >
              Go to Dashboard
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
