"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { CopyButton, Facts, Message, Section, Sheet } from "./SettingsKit";

type ManagedDatabaseInfo = {
  managed: boolean;
  engine: string;
  host?: string;
  database?: string;
  username?: string;
  password_available: boolean;
  password_hint?: string;
};

const LOAD_FAILED = "load-failed";

type ManagedDatabaseReveal = ManagedDatabaseInfo & {
  password: string;
};

/** The hub's Postgres connection. The password stays hidden until Show, and only in this browser tab. */
export function ManagedDatabaseCard() {
  const t = useTranslations("settings");
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
          throw new Error((payload && "error" in payload && payload.error) || "");
        }
        setInfo(payload as ManagedDatabaseInfo);
        setError("");
      })
      .catch((err: unknown) => {
        // An empty message shows the translated "could not load" line.
        setError(err instanceof Error && err.message ? err.message : LOAD_FAILED);
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
      const response = await fetch("/api/settings/managed-database/reveal", { method: "POST", cache: "no-store" });
      const payload = (await safeJSON(response)) as ManagedDatabaseReveal | { error?: string } | null;
      if (!response.ok) {
        throw new Error((payload && "error" in payload && payload.error) || t("database.revealFailed"));
      }
      if (!payload || !("password" in payload) || typeof payload.password !== "string") {
        throw new Error(t("database.revealFailed"));
      }
      setRevealedPassword(payload.password);
      setInfo(payload);
      setError("");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("database.revealFailed"));
    } finally {
      setRevealing(false);
    }
  };

  if (!loading && !info && error) return null;

  const password = revealedPassword
    ? <code className="st-mono">{revealedPassword}</code>
    : <span className="st-g">{info?.password_hint || (info?.password_available ? t("database.hidden") : t("database.unavailable"))}</span>;

  return (
    <Section
      title={t("database.title")}
      id="database"
      action={info?.password_available ? (
        <Button size="sm" variant="ghost" loading={revealing} onClick={() => void handleReveal()}>
          {revealedPassword ? t("database.hide") : t("database.show")}
        </Button>
      ) : null}
    >
      <Sheet className={loading ? "st-loading" : ""}>
        {info ? (
          <Facts
            items={[
              [t("database.engine"), info.engine || "postgres"],
              [t("database.host"), info.host || "postgres"],
              [t("database.database"), info.database || "labtether"],
              [t("database.username"), info.username || "labtether"],
              [t("database.password"), (
                <>
                  {password}
                  {revealedPassword ? <CopyButton text={revealedPassword} label={t("database.copy")} /> : null}
                </>
              )],
              !info.managed ? [t("database.managedBy"), <span key="m" className="st-g">{t("database.notManaged")}</span>] : false,
            ]}
          />
        ) : null}
        {error ? <Message tone="bad">{error === LOAD_FAILED ? t("database.loadFailed") : error}</Message> : null}
      </Sheet>
    </Section>
  );
}

async function safeJSON(response: Response): Promise<unknown | null> {
  try {
    return await response.json();
  } catch {
    return null;
  }
}
