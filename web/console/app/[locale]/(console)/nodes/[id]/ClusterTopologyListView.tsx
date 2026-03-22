"use client";

import { Link } from "../../../../../i18n/navigation";
import { Select } from "../../../../components/ui/Input";
import type { Asset } from "../../../../console/models";
import { friendlyTypeLabel } from "../../../../console/taxonomy";
import {
  guestStatusColor,
  haStateColor,
  sourceBadgeLabel,
} from "./clusterTopologyUtils";
import type {
  AssetDependency,
  ClusterStatusEntry,
  HAResource,
} from "./clusterTopologyTypes";

type ClusterTopologyListViewProps = {
  nodeEntries: ClusterStatusEntry[];
  haByNode: Map<string, HAResource[]>;
  guestsByNode: Map<string, Asset[]>;
  guestRunsOn: Record<string, AssetDependency | null>;
  linkDrafts: Record<string, string>;
  linkErrors: Record<string, string>;
  savingGuestID: string | null;
  autoLinkingGuestID: string | null;
  loadingGuestLinks: boolean;
  apiHosts: Asset[];
  apiHostsByID: Map<string, Asset>;
  suggestedHostForGuest: (guest: Asset) => Asset | undefined;
  onDraftChange: (guestID: string, targetID: string) => void;
  onSaveGuestLink: (guest: Asset) => void;
  onClearGuestLink: (guest: Asset) => void;
};

export function ClusterTopologyListView({
  nodeEntries,
  haByNode,
  guestsByNode,
  guestRunsOn,
  linkDrafts,
  linkErrors,
  savingGuestID,
  autoLinkingGuestID,
  loadingGuestLinks,
  apiHosts,
  apiHostsByID,
  suggestedHostForGuest,
  onDraftChange,
  onSaveGuestLink,
  onClearGuestLink,
}: ClusterTopologyListViewProps) {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
      {nodeEntries.map((node) => {
        const isOnline = node.online === 1;
        const isLocal = node.local === 1;
        const nodeHAResources = haByNode.get(node.name ?? "") ?? [];
        const nodeGuests = guestsByNode.get(node.name ?? "") ?? [];
        const linkedGuestCount = nodeGuests.reduce((count, guest) => {
          const dep = guestRunsOn[guest.id] ?? null;
          const linkedHost = dep ? apiHostsByID.get(dep.target_asset_id) : undefined;
          return count + (linkedHost ? 1 : 0);
        }, 0);

        return (
          <div
            key={node.nodeid ?? node.name}
            className={`rounded-lg border-2 p-3 transition-colors duration-150 ${
              isOnline
                ? "border-[var(--ok)]/40 bg-[var(--ok)]/5"
                : "border-[var(--bad)]/30 bg-[var(--bad-glow)]"
            }`}
          >
            <div className="flex items-center justify-between mb-2">
              <div className="flex items-center gap-2">
                <div className={`w-2 h-2 rounded-full ${isOnline ? "bg-[var(--ok)]" : "bg-[var(--bad)]"}`} />
                <span className="text-xs font-medium text-[var(--text)]">{node.name ?? "unknown"}</span>
                {isLocal ? (
                  <span className="text-[10px] px-1 py-0.5 rounded bg-[var(--accent)]/10 text-[var(--accent)]">local</span>
                ) : null}
              </div>
              <span className={`text-[10px] font-medium ${isOnline ? "text-[var(--ok)]" : "text-[var(--bad)]"}`}>
                {isOnline ? "online" : "offline"}
              </span>
            </div>

            {node.ip ? (
              <p className="text-[10px] text-[var(--muted)] mb-2">{node.ip}</p>
            ) : null}

            {nodeGuests.length > 0 ? (
              <div className="mt-2 pt-2 border-t border-[var(--line)]">
                <div className="flex items-center justify-between mb-1">
                  <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)]">
                    Guests ({nodeGuests.length})
                  </p>
                  <div className="flex items-center gap-1">
                    {(loadingGuestLinks || autoLinkingGuestID != null) ? (
                      <span className="text-[10px] text-[var(--muted)]">syncing links...</span>
                    ) : null}
                    {linkedGuestCount > 0 ? (
                      <span className="text-[10px] px-1 py-0.5 rounded bg-[var(--accent-subtle)] text-[var(--accent-text)]">
                        {linkedGuestCount} API-linked
                      </span>
                    ) : null}
                  </div>
                </div>
                {apiHosts.length === 0 ? (
                  <p className="text-[10px] text-[var(--muted)] mb-2">
                    No external API hosts available for mapping.
                  </p>
                ) : null}
                <ul className="space-y-2">
                  {nodeGuests.slice(0, 8).map((guest) => {
                    const status = (guest.metadata?.status ?? guest.status ?? "").toLowerCase();
                    const mapping = guestRunsOn[guest.id] ?? null;
                    const linkedHost = mapping ? apiHostsByID.get(mapping.target_asset_id) : undefined;
                    const draftTargetID = linkDrafts[guest.id] ?? "";
                    const guestSaving = savingGuestID === guest.id || autoLinkingGuestID === guest.id;
                    const canSaveMapping = draftTargetID !== "" && (!mapping || mapping.target_asset_id !== draftTargetID);
                    const suggestion = suggestedHostForGuest(guest);
                    const showNameSuggestion = !mapping && suggestion && draftTargetID === suggestion.id;
                    const isAutoMapping = mapping?.metadata?.binding === "auto";

                    return (
                      <li key={guest.id} className="space-y-2 rounded-md border border-[var(--line)]/70 bg-[var(--surface)]/50 p-2">
                        <div className="flex flex-col gap-1 sm:flex-row sm:items-start sm:justify-between">
                          <p className="min-w-0 text-xs text-[var(--text)]">
                            <span className="font-medium">{guest.name}</span>{" "}
                            <span className="text-[var(--muted)]">({friendlyTypeLabel(guest.type)})</span>
                          </p>
                          <div className="flex flex-wrap items-center gap-1">
                            {status ? (
                              <span className={`text-[10px] ${guestStatusColor(status)}`}>{status}</span>
                            ) : null}
                            {linkedHost ? (
                              <Link
                                href={`/nodes/${encodeURIComponent(linkedHost.id)}`}
                                className="text-[10px] px-1 py-0.5 rounded bg-[var(--accent-subtle)] text-[var(--accent-text)] hover:bg-[var(--accent-subtle)] transition-colors"
                                title={`Explicit runs_on link to ${linkedHost.source} host: ${linkedHost.name}`}
                              >
                                {sourceBadgeLabel(linkedHost.source)} API
                              </Link>
                            ) : null}
                            {isAutoMapping ? (
                              <span className="text-[10px] px-1 py-0.5 rounded bg-[var(--ok-glow)] text-[var(--ok)]">
                                auto
                              </span>
                            ) : null}
                            {mapping && !linkedHost ? (
                              <span className="text-[10px] px-1 py-0.5 rounded bg-[var(--warn-glow)] text-[var(--warn)]">
                                target missing
                              </span>
                            ) : null}
                          </div>
                        </div>

                        <div className="space-y-1.5">
                          <Select
                            value={draftTargetID}
                            disabled={apiHosts.length === 0 || guestSaving}
                            onChange={(event) => { onDraftChange(guest.id, event.target.value); }}
                            className="h-8 w-full min-w-0 px-2.5 py-1 text-xs"
                          >
                            <option value="">
                              {apiHosts.length > 0 ? "Select API host..." : "No API hosts"}
                            </option>
                            {apiHosts.map((host) => (
                              <option key={host.id} value={host.id}>
                                {host.name} ({sourceBadgeLabel(host.source)})
                              </option>
                            ))}
                          </Select>

                          <div className="flex flex-wrap items-center gap-1.5">
                            <button
                              type="button"
                              disabled={guestSaving || !canSaveMapping}
                              onClick={() => { onSaveGuestLink(guest); }}
                              className="h-7 px-2.5 rounded border border-[var(--line)] text-[10px] font-medium text-[var(--text)] hover:bg-[var(--hover)] disabled:opacity-40 disabled:pointer-events-none"
                            >
                              {guestSaving ? "Saving..." : mapping ? "Update" : "Link"}
                            </button>

                            {mapping ? (
                              <button
                                type="button"
                                disabled={guestSaving}
                                onClick={() => { onClearGuestLink(guest); }}
                                className="h-7 px-2.5 rounded border border-[var(--line)] text-[10px] font-medium text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] disabled:opacity-40 disabled:pointer-events-none"
                              >
                                Clear
                              </button>
                            ) : null}

                            {showNameSuggestion ? (
                              <span className="text-[10px] text-[var(--muted)]">name match</span>
                            ) : null}
                          </div>
                        </div>

                        {linkErrors[guest.id] ? (
                          <p className="text-[10px] text-[var(--bad)]">{linkErrors[guest.id]}</p>
                        ) : null}
                      </li>
                    );
                  })}
                  {nodeGuests.length > 8 ? (
                    <li className="text-[10px] text-[var(--muted)]">+{nodeGuests.length - 8} more guests</li>
                  ) : null}
                </ul>
              </div>
            ) : null}

            {nodeHAResources.length > 0 ? (
              <div className="mt-2 pt-2 border-t border-[var(--line)]">
                <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)] mb-1">
                  HA Resources ({nodeHAResources.length})
                </p>
                <ul className="space-y-1">
                  {nodeHAResources.map((resource, idx) => {
                    const stateColor = haStateColor(resource.state);
                    return (
                      <li key={resource.sid ?? idx} className="flex items-center justify-between gap-2">
                        <span className="text-[10px] text-[var(--text)] truncate">
                          {resource.sid ?? "unknown"}
                        </span>
                        <div className="flex items-center gap-1 shrink-0">
                          {resource.group ? (
                            <span className="text-[10px] px-1 py-0.5 rounded bg-[var(--hover)] text-[var(--muted)]">
                              {resource.group}
                            </span>
                          ) : null}
                          <span className={`text-[10px] font-medium ${stateColor}`}>
                            {resource.state ?? "unknown"}
                          </span>
                        </div>
                      </li>
                    );
                  })}
                </ul>
              </div>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}
