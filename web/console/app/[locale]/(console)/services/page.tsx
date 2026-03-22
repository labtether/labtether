"use client";

import { startTransition, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { PageHeader } from "../../../components/PageHeader";
import { RoutePerfBoundary } from "../../../components/RoutePerfBoundary";
import { useWebServices, type WebService } from "../../../hooks/useWebServices";
import { useStatusAssetNameMap } from "../../../contexts/StatusContext";
import { useToast } from "../../../contexts/ToastContext";
import { ServiceEditModal } from "../../../components/ServiceEditModal";
import { ServiceBulkEditModal } from "../../../components/ServiceBulkEditModal";
import { ServicesGroupedGrid } from "./ServicesGroupedGrid";
import { ServicesDiscoveryOverviewCard } from "./ServicesDiscoveryOverviewCard";
import { ServicesFiltersBar } from "./ServicesFiltersBar";
import { ServicesHeaderActions } from "./ServicesHeaderActions";
import { ServicesModeBanners } from "./ServicesModeBanners";
import { ServicesResultStates } from "./ServicesResultStates";
import { buildDiscoveryOverview } from "./servicesDiscoveryHelpers";
import { useDerivedServiceCollections } from "./useDerivedServiceCollections";
import { useServiceEditModalActions } from "./useServiceEditModalActions";
import { usePullImagesAction } from "./usePullImagesAction";
import { useServiceBulkEditAction } from "./useServiceBulkEditAction";
import { useServiceIconLibrary } from "./useServiceIconLibrary";
import { useServiceLayoutOrchestration } from "./useServiceLayoutOrchestration";
import { useServiceMutationActions } from "./useServiceMutationActions";
import { useServiceSelectionState } from "./useServiceSelectionState";
import { reportRoutePerfMetric } from "../../../hooks/useRoutePerfTelemetry";
import type { ServiceHealthFilter, ServiceSortMode } from "./servicesPageHelpers";

export default function ServicesPage() {
  const t = useTranslations('services');
  const { addToast } = useToast();
  const assetNameMap = useStatusAssetNameMap();
  const [categoryFilter, setCategoryFilter] = useState<string>("All");
  const [hostFilter, setHostFilter] = useState<string>("all");
  const [statusFilter, setStatusFilter] = useState<string>("all");
  const [sourceFilter, setSourceFilter] = useState<string>("all");
  const [healthFilter, setHealthFilter] = useState<ServiceHealthFilter>("all");
  const [sortMode, setSortMode] = useState<ServiceSortMode>("default");
  const [showHidden, setShowHidden] = useState(false);
  const [expandedService, setExpandedService] = useState<string | null>(null);
  const [editingService, setEditingService] = useState<WebService | null>(null);
  const [bulkEditOpen, setBulkEditOpen] = useState(false);
  const {
    services,
    discoveryStats,
    suggestions,
    loading,
    syncing,
    error,
    refresh,
    sync,
    createManualService,
    updateManualService,
    deleteManualService,
    saveServiceOverride,
    listServiceOverrides,
    listCustomServiceIcons,
    createCustomServiceIcon,
    deleteCustomServiceIcon,
    renameCustomServiceIcon,
    loadServiceDetails,
  } = useWebServices({ includeHidden: showHidden, detailLevel: "compact" });
  const [serviceDetailsByID, setServiceDetailsByID] = useState<Map<string, WebService>>(() => new Map());

  const {
    iconIndex,
    customIcons,
    createCustomIcon,
    deleteCustomIcon,
    renameCustomIcon,
  } = useServiceIconLibrary({
    listCustomServiceIcons,
    createCustomServiceIcon,
    deleteCustomServiceIcon,
    renameCustomServiceIcon,
  });

  const {
    layoutMode,
    setLayoutMode,
    layoutOrderByCategory,
    draggingServiceKey,
    dragOverServiceKey,
    clearDragState,
    handleCardDragStart,
    handleCardDragOver,
    handleCardDrop,
    resetLayoutOrder,
  } = useServiceLayoutOrchestration({ services });

  const {
    selectionMode,
    selectedServiceKeys,
    reconcileSelectionWithFiltered,
    clearSelection,
    disableSelectionMode,
    handleToggleSelectionMode,
    handleToggleServiceSelection,
    handleSelectAllFiltered,
  } = useServiceSelectionState({
    layoutMode,
    setLayoutMode,
    clearDragState,
  });

  const {
    hosts,
    activeCategories,
    filtered,
    grouped,
    selectedFilteredCount,
    bulkEditTargets,
    deriveDurationMs,
  } = useDerivedServiceCollections({
    services,
    assetNameMap,
    categoryFilter,
    hostFilter,
    statusFilter,
    sourceFilter,
    healthFilter,
    sortMode,
    showHidden,
    selectionMode,
    selectedServiceKeys,
    layoutOrderByCategory,
  });

  const { pullPlan, pullingImages, handlePullImages } = usePullImagesAction({
    services,
    hostFilter,
    assetNameMap,
    refresh,
  });

  useEffect(() => {
    setServiceDetailsByID((current) => {
      if (current.size === 0) {
        return current;
      }
      const next = new Map<string, WebService>();
      let changed = false;
      for (const service of services) {
        const detailed = current.get(service.id);
        if (detailed) {
          next.set(service.id, detailed);
        } else if (current.has(service.id)) {
          changed = true;
        }
      }
      if (next.size !== current.size) {
        changed = true;
      }
      return changed ? next : current;
    });
  }, [services]);

  useEffect(() => {
    reconcileSelectionWithFiltered(filtered);
  }, [filtered, reconcileSelectionWithFiltered]);

  const shownSuggestionIds = useRef<Set<string>>(new Set());

  useEffect(() => {
    if (!suggestions || suggestions.length === 0) return;

    for (const suggestion of suggestions) {
      if (shownSuggestionIds.current.has(suggestion.id)) continue;
      shownSuggestionIds.current.add(suggestion.id);

      addToast(
        "info",
        `Is "${suggestion.suggested_url}" an alternative URL for ${suggestion.base_service_name}?`,
        15000,
        {
          label: t('suggestions.accept'),
          onClick: async () => {
            try {
              await fetch(`/api/services/web/grouping-suggestions/${encodeURIComponent(suggestion.id)}/accept`, { method: "POST" });
              refresh();
            } catch {
              // Silently handle errors; next poll will reconcile state.
            }
          },
        },
        {
          label: t('suggestions.deny'),
          onClick: async () => {
            try {
              await fetch(`/api/services/web/grouping-suggestions/${encodeURIComponent(suggestion.id)}/deny`, { method: "POST" });
              refresh();
            } catch {
              // Silently handle errors; next poll will reconcile state.
            }
          },
        }
      );
    }
  }, [t, suggestions, addToast, refresh]);

  const totalCount = services.length;
  const upCount = useMemo(
    () => services.filter((service) => service.status === "up" && service.metadata?.hidden !== "true").length,
    [services]
  );
  const discoveryOverview = useMemo(
    () => buildDiscoveryOverview(discoveryStats, assetNameMap),
    [discoveryStats, assetNameMap]
  );
  const { bulkSaving, handleApplyBulkEdits } = useServiceBulkEditAction({
    bulkEditTargets,
    selectionMode,
    clearSelection,
    listServiceOverrides,
    saveServiceOverride,
    refresh,
  });

  const handleToggleArrangeMode = useCallback(() => {
    setLayoutMode((current) => !current);
    disableSelectionMode();
    setExpandedService(null);
  }, [disableSelectionMode, setLayoutMode]);

  const handleSelectAllFilteredClick = useCallback(() => {
    handleSelectAllFiltered(filtered);
  }, [filtered, handleSelectAllFiltered]);

  const handleRefreshServices = useCallback(async () => {
    const targetHost = hostFilter !== "all" ? hostFilter : undefined;
    try {
      await sync(targetHost);
    } catch {
      // Error state is already surfaced by the hook.
    }
    await refresh();
  }, [hostFilter, refresh, sync]);

  const groupedWithDetails = useMemo(
    () => grouped.map(([category, categoryServices]) => [
      category,
      categoryServices.map((service) => serviceDetailsByID.get(service.id) ?? service),
    ] as [string, WebService[]]),
    [grouped, serviceDetailsByID]
  );

  useEffect(() => {
    reportRoutePerfMetric({
      route: "services",
      metric: "compute.derived_services",
      durationMs: deriveDurationMs,
      sampleSize: filtered.length,
      metadata: {
        categories: activeCategories.length,
        grouped_sections: grouped.length,
        host_filtered: hostFilter !== "all",
        status_filtered: statusFilter !== "all",
        source_filtered: sourceFilter !== "all",
        health_filtered: healthFilter !== "all",
        hidden_shown: showHidden,
        selection_mode: selectionMode,
      },
    });
  }, [
    activeCategories.length,
    deriveDurationMs,
    filtered.length,
    grouped.length,
    healthFilter,
    hostFilter,
    selectionMode,
    showHidden,
    sourceFilter,
    statusFilter,
  ]);

  useEffect(() => {
    const startedAt = typeof performance !== "undefined" ? performance.now() : Date.now();
    const scheduleAfterPaint = typeof window.requestAnimationFrame === "function"
      ? window.requestAnimationFrame.bind(window)
      : (callback: FrameRequestCallback) => window.setTimeout(() => callback(performance.now()), 0);
    scheduleAfterPaint(() => {
      reportRoutePerfMetric({
        route: "services",
        metric: "render.services_page",
        durationMs: (typeof performance !== "undefined" ? performance.now() : Date.now()) - startedAt,
        sampleSize: filtered.length,
        metadata: {
          grouped_sections: grouped.length,
          host_filtered: hostFilter !== "all",
          hidden_shown: showHidden,
        },
      });
    });
  }, [filtered.length, grouped.length, hostFilter, showHidden]);

  const {
    handleAddManualService,
    handleRename,
    handleToggleHidden,
    handleDeleteManual,
  } = useServiceMutationActions({
    hostFilter,
    hosts,
    refresh,
    createManualService,
    updateManualService,
    deleteManualService,
    saveServiceOverride,
    listServiceOverrides,
  });

  const handleLoadServiceDetails = useCallback(async (service: WebService) => {
    const cached = serviceDetailsByID.get(service.id);
    if (cached) {
      return cached;
    }
    const detailed = await loadServiceDetails(service.id, service.host_asset_id);
    if (!detailed) {
      return null;
    }
    startTransition(() => {
      setServiceDetailsByID((current) => {
        const next = new Map(current);
        next.set(detailed.id, detailed);
        return next;
      });
    });
    return detailed;
  }, [loadServiceDetails, serviceDetailsByID]);

  const handleToggleExpand = useCallback((service: WebService) => {
    setExpandedService((current) => (current === service.id ? null : service.id));
    void handleLoadServiceDetails(service).catch(() => {
      // Leave the compact row visible; the next expand can retry detail loading.
    });
  }, [handleLoadServiceDetails]);

  const handleOpenEditService = useCallback((service: WebService) => {
    setEditingService(serviceDetailsByID.get(service.id) ?? service);
  }, [serviceDetailsByID]);

  const {
    handleSaveEditModal,
    handleResetEditModal,
    handleAddAltURLEditModal,
    handleRemoveAltURLEditModal,
  } = useServiceEditModalActions({
    editingService,
    saveServiceOverride,
    refresh,
  });

  return (
    <RoutePerfBoundary
      route="services"
      sampleSize={filtered.length}
      metadata={{
        total_services: totalCount,
        filtered_services: filtered.length,
        grouped_sections: grouped.length,
        hosts: hosts.size,
        suggestions: suggestions.length,
        selection_mode: selectionMode,
      }}
    >
      <>
      <PageHeader
        title={t('title')}
        subtitle={
          totalCount > 0
            ? t('subtitle.count', { upCount, totalCount })
            : t('subtitle.default')
        }
        action={
          <ServicesHeaderActions
            selectionMode={selectionMode}
            layoutMode={layoutMode}
            bulkEditTargetCount={bulkEditTargets.length}
            bulkSaving={bulkSaving}
            syncing={syncing}
            pullingImages={pullingImages}
            pullPlanItemCount={pullPlan.items.length}
            onAddManual={handleAddManualService}
            onToggleSelectionMode={handleToggleSelectionMode}
            onOpenBulkEdit={() => setBulkEditOpen(true)}
            onToggleArrange={handleToggleArrangeMode}
            onRefresh={handleRefreshServices}
            onPullImages={handlePullImages}
          />
        }
      />

      <div className="flex-1 flex flex-col gap-4 p-4 md:p-6 overflow-y-auto">
        <ServicesFiltersBar
          totalCount={totalCount}
          activeCategories={activeCategories}
          categoryFilter={categoryFilter}
          onCategoryFilterChange={setCategoryFilter}
          hosts={hosts}
          hostFilter={hostFilter}
          onHostFilterChange={setHostFilter}
          sourceFilter={sourceFilter}
          onSourceFilterChange={setSourceFilter}
          healthFilter={healthFilter}
          onHealthFilterChange={(value) => setHealthFilter(value as ServiceHealthFilter)}
          sortMode={sortMode}
          onSortModeChange={(value) => setSortMode(value as ServiceSortMode)}
          showHidden={showHidden}
          onShowHiddenChange={setShowHidden}
          statusFilter={statusFilter}
          onStatusFilterChange={setStatusFilter}
        />

        <ServicesModeBanners
          layoutMode={layoutMode}
          totalCount={totalCount}
          selectionMode={selectionMode}
          selectedFilteredCount={selectedFilteredCount}
          filteredCount={filtered.length}
          onResetLayoutOrder={resetLayoutOrder}
          onSelectAllFiltered={handleSelectAllFilteredClick}
          onClearSelection={clearSelection}
        />

        <ServicesDiscoveryOverviewCard discoveryOverview={discoveryOverview} />

        <ServicesResultStates
          loading={loading}
          servicesCount={services.length}
          error={error}
          filteredCount={filtered.length}
        />

        {/* Service groups */}
        <ServicesGroupedGrid
          grouped={groupedWithDetails}
          assetNameMap={assetNameMap}
          expandedService={expandedService}
          layoutMode={layoutMode}
          selectionMode={selectionMode}
          selectedServiceKeys={selectedServiceKeys}
          draggingServiceKey={draggingServiceKey}
          dragOverServiceKey={dragOverServiceKey}
          onToggleExpand={handleToggleExpand}
          onToggleSelect={handleToggleServiceSelection}
          onEdit={handleOpenEditService}
          onRename={handleRename}
          onToggleHidden={handleToggleHidden}
          onDeleteManual={handleDeleteManual}
          onDragStart={handleCardDragStart}
          onDragOver={handleCardDragOver}
          onDrop={handleCardDrop}
          onDragEnd={clearDragState}
          onFilterHost={setHostFilter}
        />
      </div>

      {editingService && (
        <ServiceEditModal
          service={editingService}
          icons={iconIndex}
          customIcons={customIcons}
          onSave={handleSaveEditModal}
          onReset={handleResetEditModal}
          onAddAltURL={handleAddAltURLEditModal}
          onRemoveAltURL={handleRemoveAltURLEditModal}
          onCreateCustomIcon={createCustomIcon}
          onDeleteCustomIcon={deleteCustomIcon}
          onRenameCustomIcon={renameCustomIcon}
          onClose={() => setEditingService(null)}
        />
      )}

      {bulkEditOpen && (
        <ServiceBulkEditModal
          open={bulkEditOpen}
          affectedCount={bulkEditTargets.length}
          icons={iconIndex}
          customIcons={customIcons}
          busy={bulkSaving}
          onApply={handleApplyBulkEdits}
          onClose={() => setBulkEditOpen(false)}
        />
      )}

</>
    </RoutePerfBoundary>
  );
}
