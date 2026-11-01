"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { Input } from "../../../../components/ui/Input";
import { Message, Row, Section, SettingsDialog, Sheet } from "./SettingsKit";

const CONFIRM_WORD = "RESET";

/** Reset hub: the quiet last part of General. The dialog asks for RESET before the button works. */
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

  const closeConfirm = () => {
    if (resetting) return;
    setShowConfirm(false);
    setConfirmInput("");
  };

  const handleReset = async () => {
    if (confirmInput !== CONFIRM_WORD) return;
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
        body: JSON.stringify({ confirm: CONFIRM_WORD }),
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
    <Section title={t("dangerZone.title")} id="reset">
      <Sheet>
        <Row label={t("dangerZone.row")} hint={t("dangerZone.description")} two>
          <Button variant="danger" size="sm" onClick={() => setShowConfirm(true)} disabled={resetting}>
            {resetting ? t("dangerZone.wiping") : t("dangerZone.wipeAll")}
          </Button>
        </Row>
        {resetResult ? (
          <Message>
            {t("dangerZone.resetComplete", {
              count: resetResult.tables_cleared,
              time: new Date(resetResult.reset_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hourCycle: "h23" }),
            })}
          </Message>
        ) : null}
        {resetError ? <Message tone="bad">{resetError}</Message> : null}
      </Sheet>

      {showConfirm ? (
        <SettingsDialog labelledBy="st-reset-title" onClose={closeConfirm}>
          <form
            className="st-dlg-form"
            onSubmit={(event) => {
              event.preventDefault();
              void handleReset();
            }}
          >
            <h3 id="st-reset-title">{t("dangerZone.confirmTitle")}</h3>
            <p>{t("dangerZone.confirmDescription")}</p>
            <div className="st-field">
              <label htmlFor="st-reset-word">{t("dangerZone.confirmLabel", { word: CONFIRM_WORD })}</label>
              <Input
                id="st-reset-word"
                placeholder={CONFIRM_WORD}
                value={confirmInput}
                onChange={(event) => setConfirmInput(event.target.value)}
                autoComplete="off"
                spellCheck={false}
                autoFocus
              />
            </div>
            <div className="st-dlg-acts">
              <Button type="button" onClick={closeConfirm} disabled={resetting}>
                {t("dangerZone.cancel")}
              </Button>
              <Button type="submit" variant="danger" className="st-danger-fill" disabled={confirmInput !== CONFIRM_WORD || resetting}>
                {resetting ? t("dangerZone.wiping") : t("dangerZone.wipeConfirm")}
              </Button>
            </div>
          </form>
        </SettingsDialog>
      ) : null}
    </Section>
  );
}
