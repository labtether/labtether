"use client";

import { useEffect, useState } from "react";
import { Copy, Database, Eye, EyeOff } from "lucide-react";

import { Card } from "../../../../components/ui/Card";
import { Button } from "../../../../components/ui/Button";
import { SkeletonRow } from "../../../../components/ui/Skeleton";

type ManagedDatabaseInfo = {
  managed: boolean;
  engine: string;
  host?: string;
  database?: string;
  username?: string;
  password_available: boolean;
  password_hint?: string;
};

type ManagedDatabaseReveal = ManagedDatabaseInfo & {
  password: string;
};

type ManagedDatabaseCardProps = {
  copyToClipboard: (text: string, label: string, toastMessage?: string) => void;
  copied: string;
};

export function ManagedDatabaseCard({ copyToClipboard, copied }: ManagedDatabaseCardProps) {
  const [info, setInfo] = useState<ManagedDatabaseInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [revealedPassword, setRevealedPassword] = useState("");
  const [revealing, setRevealing] = useState(false);

  useEffect(() => {
    setLoading(true);
    fetch("/api/settings/managed-database", { cache: "no-store" })
      .then(async (response) => {
        const payload = (await safeJSON(response)) as ManagedDatabaseInfo | { error?: string } | null;
        if (!response.ok) {
          throw new Error((payload && "error" in payload && payload.error) || `HTTP ${response.status}`);
        }
        setInfo(payload as ManagedDatabaseInfo);
        setError("");
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : "Failed to load managed database settings");
      })
      .finally(() => {
        setLoading(false);
      });
  }, []);

  const handleReveal = async () => {
    if (revealedPassword) {
      setRevealedPassword("");
      return;
    }
    setRevealing(true);
    try {
      const response = await fetch("/api/settings/managed-database/reveal", {
        method: "POST",
        cache: "no-store",
      });
      const payload = (await safeJSON(response)) as ManagedDatabaseReveal | { error?: string } | null;
      if (!response.ok) {
        throw new Error((payload && "error" in payload && payload.error) || `HTTP ${response.status}`);
      }
      if (!payload || !("password" in payload) || typeof payload.password !== "string") {
        throw new Error("Reveal response was missing the password");
      }
      setRevealedPassword(payload.password);
      setInfo(payload);
      setError("");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to reveal managed database password");
    } finally {
      setRevealing(false);
    }
  };

  if (!loading && !info && error) {
    return null;
  }

  return (
    <Card className="mb-6">
      <p className="mb-3 text-xs font-mono uppercase tracking-wider text-[var(--muted)]">// Managed Database</p>
      {loading ? (
        <div className="space-y-1">
          <SkeletonRow />
          <SkeletonRow />
        </div>
      ) : null}
      {error ? <p className="text-xs text-[var(--bad)]">{error}</p> : null}

      {!loading && info ? (
        <div className="space-y-4">
          <div className="flex items-start gap-2 rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2.5">
            <Database size={14} className="mt-0.5 shrink-0 text-[var(--accent)]" />
            <p className="text-xs text-[var(--muted)]">
              {info.managed && info.password_available
                ? "LabTether is managing the split-Postgres password for this install. Reveal it only when you need to connect directly."
                : "This install does not have a LabTether-managed Postgres password available to reveal."}
            </p>
          </div>

          <div className="divide-y divide-[var(--line)]">
            <DatabaseField label="Engine" value={info.engine || "postgres"} />
            <DatabaseField label="Host" value={info.host || "postgres"} />
            <DatabaseField label="Database" value={info.database || "labtether"} />
            <DatabaseField label="Username" value={info.username || "labtether"} />
            <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
              <div className="flex flex-col gap-0.5">
                <span className="text-sm font-medium text-[var(--text)]">Password</span>
                <span className="text-xs text-[var(--muted)]">
                  {revealedPassword ? "Currently revealed in this browser session only." : "Hidden by default."}
                </span>
              </div>
              <div className="flex min-w-0 flex-wrap items-center gap-2 sm:justify-end">
                <code className="block max-w-full break-all text-xs sm:text-sm">
                  {revealedPassword || info.password_hint || (info.password_available ? "Available on request" : "Unavailable")}
                </code>
                {revealedPassword ? (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() =>
                      copyToClipboard(revealedPassword, "managed-db-password", "Database password copied to clipboard")
                    }
                  >
                    <Copy size={13} className="shrink-0" />
                    {copied === "managed-db-password" ? "Copied" : "Copy"}
                  </Button>
                ) : null}
                <Button
                  variant="ghost"
                  size="sm"
                  loading={revealing}
                  disabled={!info.password_available}
                  onClick={() => void handleReveal()}
                >
                  {revealedPassword ? <EyeOff size={13} className="shrink-0" /> : <Eye size={13} className="shrink-0" />}
                  {revealedPassword ? "Hide" : "Show Password"}
                </Button>
              </div>
            </div>
          </div>
        </div>
      ) : null}
    </Card>
  );
}

function DatabaseField({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
      <div className="flex flex-col gap-0.5">
        <span className="text-sm font-medium text-[var(--text)]">{label}</span>
      </div>
      <div className="flex items-center gap-2 sm:justify-end">
        <code className="block max-w-full break-all text-xs sm:text-sm">{value}</code>
      </div>
    </div>
  );
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
