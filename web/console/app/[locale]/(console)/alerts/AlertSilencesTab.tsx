"use client";

import { useState, type FormEvent } from "react";
import { BellOff } from "lucide-react";
import { Badge } from "../../../components/ui/Badge";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { EmptyState } from "../../../components/ui/EmptyState";
import { Input, Select } from "../../../components/ui/Input";
import type { AlertSilence } from "../../../console/models";
import { durationPresets, type DurationPreset } from "./alertsPageTypes";

type AlertSilencesTabProps = {
  silences: AlertSilence[];
  createSilence: (payload: {
    matchers: Record<string, string>;
    starts_at: string;
    ends_at: string;
    reason?: string;
  }) => Promise<void>;
  deleteSilence: (id: string) => Promise<void>;
};

export function AlertSilencesTab({
  silences,
  createSilence,
  deleteSilence,
}: AlertSilencesTabProps) {
  const [showSilenceForm, setShowSilenceForm] = useState(false);
  const [silenceMatchers, setSilenceMatchers] = useState<Array<{ key: string; value: string }>>([{ key: "", value: "" }]);
  const [silenceDuration, setSilenceDuration] = useState<DurationPreset>("1h");
  const [silenceReason, setSilenceReason] = useState("");
  const [silenceSubmitting, setSilenceSubmitting] = useState(false);
  const [silenceError, setSilenceError] = useState<string | null>(null);

  function resetSilenceForm() {
    setSilenceMatchers([{ key: "", value: "" }]);
    setSilenceDuration("1h");
    setSilenceReason("");
    setSilenceError(null);
    setSilenceSubmitting(false);
  }

  function addMatcherRow() {
    setSilenceMatchers((prev) => [...prev, { key: "", value: "" }]);
  }

  function removeMatcherRow(index: number) {
    setSilenceMatchers((prev) => prev.filter((_, idx) => idx !== index));
  }

  function updateMatcher(index: number, field: "key" | "value", value: string) {
    setSilenceMatchers((prev) => prev.map((matcher, idx) => (idx === index ? { ...matcher, [field]: value } : matcher)));
  }

  async function handleCreateSilence(event: FormEvent) {
    event.preventDefault();
    setSilenceError(null);

    const validMatchers = silenceMatchers.filter((matcher) => matcher.key.trim() !== "");
    if (validMatchers.length === 0) {
      setSilenceError("At least one matcher key is required.");
      return;
    }

    const matchersObj: Record<string, string> = {};
    for (const matcher of validMatchers) {
      matchersObj[matcher.key.trim()] = matcher.value.trim();
    }

    const preset = durationPresets.find((duration) => duration.id === silenceDuration);
    const now = new Date();
    const endsAt = new Date(now.getTime() + (preset?.hours ?? 1) * 60 * 60 * 1000);

    setSilenceSubmitting(true);
    try {
      await createSilence({
        matchers: matchersObj,
        starts_at: now.toISOString(),
        ends_at: endsAt.toISOString(),
        reason: silenceReason.trim() || undefined,
      });
      resetSilenceForm();
      setShowSilenceForm(false);
    } catch (err) {
      setSilenceError(err instanceof Error ? err.message : "Could not create mute rule");
    } finally {
      setSilenceSubmitting(false);
    }
  }

  async function handleDeleteSilence(id: string) {
    try {
      await deleteSilence(id);
    } catch {
      // Swallow for now to preserve existing behavior.
    }
  }

  return (
    <Card className="mb-4">
      <div className="flex items-center justify-between mb-3">
        <h2>Muted Alerts</h2>
        <Button
          size="sm"
          onClick={() => {
            if (showSilenceForm) {
              resetSilenceForm();
              setShowSilenceForm(false);
            } else {
              setShowSilenceForm(true);
            }
          }}
        >
          {showSilenceForm ? "Cancel" : "New Mute Rule"}
        </Button>
      </div>

      {showSilenceForm ? (
        <form className="space-y-3 py-3 border-t border-[var(--line)]" onSubmit={(event) => void handleCreateSilence(event)}>
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-xs text-[var(--muted)]">Matchers</span>
              <Button type="button" variant="ghost" size="sm" onClick={addMatcherRow}>+ Row</Button>
            </div>
            {silenceMatchers.map((matcher, idx) => (
              <div key={idx} className="flex items-center gap-2">
                <Input
                  placeholder="Key (e.g. severity)"
                  value={matcher.key}
                  onChange={(event) => updateMatcher(idx, "key", event.target.value)}
                />
                <Input
                  placeholder="Value (e.g. low)"
                  value={matcher.value}
                  onChange={(event) => updateMatcher(idx, "value", event.target.value)}
                />
                {silenceMatchers.length > 1 ? (
                  <Button type="button" variant="ghost" size="sm" onClick={() => removeMatcherRow(idx)}>
                    &times;
                  </Button>
                ) : <span />}
              </div>
            ))}
          </div>
          <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
            Duration
            <Select
              value={silenceDuration}
              onChange={(event) => setSilenceDuration(event.target.value as DurationPreset)}
            >
              {durationPresets.map((preset) => (
                <option key={preset.id} value={preset.id}>{preset.label}</option>
              ))}
            </Select>
          </label>
          <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
            Reason
            <Input
              value={silenceReason}
              onChange={(event) => setSilenceReason(event.target.value)}
              placeholder="Maintenance, testing, etc."
            />
          </label>
          <Button type="submit" variant="primary" disabled={silenceSubmitting}>
            {silenceSubmitting ? "Creating..." : "Create Silence"}
          </Button>
          {silenceError ? <p className="text-xs text-[var(--bad)]">{silenceError}</p> : null}
        </form>
      ) : null}

      {silences.length === 0 && !showSilenceForm ? (
        <EmptyState
          icon={BellOff}
          title="No active mutes"
          description="You can mute specific alerts here during downtime or maintenance."
        />
      ) : silences.length > 0 ? (
        <ul className="divide-y divide-[var(--line)]">
          {silences.map((silence) => {
            const active = new Date(silence.ends_at) > new Date();
            return (
              <li key={silence.id} className="flex items-center justify-between gap-3 py-2.5">
                <div>
                  <span className="text-sm font-medium text-[var(--text)]">{silence.reason ?? "Silence"}</span>
                  <code>{Object.entries(silence.matchers).map(([key, value]) => `${key}=${value}`).join(", ")}</code>
                </div>
                <div className="flex items-center gap-2">
                  <Badge status={active ? "active" : "expired"} />
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => void handleDeleteSilence(silence.id)}
                    title="Delete silence"
                  >
                    Delete
                  </Button>
                </div>
              </li>
            );
          })}
        </ul>
      ) : null}
    </Card>
  );
}
