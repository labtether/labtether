"use client";

import { Link } from "../../../../i18n/navigation";
import { Button } from "../../../components/ui/Button";
import { Input, Select } from "../../../components/ui/Input";
import { Badge } from "../../../components/ui/Badge";
import { formatAge } from "../../../console/formatters";
import type { Asset, Group } from "../../../console/models";
import { friendlySourceLabel, friendlyTypeLabel, sourceIcon } from "../../../console/taxonomy";
import { assetFreshness, type DensityMode } from "./nodesPageUtils";

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
