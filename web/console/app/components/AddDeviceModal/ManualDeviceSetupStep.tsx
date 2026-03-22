"use client";

import { useState } from "react";
import { Button } from "../ui/Button";
import { Input } from "../ui/Input";
import { useToast } from "../../contexts/ToastContext";
import type { AddDeviceAddedEvent } from "./types";

type ManualDeviceSetupStepProps = {
  onBack: () => void;
  onClose: () => void;
  onAdded?: (event: AddDeviceAddedEvent) => void;
};

const PLATFORM_OPTIONS = [
  { value: "", label: "Unknown / Other" },
  { value: "linux", label: "Linux" },
  { value: "windows", label: "Windows" },
  { value: "macos", label: "macOS" },
  { value: "freebsd", label: "FreeBSD" },
];

export function ManualDeviceSetupStep({ onBack, onClose, onAdded }: ManualDeviceSetupStepProps) {
  const { addToast } = useToast();

  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [platform, setPlatform] = useState("");
  const [tags, setTags] = useState("");

  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState("");

  const handleSave = async () => {
    const trimmedName = name.trim();
    const trimmedHost = host.trim();

    if (!trimmedName) {
      const err = "Device name is required.";
      setFormError(err);
      addToast("error", err);
      return;
    }
    if (!trimmedHost) {
      const err = "Host (IP or hostname) is required.";
      setFormError(err);
      addToast("error", err);
      return;
    }

    setFormError("");
    setSaving(true);

    try {
      const tagList = tags
        .split(",")
        .map((t) => t.trim())
        .filter((t) => t.length > 0);

      const body: Record<string, unknown> = {
        name: trimmedName,
        host: trimmedHost,
      };
      if (platform) {
        body.platform = platform;
      }
      if (tagList.length > 0) {
        body.tags = tagList;
      }

      const res = await fetch("/api/assets/manual", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });

      if (!res.ok) {
        const payload = (await res.json().catch(() => ({}))) as { error?: string };
        throw new Error(payload.error || `Failed to create device (${res.status})`);
      }

      const payload = (await res.json()) as { id?: string; name?: string };
      addToast("success", `Device "${trimmedName}" added.`);
      onAdded?.({ source: "manual", focusQuery: trimmedName });
      onClose();
      void payload;
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to create device.";
      setFormError(msg);
      addToast("error", msg);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-3">
      {formError ? <p className="text-xs text-[var(--bad)]">{formError}</p> : null}

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Device Name</label>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="my-server"
        />
      </div>

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Host</label>
        <Input
          value={host}
          onChange={(e) => setHost(e.target.value)}
          placeholder="192.168.1.100 or hostname.local"
        />
        <p className="mt-1 text-xs text-[var(--muted)]">IP address or hostname used to reach this device.</p>
      </div>

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Platform</label>
        <select
          value={platform}
          onChange={(e) => setPlatform(e.target.value)}
          className="w-full rounded-md border border-[var(--line)] bg-[var(--surface)] px-2 py-2 text-xs text-[var(--text)]"
        >
          {PLATFORM_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      </div>

      <div>
        <label className="text-xs font-medium text-[var(--muted)] mb-1 block">Tags</label>
        <Input
          value={tags}
          onChange={(e) => setTags(e.target.value)}
          placeholder="homelab, dmz, storage (comma-separated)"
        />
        <p className="mt-1 text-xs text-[var(--muted)]">Optional comma-separated tags for grouping and filtering.</p>
      </div>

      <div className="flex items-center gap-3 pt-2">
        <Button onClick={onBack}>Back</Button>
        <Button
          variant="primary"
          disabled={saving}
          onClick={() => void handleSave()}
        >
          {saving ? "Adding..." : "Add Device"}
        </Button>
      </div>
    </div>
  );
}
