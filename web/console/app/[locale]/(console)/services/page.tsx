"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { PageHeader } from "../../../components/PageHeader";
import { RoutePerfBoundary } from "../../../components/RoutePerfBoundary";
import { ServiceEditModal } from "../../../components/ServiceEditModal";
import { ServiceBulkEditModal } from "../../../components/ServiceBulkEditModal";
import { ConfirmDialog } from "../../../components/ui/ConfirmDialog";
import { useWebServices, type WebService } from "../../../hooks/useWebServices";
import { useStatusAssetNameMap } from "../../../contexts/StatusContext";
import { useToast } from "../../../contexts/ToastContext";
import { reportRoutePerfMetric } from "../../../hooks/useRoutePerfTelemetry";
import { ServiceAddForm } from "./ServiceAddForm";
import { ServiceList } from "./ServiceList";
import type { ServiceRowActions } from "./ServiceRow";
import { ServicesBulkBar, ServicesEmpty, ServicesSkeleton } from "./ServicesStates";
import { ServicesHeaderActions } from "./ServicesHeaderActions";
import { ServicesToolbar } from "./ServicesToolbar";
import { serviceLayoutKey } from "./servicesLayoutHelpers";
import { isHidden, serviceAddress, type Notify } from "./servicesListModel";
import { useServiceList } from "./useServiceList";
import { useServiceEditModalActions } from "./useServiceEditModalActions";
import { usePullImagesAction } from "./usePullImagesAction";
import { useServiceBulkEditAction } from "./useServiceBulkEditAction";
import { useServiceDetailsCache } from "./useServiceDetailsCache";
import { useServiceIconLibrary } from "./useServiceIconLibrary";
import { useServiceLayoutOrchestration } from "./useServiceLayoutOrchestration";
import { useServiceMutationActions } from "./useServiceMutationActions";
import { useServiceSelectionState } from "./useServiceSelectionState";
import { useServicesView } from "./useServicesView";
import { useGroupingSuggestionToasts } from "./useGroupingSuggestionToasts";

export default function ServicesPage() {
  const t = useTranslations("services");
  const { addToast } = useToast();
  const notify = useCallback<Notify>((type, message) => addToast(type, message), [addToast]);
  const assetNameMap = useStatusAssetNameMap();
  const web = useWebServices({ includeHidden: true, detailLevel: "compact" });
  const { services, loading, syncing, error, refresh, sync } = web;
  const view = useServicesView();
  const icons = useServiceIconLibrary(web);
  const layout = useServiceLayoutOrchestration({ services });
  const details = useServiceDetailsCache(services, web.loadServiceDetails);
  const list = useServiceList({ services, view, assetNameMap, arranging: layout.layoutMode, arranged: layout.layoutOrderByCategory });
  const selection = useServiceSelectionState(list.visible);
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<WebService | null>(null);
  const [bulkTargets, setBulkTargets] = useState<WebService[] | null>(null);
  const [removing, setRemoving] = useState<WebService | null>(null);
  const [askPull, setAskPull] = useState(false);

  useGroupingSuggestionToasts(web.suggestions, refresh);
  const mutations = useServiceMutationActions({ ...web, refresh, notify });
  const { toggleHidden } = mutations;
  const bulk = useServiceBulkEditAction({ ...web, refresh, notify });
  const unhidden = useMemo(() => services.filter((s) => !isHidden(s)), [services]);
  const pull = usePullImagesAction({ services: unhidden, hostFilter: view.filters.host, assetNameMap, refresh, notify });
  const editActions = useServiceEditModalActions({ editingService: editing, saveServiceOverride: web.saveServiceOverride, refresh });
  const picked = useMemo(() => list.visible.filter((s) => selection.selected.has(serviceLayoutKey(s))), [list.visible, selection.selected]);

  const refreshAll = useCallback(async () => {
    try {
      await sync(view.filters.host !== "all" ? view.filters.host : undefined);
    } catch {
      // The hook keeps the error; the list shows it.
    }
    await refresh();
  }, [refresh, sync, view.filters.host]);

  const { toggleDetails, withDetails } = details;
  const { handleCardDragStart, handleCardDragOver, handleCardDrop, clearDragState } = layout;
  const rowActions = useMemo<ServiceRowActions>(() => ({
    onSelect: selection.toggle,
    onToggleDetails: toggleDetails,
    onEdit: (service) => setEditing(withDetails(service)),
    onToggleHidden: (service) => void toggleHidden(service),
    onRemove: setRemoving,
    onCopy: (service) => {
      void navigator.clipboard?.writeText(service.url).then(
        () => notify("success", t("toasts.copied", { address: serviceAddress(service.url) })),
        () => notify("error", t("toasts.copyFailed")),
      );
    },
    onDragStart: handleCardDragStart,
    onDragOver: handleCardDragOver,
    onDrop: handleCardDrop,
    onDragEnd: clearDragState,
  }), [clearDragState, handleCardDragOver, handleCardDragStart, handleCardDrop, notify, selection.toggle, t, toggleDetails, toggleHidden, withDetails]);

  useEffect(() => {
    reportRoutePerfMetric({
      route: "services",
      metric: "compute.derived_services",
      durationMs: list.deriveDurationMs,
      sampleSize: list.visible.length,
      metadata: {
        categories: list.categories.length,
        grouped_sections: list.groups.length,
        host_filtered: view.filters.host !== "all",
        status_filtered: view.filters.status !== "all",
        source_filtered: view.filters.source !== "all",
        health_filtered: view.filters.health !== "all",
        hidden_shown: view.filters.show === "hidden",
        selection_mode: selection.selected.size > 0,
      },
    });
  }, [list, selection.selected.size, view.filters]);

  const toggleArrange = useCallback(() => {
    layout.setLayoutMode((on) => !on);
    selection.clear();
    details.setOpenID(null);
  }, [details, layout, selection]);

  const hostLabel = view.filters.host === "all" ? t("pull.allDevices") : (assetNameMap.get(view.filters.host) ?? view.filters.host);
  const showList = services.length > 0 || (!loading && error);

  return (
    <RoutePerfBoundary
      route="services"
      sampleSize={list.visible.length}
      metadata={{ total_services: services.length, filtered_services: list.visible.length, grouped_sections: list.groups.length, hosts: list.hosts.length, suggestions: web.suggestions.length, selection_mode: selection.selected.size > 0 }}
    >
      <PageHeader
        title={t("title")}
        action={
          <ServicesHeaderActions
            adding={adding}
            arranging={layout.layoutMode}
            editCount={list.visible.length}
            bulkSaving={bulk.bulkSaving}
            syncing={syncing}
            pullingImages={pull.pullingImages}
            pullCount={pull.pullPlan.items.length}
            onAdd={() => setAdding((on) => !on)}
            onEditShown={() => setBulkTargets(list.visible)}
            onToggleArrange={toggleArrange}
            onRefresh={refreshAll}
            onPullImages={() => setAskPull(true)}
          />
        }
      />

      {adding && (
        <ServiceAddForm
          devices={list.devices}
          defaultDevice={view.filters.host !== "all" ? view.filters.host : ""}
          onAdd={mutations.addManual}
          onClose={() => setAdding(false)}
        />
      )}

      {layout.layoutMode ? (
        <div className="sv-arr" role="status">
          <b>{t("arrange.title")}</b>
          <span>{t("arrange.hint")}</span>
          <span className="lt-sp" />
          <button type="button" className="sv-chip" onClick={layout.resetLayoutOrder}>{t("arrange.reset")}</button>
          <button type="button" className="sv-chip" onClick={toggleArrange}>{t("arrange.done")}</button>
        </div>
      ) : services.length > 0 ? (
        <ServicesToolbar
          filters={view.filters}
          onFilters={view.patchFilters}
          counts={list.counts}
          categories={list.categories}
          hosts={list.hosts}
          sort={view.sort}
          onSortKey={view.setSortKey}
          grouping={view.grouping}
          onGrouping={view.setGrouping}
        />
      ) : null}

      {error && (
        <div className="sv-error" role="alert">
          <span>{services.length > 0 ? t("list.updateFailed") : t("list.loadFailed")}</span>
          <button type="button" className="sv-chip" onClick={() => void refresh()}>{t("list.retry")}</button>
        </div>
      )}

      {loading && services.length === 0 ? (
        <ServicesSkeleton />
      ) : !showList ? (
        <ServicesEmpty adding={adding} onAdd={() => setAdding(true)} />
      ) : (
        <ServiceList
          groups={list.groups}
          grouping={list.grouping}
          hostNames={assetNameMap}
          shown={list.visible.length}
          total={view.filters.show === "hidden" ? list.counts.hidden : list.counts.all}
          filtering={list.filtering}
          emptyText={list.emptyText}
          sort={view.sort}
          onSort={view.sortBy}
          selected={selection.selected}
          onSelectAll={selection.setAll}
          openID={details.openID}
          withDetails={details.withDetails}
          arranging={layout.layoutMode}
          dragKey={layout.draggingServiceKey}
          dragOverKey={layout.dragOverServiceKey}
          onClearFilters={view.clearFilters}
          actions={rowActions}
        />
      )}

      <ServicesBulkBar
        picked={picked}
        busy={bulk.bulkSaving}
        onEdit={() => setBulkTargets(picked)}
        onHide={(hidden) => void bulk.applyBulkEdits(picked, { iconMode: "no_change", hidden }).then(selection.clear)}
        onClear={selection.clear}
      />

      {editing && (
        <ServiceEditModal
          service={editing}
          icons={icons.iconIndex}
          customIcons={icons.customIcons}
          onSave={editActions.handleSaveEditModal}
          onReset={editActions.handleResetEditModal}
          onAddAltURL={editActions.handleAddAltURLEditModal}
          onRemoveAltURL={editActions.handleRemoveAltURLEditModal}
          onCreateCustomIcon={icons.createCustomIcon}
          onDeleteCustomIcon={icons.deleteCustomIcon}
          onRenameCustomIcon={icons.renameCustomIcon}
          onClose={() => setEditing(null)}
        />
      )}

      {bulkTargets && (
        <ServiceBulkEditModal
          open
          affectedCount={bulkTargets.length}
          icons={icons.iconIndex}
          customIcons={icons.customIcons}
          busy={bulk.bulkSaving}
          onApply={async (changes) => {
            await bulk.applyBulkEdits(bulkTargets, changes);
            selection.clear();
          }}
          onClose={() => setBulkTargets(null)}
        />
      )}

      <ConfirmDialog
        open={removing !== null}
        onClose={() => setRemoving(null)}
        onConfirm={() => {
          if (removing) void mutations.removeManual(removing);
          setRemoving(null);
        }}
        title={t("remove.title", { name: removing?.name ?? "" })}
        message={t("remove.body", { address: removing ? serviceAddress(removing.url) : "" })}
        confirmLabel={t("remove.confirm")}
        danger
      />

      <ConfirmDialog
        open={askPull}
        onClose={() => setAskPull(false)}
        onConfirm={() => {
          setAskPull(false);
          void pull.pullImages();
        }}
        title={t("pull.title", { count: pull.pullPlan.items.length })}
        message={t("pull.body", { scope: hostLabel })}
        confirmLabel={t("pull.confirm")}
      />
    </RoutePerfBoundary>
  );
}
