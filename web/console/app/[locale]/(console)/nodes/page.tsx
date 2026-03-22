"use client";

import { useDeferredValue, useMemo, useState, useCallback, useEffect, useRef } from "react";
import { useTranslations } from "next-intl";
import { Plus, Search, Server } from "lucide-react";
import { PageHeader } from "../../../components/PageHeader";
import { Button } from "../../../components/ui/Button";
import { EmptyState } from "../../../components/ui/EmptyState";
import { Input } from "../../../components/ui/Input";
import { AddDeviceModal } from "../../../components/AddDeviceModal";
import { RoutePerfBoundary } from "../../../components/RoutePerfBoundary";
import { useFastStatus, useSlowStatus, useStatusControls } from "../../../contexts/StatusContext";
import { PendingAgentsBanner } from "./nodesPageComponents";
import { DeviceTree } from "./DeviceTree";
import { useDeviceTree } from "./useDeviceTree";
import { useHierarchyManagement } from "./useHierarchyManagement";
import { ManageHierarchyBar } from "./ManageHierarchyBar";
import { GroupCreateDialog } from "./GroupCreateDialog";
import { LinkSuggestionsBanner } from "./LinkSuggestionsBanner";
import { LinkReviewPanel } from "./LinkReviewPanel";
import { useLinkSuggestions } from "../../../hooks/useLinkSuggestions";
import { useEdges } from "../../../hooks/useEdges";
import { useProposals } from "../../../hooks/useProposals";
import { reportRoutePerfMetric } from "../../../hooks/useRoutePerfTelemetry";
import type { CompositeResolvedAsset } from "../../../console/models";

export default function NodesPage() {
  const t = useTranslations('devices');
  const status = useFastStatus();
  const slowStatus = useSlowStatus();
  const { fetchStatus } = useStatusControls();
  const [showAddDevice, setShowAddDevice] = useState(false);
  const [query, setQuery] = useState("");
  const queryInteractionStartedAtRef = useRef<number | null>(null);
  const [showCreateGroup, setShowCreateGroup] = useState(false);
  const [createGroupParent, setCreateGroupParent] = useState<string | undefined>();
  const [showLinkReview, setShowLinkReview] = useState(false);
  const [resolvedAssets, setResolvedAssets] = useState<CompositeResolvedAsset[]>([]);

  const management = useHierarchyManagement(fetchStatus);
  const { suggestions } = useLinkSuggestions();

  // Edge data for shallow parent-child nesting
  const allAssetIDs = useMemo(
    () => (status?.assets ?? []).map((a) => a.id),
    [status?.assets],
  );
  const { edges } = useEdges(allAssetIDs);
  const { proposals, accept, dismiss } = useProposals();

  // Fetch assets with composites resolved so device tree nodes get facet annotations.
  useEffect(() => {
    let cancelled = false;
    async function fetchResolvedAssets() {
      try {
        const res = await fetch("/api/assets?resolve_composites=true", { cache: "no-store" });
        if (!res.ok) return;
        const data = (await res.json()) as { assets?: CompositeResolvedAsset[] };
        if (!cancelled && Array.isArray(data.assets)) {
          setResolvedAssets(data.assets);
        }
      } catch {
        // Non-critical — tree still renders without composite facets
      }
    }
    void fetchResolvedAssets();
    return () => { cancelled = true; };
  }, []);

  const deferredQuery = useDeferredValue(query);
  const queryTerm = deferredQuery.trim().toLowerCase();

  const { tree, toggleExpand, expandAll, collapseAll } = useDeviceTree({
    groups: slowStatus?.groups ?? [],
    assets: status?.assets ?? [],
    telemetryOverview: status?.telemetryOverview ?? [],
    query: queryTerm,
    edges,
    proposals,
    resolvedAssets,
  });

  // Perf telemetry: count devices in tree
  const deviceCount = useMemo(() => {
    let count = 0;
    function walk(items: typeof tree) {
      for (const item of items) {
        if (item.type === "device") count++;
        if (item.children.length > 0) walk(item.children);
      }
    }
    walk(tree);
    return count;
  }, [tree]);

  useEffect(() => {
    reportRoutePerfMetric({
      route: "devices",
      metric: "compute.device_tree",
      durationMs: 0,
        sampleSize: deviceCount,
        metadata: {
          assets: status?.assets?.length ?? 0,
          groups: slowStatus?.groups?.length ?? 0,
          query_active: queryTerm !== "",
        },
      });
  }, [queryTerm, deviceCount, slowStatus?.groups?.length, status?.assets?.length]);

  useEffect(() => {
    const startedAt = queryInteractionStartedAtRef.current;
    if (startedAt == null) {
      return;
    }
    queryInteractionStartedAtRef.current = null;
    reportRoutePerfMetric({
      route: "devices",
      metric: "render.query_to_tree",
      durationMs: performance.now() - startedAt,
      sampleSize: deviceCount,
      metadata: {
        query_length: queryTerm.length,
      },
    });
  }, [queryTerm, deviceCount, tree]);

  const handleDeviceAdded = useCallback(() => {
    void fetchStatus();
  }, [fetchStatus]);

  const handleCreateChildGroup = useCallback((parentGroupID: string) => {
    setCreateGroupParent(parentGroupID);
    setShowCreateGroup(true);
  }, []);

  const handleOpenCreateGroup = useCallback(() => {
    setCreateGroupParent(undefined);
    setShowCreateGroup(true);
  }, []);

  const hasAssets = (status?.assets ?? []).length > 0;

  return (
    <RoutePerfBoundary
      route="devices"
      sampleSize={deviceCount}
      metadata={{
        assets: status?.assets?.length ?? 0,
        groups: slowStatus?.groups?.length ?? 0,
        query_active: queryTerm !== "",
        suggestions: suggestions.length,
      }}
    >
      <>
      <PageHeader
        title={t('title')}
        subtitle={t('subtitle')}
        action={
          <Button
            variant="primary"
            size="sm"
            onClick={() => setShowAddDevice(true)}
          >
            <Plus size={14} />
            {t('addDevice')}
          </Button>
        }
      />

      <AddDeviceModal
        open={showAddDevice}
        onClose={() => setShowAddDevice(false)}
        onAdded={handleDeviceAdded}
      />

      <GroupCreateDialog
        open={showCreateGroup}
        onClose={() => setShowCreateGroup(false)}
        onSubmit={(name, parentGroupID) =>
          management.createGroup(name, parentGroupID ?? createGroupParent)
        }
        groups={slowStatus?.groups ?? []}
      />

      <PendingAgentsBanner />

      <LinkSuggestionsBanner
        count={proposals.length}
        onReview={() => setShowLinkReview(true)}
      />

      {showLinkReview && (
        <LinkReviewPanel
          proposals={proposals}
          assets={status?.assets ?? []}
          onAccept={accept}
          onDismiss={dismiss}
          onClose={() => setShowLinkReview(false)}
        />
      )}

      {hasAssets ? (
        <div className="space-y-4">
          {/* Search + manage toolbar */}
          <div className="flex items-center justify-between gap-4">
            <div className="relative max-w-md flex-1">
              <Search
                size={14}
                className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--muted)] pointer-events-none"
              />
              <Input
                value={query}
                onChange={e => {
                  queryInteractionStartedAtRef.current = performance.now();
                  setQuery(e.target.value);
                }}
                placeholder={t('search.placeholder')}
                className="pl-9"
                aria-label={t('search.label')}
              />
            </div>

            <ManageHierarchyBar
              isManaging={management.isManaging}
              onToggle={() => management.setIsManaging(!management.isManaging)}
              onCreateGroup={handleOpenCreateGroup}
            />
          </div>

          {/* Device tree */}
          <DeviceTree
            tree={tree}
            onToggle={toggleExpand}
            onExpandAll={expandAll}
            onCollapseAll={collapseAll}
            isManaging={management.isManaging}
            onMoveAsset={management.moveAsset}
            onMoveGroup={management.moveGroup}
            onRenameGroup={management.renameGroup}
            onDeleteGroup={management.deleteGroup}
            onCreateChildGroup={handleCreateChildGroup}
          />
        </div>
      ) : (
        <EmptyState
          icon={Server}
          title={t('empty.title')}
          description={t('empty.description')}
          action={
            <Button
              variant="primary"
              size="sm"
              onClick={() => setShowAddDevice(true)}
            >
              <Plus size={14} />
              {t('empty.action')}
            </Button>
          }
        />
      )}
      </>
    </RoutePerfBoundary>
  );
}
