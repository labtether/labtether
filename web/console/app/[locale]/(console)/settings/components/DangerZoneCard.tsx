"use client";

import { useEffect, useRef, useState } from "react";
import { AlertTriangle } from "lucide-react";
import { useTranslations } from "next-intl";
import { Card } from "../../../../components/ui/Card";
import { Button } from "../../../../components/ui/Button";
import { Input } from "../../../../components/ui/Input";

export function DangerZoneCard() {
  const t = useTranslations("settings");
  const [showConfirm, setShowConfirm] = useState(false);
  const [confirmInput, setConfirmInput] = useState("");
  const [resetting, setResetting] = useState(false);
  const [resetResult, setResetResult] = useState<{ tables_cleared: number; reset_at: string } | null>(null);
  const [resetError, setResetError] = useState("");
  const resetAbortRef = useRef<AbortController | null>(null);
  const resetSeqRef = useRef(0);

  useEffect(() => {
    return () => {
      resetAbortRef.current?.abort();
      resetAbortRef.current = null;
    };
  }, []);

  const handleReset = async () => {
    setResetting(true);
    setResetError("");
    setResetResult(null);
    const requestID = ++resetSeqRef.current;
    resetAbortRef.current?.abort();
    const controller = new AbortController();
    resetAbortRef.current = controller;
    try {
      const res = await fetch("/api/admin/reset", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ confirm: "RESET" }),
        signal: controller.signal,
      });
      const data = await res.json();
      if (!res.ok) {
        setResetError(data.error || t("dangerZone.resetFailed"));
      } else {
        setResetResult(data);
      }
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") {
        return;
      }
      setResetError(t("dangerZone.resetFailed"));
    } finally {
      if (requestID === resetSeqRef.current) {
        setResetting(false);
        setShowConfirm(false);
        setConfirmInput("");
      }
    }
  };

  return (
    <Card className="border-[var(--bad)]/20 mb-6" style={{ background: "rgba(255,51,85,0.03)" }}>
      <h2 className="flex items-center gap-2"><AlertTriangle className="w-4 h-4 text-[var(--bad)]" strokeWidth={2} /> {t("dangerZone.title")}</h2>
      <p className="text-sm text-[var(--muted)] mb-4">
        {t("dangerZone.description")}
      </p>
      <div className="flex items-center gap-3 pt-4">
        <Button variant="danger" className="bg-[var(--bad-glow)]" onClick={() => setShowConfirm(true)} disabled={resetting}>
          {resetting ? t("dangerZone.wiping") : t("dangerZone.wipeAll")}
        </Button>
      </div>

      {resetResult ? (
        <Card className="bg-[var(--ok-glow)] border-[var(--ok)]/20 text-sm text-[var(--ok)] mt-4">
          {t("dangerZone.resetComplete", {
            count: resetResult.tables_cleared,
            time: new Date(resetResult.reset_at).toLocaleTimeString(),
          })}
        </Card>
      ) : null}

      {resetError ? <Card className="bg-[var(--bad-glow)] border-[var(--bad)]/20 text-sm text-[var(--bad)] mt-4">{resetError}</Card> : null}

      {showConfirm ? (
        <div className="fixed inset-0 flex items-center justify-center z-50 bg-black/60 backdrop-blur-sm" onClick={() => setShowConfirm(false)}>
          <div onClick={(event) => event.stopPropagation()}>
            <Card className="w-96 space-y-4">
              <h3>{t("dangerZone.confirmTitle")}</h3>
              <p>
                {t("dangerZone.confirmDescription")}
              </p>
              <Input
                placeholder={t("dangerZone.confirmPlaceholder")}
                value={confirmInput}
                onChange={(event) => setConfirmInput(event.target.value)}
                autoFocus
                onKeyDown={(event) => {
                  if (event.key === "Enter" && confirmInput === "RESET") {
                    void handleReset();
                  }
                  if (event.key === "Escape") {
                    setShowConfirm(false);
                    setConfirmInput("");
                  }
                }}
              />
              <div className="flex items-center gap-3">
                <Button
                  onClick={() => {
                    setShowConfirm(false);
                    setConfirmInput("");
                  }}
                >
                  {t("dangerZone.cancel")}
                </Button>
                <Button
                  variant="danger"
                  className="bg-[var(--bad-glow)]"
                  disabled={confirmInput !== "RESET" || resetting}
                  onClick={() => void handleReset()}
                >
                  {resetting ? t("dangerZone.wiping") : t("dangerZone.wipeConfirm")}
                </Button>
              </div>
            </Card>
          </div>
        </div>
      ) : null}
    </Card>
  );
}
