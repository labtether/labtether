import { memo } from "react";
import type { WebService } from "../../../hooks/useWebServices";
import { ServiceCard } from "./ServiceCard";
import { serviceLayoutKey } from "./servicesPageHelpers";

interface ServicesGroupedGridProps {
  grouped: Array<[string, WebService[]]>;
  assetNameMap: Map<string, string>;
  expandedService: string | null;
  layoutMode: boolean;
  selectionMode: boolean;
  selectedServiceKeys: Set<string>;
  draggingServiceKey: string | null;
  dragOverServiceKey: string | null;
  onToggleExpand: (service: WebService) => void;
  onToggleSelect: (service: WebService) => void;
  onEdit: (service: WebService) => void;
  onRename: (service: WebService) => void;
  onToggleHidden: (service: WebService) => void;
  onDeleteManual: (service: WebService) => void;
  onDragStart: (service: WebService) => void;
  onDragOver: (service: WebService) => void;
  onDrop: (service: WebService) => void;
  onDragEnd: () => void;
  onFilterHost?: (hostAssetID: string) => void;
}

function ServicesGroupedGridComponent({
  grouped,
  assetNameMap,
  expandedService,
  layoutMode,
  selectionMode,
  selectedServiceKeys,
  draggingServiceKey,
  dragOverServiceKey,
  onToggleExpand,
  onToggleSelect,
  onEdit,
  onRename,
  onToggleHidden,
  onDeleteManual,
  onDragStart,
  onDragOver,
  onDrop,
  onDragEnd,
  onFilterHost,
}: ServicesGroupedGridProps) {
  return (
    <>
      {grouped.map(([category, categoryServices]) => (
        <section
          key={category}
          style={{
            contain: "layout paint style",
            contentVisibility: "auto",
            containIntrinsicSize: `${Math.max(categoryServices.length, 1) * 188}px 960px`,
          }}
        >
          <h2 className="text-xs font-semibold uppercase tracking-[0.06em] text-[var(--muted)] mb-2">
            {category}
            <span className="ml-1.5 text-[10px] font-normal">
              ({categoryServices.length})
            </span>
          </h2>
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3">
            {categoryServices.map((service) => {
              const key = serviceLayoutKey(service);
              return (
                <ServiceCard
                  key={key}
                  service={service}
                  hostName={assetNameMap.get(service.host_asset_id) ?? ""}
                  expanded={expandedService === service.id}
                  layoutMode={layoutMode}
                  selectionMode={selectionMode}
                  selected={selectedServiceKeys.has(key)}
                  dragging={draggingServiceKey === key}
                  dragOver={dragOverServiceKey === key}
                  onToggleExpand={onToggleExpand}
                  onToggleSelect={onToggleSelect}
                  onEdit={onEdit}
                  onRename={onRename}
                  onToggleHidden={onToggleHidden}
                  onDeleteManual={onDeleteManual}
                  onDragStart={onDragStart}
                  onDragOver={onDragOver}
                  onDrop={onDrop}
                  onDragEnd={onDragEnd}
                  onFilterHost={onFilterHost}
                />
              );
            })}
          </div>
        </section>
      ))}
    </>
  );
}

export const ServicesGroupedGrid = memo(ServicesGroupedGridComponent);
