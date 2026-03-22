"use client";

import { useEffect, useRef, useState } from "react";
import { Link } from "../../../../i18n/navigation";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { Input, Select } from "../../../components/ui/Input";
import { Badge } from "../../../components/ui/Badge";
import { formatAge } from "../../../console/formatters";
import type { Asset, Group } from "../../../console/models";
import { ensureArray, ensureRecord } from "../../../lib/responseGuards";
import { friendlySourceLabel, friendlyTypeLabel, sourceIcon } from "../../../console/taxonomy";
import { assetFreshness, type DensityMode } from "./nodesPageUtils";

interface PendingAgent {
  asset_id: string;
  hostname: string;
  platform: string;
  remote_ip: string;
  connected_at: string;
  device_fingerprint?: string;
  device_key_alg?: string;
  identity_verified: boolean;
  identity_verified_at?: string;
}

export function PendingAgentsBanner() {
  const [pending, setPending] = useState<PendingAgent[]>([]);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busyAssetID, setBusyAssetID] = useState<string | null>(null);
  const inFlightRef = useRef(false);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    let intervalID: ReturnType<typeof setInterval> | null = null;

    const poll = async () => {
      if (inFlightRef.current || document.visibilityState === "hidden") {
        return;
      }
      inFlightRef.current = true;
      const controller = new AbortController();
      abortRef.current = controller;
      try {
        const res = await fetch("/api/agents/pending", {
          cache: "no-store",
          signal: controller.signal,
        });
        if (res.ok) {
          const data = ensureRecord(await res.json().catch(() => null));
          setPending(ensureArray<PendingAgent>(data?.agents));
        }
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") {
          return;
        }
        // ignore network errors — banner simply stays empty
      } finally {
        if (abortRef.current === controller) {
          abortRef.current = null;
        }
        inFlightRef.current = false;
      }
    };

    const start = () => {
      void poll();
      if (intervalID === null) {
        intervalID = setInterval(() => void poll(), 5000);
      }
    };

    const stop = () => {
      if (intervalID !== null) {
        clearInterval(intervalID);
        intervalID = null;
      }
      abortRef.current?.abort();
      abortRef.current = null;
      inFlightRef.current = false;
    };

    const onVisibilityChange = () => {
      if (document.visibilityState === "visible") {
        start();
        return;
      }
      stop();
    };

    if (document.visibilityState === "visible") {
      start();
    }
    document.addEventListener("visibilitychange", onVisibilityChange);

    return () => {
      document.removeEventListener("visibilitychange", onVisibilityChange);
      stop();
    };
  }, []);

  if (pending.length === 0) return null;

  const approve = async (assetId: string) => {
    setBusyAssetID(assetId);
    setActionError(null);
    try {
      const response = await fetch("/api/agents/approve", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ asset_id: assetId }),
      });
      if (!response.ok) {
        const payload = (await response.json().catch(() => null)) as { error?: string } | null;
        throw new Error(payload?.error || `approve failed (${response.status})`);
      }
      setPending((p) => p.filter((a) => a.asset_id !== assetId));
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "approve failed");
    } finally {
      setBusyAssetID((current) => (current === assetId ? null : current));
    }
  };

  const reject = async (assetId: string) => {
    setBusyAssetID(assetId);
    setActionError(null);
    try {
      const response = await fetch("/api/agents/reject", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ asset_id: assetId }),
      });
      if (!response.ok) {
        const payload = (await response.json().catch(() => null)) as { error?: string } | null;
        throw new Error(payload?.error || `reject failed (${response.status})`);
      }
      setPending((p) => p.filter((a) => a.asset_id !== assetId));
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "reject failed");
    } finally {
      setBusyAssetID((current) => (current === assetId ? null : current));
    }
  };

  return (
    <Card className="mb-4" style={{ borderColor: "var(--warn-glow)", boxShadow: "0 0 12px var(--warn-glow)" }}>
      <div>
        <h3 className="text-sm font-semibold text-[var(--warn)] mb-3">
          Pending Agent Approvals ({pending.length})
        </h3>
        {actionError ? (
          <p className="text-xs text-[var(--bad)] mb-2">{actionError}</p>
        ) : null}
        <div className="space-y-2">
          {pending.map((agent) => (
            <div key={agent.asset_id} className="flex items-center gap-3 flex-wrap text-sm">
              <span className="inline-block h-2 w-2 rounded-full bg-[var(--warn)]" style={{ boxShadow: "0 0 8px var(--warn-glow)" }} />
              <span className="font-medium text-[var(--text)]">{agent.hostname}</span>
              <code className="text-xs text-[var(--muted)] bg-[var(--surface)] px-1.5 py-0.5 rounded">
                {agent.platform}
              </code>
              <code className="text-xs text-[var(--muted)]">
                {agent.remote_ip}
              </code>
              <span className="text-xs text-[var(--muted)]">
                Connected {formatAge(agent.connected_at)}
              </span>
              <code className="text-[10px] text-[var(--text)]/90 bg-[var(--bg-secondary)] px-1.5 py-0.5 rounded break-all">
                {agent.device_fingerprint || "fingerprint unavailable"}
              </code>
              <span
                className={`text-[10px] font-semibold px-1.5 py-0.5 rounded border ${
                  agent.identity_verified
                    ? "text-[var(--ok)] border-[var(--ok)]/40 bg-[var(--ok-glow)]"
                    : "text-[var(--warn)] border-[var(--warn)]/40 bg-[var(--warn-glow)]"
                }`}
              >
                {agent.identity_verified ? "Identity Verified" : "Identity Unverified"}
              </span>
              <div className="ml-auto flex gap-2">
                <Button
                  size="sm"
                  variant="primary"
                  disabled={busyAssetID === agent.asset_id || !agent.identity_verified}
                  onClick={() => void approve(agent.asset_id)}
                >
                  {busyAssetID === agent.asset_id
                    ? "Working..."
                    : agent.identity_verified
                      ? "Approve"
                      : "Awaiting Proof"}
                </Button>
                <Button
                  size="sm"
                  variant="danger"
                  disabled={busyAssetID === agent.asset_id}
                  onClick={() => void reject(agent.asset_id)}
                >
                  Reject
                </Button>
              </div>
            </div>
          ))}
        </div>
      </div>
    </Card>
  );
}

export function locationChipLabel(
  asset: Asset,
  groupNameByID: ReadonlyMap<string, string>,
): string {
  if (!asset.group_id) return "Unassigned";
  return groupNameByID.get(asset.group_id) ?? asset.group_id;
}

type AssetEditButtonProps = {
  asset: Asset;
  active: boolean;
  onClick: () => void;
};

export function AssetEditButton({ asset, active, onClick }: AssetEditButtonProps) {
  return (
    <Button
      size="sm"
      variant={active ? "secondary" : "ghost"}
      aria-label={`${active ? "Close edit for" : "Edit"} ${asset.name}`}
      onClick={onClick}
    >
      {active ? "Editing" : "Edit"}
    </Button>
  );
}

type InlineAssetEditorProps = {
  draftName: string;
  draftGroupID: string;
  draftTags: string;
  groupOptions: Group[];
  saving: boolean;
  error: string;
  onDraftNameChange: (value: string) => void;
  onDraftGroupChange: (value: string) => void;
  onDraftTagsChange: (value: string) => void;
  onSave: () => void;
  onCancel: () => void;
};

export function InlineAssetEditor({
  draftName,
  draftGroupID,
  draftTags,
  groupOptions,
  saving,
  error,
  onDraftNameChange,
  onDraftGroupChange,
  onDraftTagsChange,
  onSave,
  onCancel,
}: InlineAssetEditorProps) {
  return (
    <div className="mt-2 rounded-lg border border-[var(--line)] bg-[var(--bg-secondary)] p-3">
      <div className="grid grid-cols-1 gap-2 md:grid-cols-3">
        <Input
          value={draftName}
          onChange={event => onDraftNameChange(event.target.value)}
          placeholder="Device name"
          disabled={saving}
        />
        <Select
          value={draftGroupID}
          onChange={event => onDraftGroupChange(event.target.value)}
          disabled={saving}
        >
          <option value="">Unassigned</option>
          {groupOptions.map(group => (
            <option key={group.id} value={group.id}>
              {group.name}
            </option>
          ))}
        </Select>
        <Input
          value={draftTags}
          onChange={event => onDraftTagsChange(event.target.value)}
          placeholder="tag1, tag2"
          disabled={saving}
        />
      </div>
      {error ? <p className="mt-2 text-xs text-[var(--bad)]">{error}</p> : null}
      <div className="mt-3 flex items-center gap-2">
        <Button size="sm" variant="primary" onClick={onSave} disabled={saving}>
          {saving ? "Saving..." : "Save"}
        </Button>
        <Button size="sm" variant="secondary" onClick={onCancel} disabled={saving}>
          Cancel
        </Button>
      </div>
    </div>
  );
}

type AssetRowProps = {
  asset: Asset;
  density: DensityMode;
  groupOptions: Group[];
  groupNameByID: ReadonlyMap<string, string>;
  selected: boolean;
  isEditing: boolean;
  draftName: string;
  draftGroupID: string;
  draftTags: string;
  savingEdit: boolean;
  editError: string;
  onToggleSelected: (assetID: string) => void;
  onStartEdit: (asset: Asset) => void;
  onDraftNameChange: (value: string) => void;
  onDraftGroupChange: (value: string) => void;
  onDraftTagsChange: (value: string) => void;
  onSaveEdit: () => void;
  onCancelEdit: () => void;
};

export function AssetRow({
  asset,
  density,
  groupOptions,
  groupNameByID,
  selected,
  isEditing,
  draftName,
  draftGroupID,
  draftTags,
  savingEdit,
  editError,
  onToggleSelected,
  onStartEdit,
  onDraftNameChange,
  onDraftGroupChange,
  onDraftTagsChange,
  onSaveEdit,
  onCancelEdit,
}: AssetRowProps) {
  const Icon = sourceIcon(asset.source);
  const tagList = asset.tags ?? [];
  return (
    <li className={density === "compact" ? "py-2" : "py-2.5"}>
      <div className="flex flex-wrap items-center gap-3">
        <input
          type="checkbox"
          checked={selected}
          onChange={() => onToggleSelected(asset.id)}
          className="h-3.5 w-3.5 cursor-pointer rounded border border-[var(--line)] bg-[var(--control-input-bg)]"
          aria-label={`Select ${asset.name}`}
        />
        <Icon size={14} className="shrink-0 text-[var(--muted)]" />
        <Link
          href={`/nodes/${asset.id}`}
          className="text-sm font-medium text-[var(--accent)] hover:underline"
        >
          {asset.name}
        </Link>
        <span className="rounded-lg border border-[var(--line)] px-1.5 py-0.5 text-[10px] text-[var(--muted)]">
          {friendlySourceLabel(asset.source)} &middot;{" "}
          {friendlyTypeLabel((asset.resource_kind || asset.type || "").trim())}
        </span>
        <span
          className={`rounded-lg border px-1.5 py-0.5 text-[10px] ${
            asset.group_id
              ? "border-emerald-500/35 bg-emerald-500/10 text-emerald-300"
              : "border-amber-500/35 bg-amber-500/10 text-amber-300"
          }`}
        >
          {locationChipLabel(asset, groupNameByID)}
        </span>
        <Badge status={assetFreshness(asset)} size="sm" />
        <span className="text-xs text-[var(--muted)]">{formatAge(asset.last_seen_at)}</span>
        {tagList.map(tag => (
          <span
            key={`${asset.id}-${tag}`}
            className="rounded-lg border border-violet-500/35 bg-violet-500/10 px-1.5 py-0.5 text-[10px] text-violet-300"
          >
            #{tag}
          </span>
        ))}
        <span className="ml-auto">
          <AssetEditButton
            asset={asset}
            active={isEditing}
            onClick={() => onStartEdit(asset)}
          />
        </span>
      </div>
      {isEditing ? (
        <InlineAssetEditor
          draftName={draftName}
          draftGroupID={draftGroupID}
          draftTags={draftTags}
          groupOptions={groupOptions}
          saving={savingEdit}
          error={editError}
          onDraftNameChange={onDraftNameChange}
          onDraftGroupChange={onDraftGroupChange}
          onDraftTagsChange={onDraftTagsChange}
          onSave={onSaveEdit}
          onCancel={onCancelEdit}
        />
      ) : null}
    </li>
  );
}
