"use client";

import { Link } from "../../../../i18n/navigation";
import { ChevronDown, ChevronRight } from "lucide-react";
import { Badge } from "../../../components/ui/Badge";
import { MiniBar } from "../../../components/ui/MiniBar";
import { formatAge } from "../../../console/formatters";
import type { Asset, Group } from "../../../console/models";
import {
  friendlySourceLabel,
  friendlyTypeLabel,
  groupByCategory,
  sourceIcon,
} from "../../../console/taxonomy";
import {
  AssetEditButton,
  AssetRow,
  InlineAssetEditor,
  locationChipLabel,
} from "./nodesPageComponents";
import {
  assetFreshness,
  parsePercent,
  type DensityMode,
  type FreshnessCounts,
} from "./nodesPageUtils";

type GroupSection = {
  groupID: string;
  groupLabel: string;
  counts: FreshnessCounts;
  servers: Array<{ server: Asset; children: Asset[] }>;
  orphans: Asset[];
  regular: Asset[];
};

type NodesGroupSectionsProps = {
  groupSections: GroupSection[];
  expandedSites: ReadonlySet<string>;
  expandedServers: ReadonlySet<string>;
  selectedAssetIDs: ReadonlySet<string>;
  density: DensityMode;
  groupOptions: Group[];
  groupNameByID: ReadonlyMap<string, string>;
  editingAssetID: string | null;
  editingAssetName: string;
  editingAssetGroupID: string;
  editingAssetTags: string;
  savingAssetEdit: boolean;
  assetEditError: string;
  onToggleGroup: (groupID: string) => void;
  onToggleServer: (serverID: string) => void;
  onToggleAssetSelection: (assetID: string) => void;
  onStartAssetEdit: (asset: Asset) => void;
  onEditingAssetNameChange: (value: string) => void;
  onEditingAssetGroupIDChange: (value: string) => void;
  onEditingAssetTagsChange: (value: string) => void;
  onSaveAssetEdit: () => void;
  onCancelAssetEdit: () => void;
};

export function NodesGroupSections({
  groupSections,
  expandedSites,
  expandedServers,
  selectedAssetIDs,
  density,
  groupOptions,
  groupNameByID,
  editingAssetID,
  editingAssetName,
  editingAssetGroupID,
  editingAssetTags,
  savingAssetEdit,
  assetEditError,
  onToggleGroup,
  onToggleServer,
  onToggleAssetSelection,
  onStartAssetEdit,
  onEditingAssetNameChange,
  onEditingAssetGroupIDChange,
  onEditingAssetTagsChange,
  onSaveAssetEdit,
  onCancelAssetEdit,
}: NodesGroupSectionsProps) {
  if (groupSections.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-2 py-12">
        <p className="text-sm font-medium text-[var(--text)]">
          No devices match these filters
        </p>
        <p className="max-w-sm text-center text-xs text-[var(--muted)]">
          Try clearing search text or widening status/source filters.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {groupSections.map((section) => {
        const groupExpanded = expandedSites.has(section.groupID);
        return (
          <section
            key={section.groupID}
            className="overflow-hidden rounded-lg border border-[var(--line)]"
          >
            <button
              className="flex w-full items-center gap-3 bg-[var(--bg-secondary)] p-3 text-left transition-colors duration-150 hover:bg-[var(--hover)]"
              onClick={() => onToggleGroup(section.groupID)}
              aria-expanded={groupExpanded}
            >
              {groupExpanded ? (
                <ChevronDown
                  size={14}
                  className="shrink-0 text-[var(--muted)]"
                />
              ) : (
                <ChevronRight
                  size={14}
                  className="shrink-0 text-[var(--muted)]"
                />
              )}
              <span className="text-sm font-medium text-[var(--text)]">
                {section.groupLabel}
              </span>
              <span className="text-xs text-[var(--muted)]">
                {section.counts.total} device
                {section.counts.total !== 1 ? "s" : ""}
              </span>
              {section.groupID === "unassigned" ? (
                <span className="rounded-lg bg-sky-500/10 px-1.5 py-0.5 text-[10px] text-sky-400">
                  Use Assign Device on a row
                </span>
              ) : null}
              {section.counts.issues > 0 ? (
                <span className="rounded-lg bg-amber-500/10 px-1.5 py-0.5 text-[10px] text-amber-500">
                  {section.counts.issues} need attention
                </span>
              ) : (
                <span className="rounded-lg bg-emerald-500/10 px-1.5 py-0.5 text-[10px] text-emerald-500">
                  Healthy
                </span>
              )}
              <div className="ml-auto flex items-center gap-2 text-[10px] text-[var(--muted)]">
                {section.counts.unresponsive > 0 ? (
                  <span>{section.counts.unresponsive} unresponsive</span>
                ) : null}
                {section.counts.offline > 0 ? (
                  <span>{section.counts.offline} offline</span>
                ) : null}
              </div>
            </button>

            {groupExpanded ? (
              <ul className="divide-y divide-[var(--line)] px-4 py-1">
                {section.servers.map(({ server, children }) => {
                  const isExpanded = expandedServers.has(server.id);
                  const categories = groupByCategory(children);
                  const freshness = assetFreshness(server);
                  const cpu = parsePercent(
                    server.metadata?.cpu_used_percent ??
                      server.metadata?.cpu_percent,
                  );
                  const mem = parsePercent(
                    server.metadata?.memory_used_percent ??
                      server.metadata?.memory_percent,
                  );
                  const showPerfBars =
                    density === "diagnostic" && (cpu != null || mem != null);
                  const tagList = server.tags ?? [];

                  return (
                    <li
                      key={server.id}
                      className={density === "compact" ? "py-2" : "py-2.5"}
                    >
                      <div className="flex flex-wrap items-center gap-3">
                        {children.length > 0 ? (
                          <button
                            onClick={() => onToggleServer(server.id)}
                            className="flex h-5 w-5 items-center justify-center text-[var(--muted)] transition-colors duration-150 hover:text-[var(--text)]"
                            aria-label={isExpanded ? "Collapse" : "Expand"}
                          >
                            {isExpanded ? (
                              <ChevronDown size={14} />
                            ) : (
                              <ChevronRight size={14} />
                            )}
                          </button>
                        ) : (
                          <span className="w-5" />
                        )}
                        <input
                          type="checkbox"
                          checked={selectedAssetIDs.has(server.id)}
                          onChange={() => onToggleAssetSelection(server.id)}
                          className="h-3.5 w-3.5 cursor-pointer rounded border border-[var(--line)] bg-[var(--control-input-bg)]"
                          aria-label={`Select ${server.name}`}
                        />
                        {(() => {
                          const Icon = sourceIcon(server.source);
                          return (
                            <Icon
                              size={14}
                              className="shrink-0 text-[var(--muted)]"
                            />
                          );
                        })()}
                        <Link
                          href={`/nodes/${server.id}`}
                          className="text-sm font-medium text-[var(--accent)] hover:underline"
                        >
                          {server.name}
                        </Link>
                        <span className="rounded-lg border border-[var(--line)] px-1.5 py-0.5 text-[10px] text-[var(--muted)]">
                          {friendlySourceLabel(server.source)} &middot;{" "}
                          {friendlyTypeLabel(
                            (server.resource_kind || server.type || "").trim(),
                          )}
                        </span>
                        <span
                          className={`rounded-lg border px-1.5 py-0.5 text-[10px] ${
                            server.group_id
                              ? "border-emerald-500/35 bg-emerald-500/10 text-emerald-300"
                              : "border-amber-500/35 bg-amber-500/10 text-amber-300"
                          }`}
                        >
                          {locationChipLabel(server, groupNameByID)}
                        </span>
                        <Badge status={freshness} size="sm" />
                        <span className="text-xs text-[var(--muted)]">
                          {formatAge(server.last_seen_at)}
                        </span>
                        {tagList.map((tag) => (
                          <span
                            key={`${server.id}-${tag}`}
                            className="rounded-lg border border-violet-500/35 bg-violet-500/10 px-1.5 py-0.5 text-[10px] text-violet-300"
                          >
                            #{tag}
                          </span>
                        ))}
                        {showPerfBars ? (
                          <span className="inline-flex items-center gap-3">
                            {cpu != null ? (
                              <MiniBar
                                value={cpu}
                                label={`CPU ${Math.round(cpu)}%`}
                              />
                            ) : null}
                            {mem != null ? (
                              <MiniBar
                                value={mem}
                                label={`Mem ${Math.round(mem)}%`}
                              />
                            ) : null}
                          </span>
                        ) : null}
                        <span className="ml-auto inline-flex items-center gap-1.5">
                          <AssetEditButton
                            asset={server}
                            active={editingAssetID === server.id}
                            onClick={() => onStartAssetEdit(server)}
                          />
                          {!isExpanded && children.length > 0 ? (
                            <>
                              {(() => {
                                const typeCounts = new Map<string, number>();
                                for (const child of children) {
                                  const label = friendlyTypeLabel(
                                    (
                                      child.resource_kind ||
                                      child.type ||
                                      ""
                                    ).trim(),
                                  );
                                  typeCounts.set(
                                    label,
                                    (typeCounts.get(label) ?? 0) + 1,
                                  );
                                }
                                return Array.from(typeCounts.entries()).map(
                                  ([label, count]) => (
                                    <span
                                      key={label}
                                      className="rounded-lg bg-[var(--surface)] px-1.5 py-0.5 text-[10px] text-[var(--muted)]"
                                    >
                                      {count} {label}
                                      {count !== 1 ? "s" : ""}
                                    </span>
                                  ),
                                );
                              })()}
                            </>
                          ) : null}
                        </span>
                      </div>
                      {editingAssetID === server.id ? (
                        <InlineAssetEditor
                          draftName={editingAssetName}
                          draftGroupID={editingAssetGroupID}
                          draftTags={editingAssetTags}
                          groupOptions={groupOptions}
                          saving={savingAssetEdit}
                          error={assetEditError}
                          onDraftNameChange={onEditingAssetNameChange}
                          onDraftGroupChange={onEditingAssetGroupIDChange}
                          onDraftTagsChange={onEditingAssetTagsChange}
                          onSave={onSaveAssetEdit}
                          onCancel={onCancelAssetEdit}
                        />
                      ) : null}
                      {isExpanded && categories.size > 0 ? (
                        <ul className="mt-1 ml-7 space-y-0">
                          {Array.from(categories.entries()).map(
                            ([cat, assets]) => (
                              <li key={cat.slug}>
                                <Link
                                  href={`/nodes/${server.id}?panel=${cat.slug}`}
                                  className="group flex items-center gap-3 py-1.5"
                                >
                                  <span className="text-sm text-[var(--text)] transition-colors duration-150 group-hover:text-[var(--accent)]">
                                    {cat.label}
                                  </span>
                                  <span className="text-xs text-[var(--muted)]">
                                    {assets.length}{" "}
                                    {assets.length === 1
                                      ? "resource"
                                      : "resources"}
                                  </span>
                                  <ChevronRight
                                    size={12}
                                    className="text-[var(--muted)] transition-colors duration-150 group-hover:text-[var(--accent)]"
                                  />
                                </Link>
                              </li>
                            ),
                          )}
                        </ul>
                      ) : null}
                    </li>
                  );
                })}

                {section.orphans.map((asset) => (
                  <AssetRow
                    key={asset.id}
                    asset={asset}
                    density={density}
                    groupOptions={groupOptions}
                    groupNameByID={groupNameByID}
                    selected={selectedAssetIDs.has(asset.id)}
                    isEditing={editingAssetID === asset.id}
                    draftName={editingAssetName}
                    draftGroupID={editingAssetGroupID}
                    draftTags={editingAssetTags}
                    savingEdit={savingAssetEdit}
                    editError={assetEditError}
                    onToggleSelected={onToggleAssetSelection}
                    onStartEdit={onStartAssetEdit}
                    onDraftNameChange={onEditingAssetNameChange}
                    onDraftGroupChange={onEditingAssetGroupIDChange}
                    onDraftTagsChange={onEditingAssetTagsChange}
                    onSaveEdit={onSaveAssetEdit}
                    onCancelEdit={onCancelAssetEdit}
                  />
                ))}

                {section.regular.map((asset) => (
                  <AssetRow
                    key={asset.id}
                    asset={asset}
                    density={density}
                    groupOptions={groupOptions}
                    groupNameByID={groupNameByID}
                    selected={selectedAssetIDs.has(asset.id)}
                    isEditing={editingAssetID === asset.id}
                    draftName={editingAssetName}
                    draftGroupID={editingAssetGroupID}
                    draftTags={editingAssetTags}
                    savingEdit={savingAssetEdit}
                    editError={assetEditError}
                    onToggleSelected={onToggleAssetSelection}
                    onStartEdit={onStartAssetEdit}
                    onDraftNameChange={onEditingAssetNameChange}
                    onDraftGroupChange={onEditingAssetGroupIDChange}
                    onDraftTagsChange={onEditingAssetTagsChange}
                    onSaveEdit={onSaveAssetEdit}
                    onCancelEdit={onCancelAssetEdit}
                  />
                ))}
              </ul>
            ) : null}
          </section>
        );
      })}
    </div>
  );
}
