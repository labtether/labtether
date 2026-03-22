"use client";

import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { Card } from "../../../components/ui/Card";
import {
  formatDiscoveryCollectedAt,
  type DiscoveryOverview,
} from "./servicesPageHelpers";

const STORAGE_KEY = "lt:services:discovery-collapsed";

function readCollapsed(): boolean {
  if (typeof window === "undefined") return false;
  return localStorage.getItem(STORAGE_KEY) === "1";
}

interface ServicesDiscoveryOverviewCardProps {
  discoveryOverview: DiscoveryOverview;
}

export function ServicesDiscoveryOverviewCard({
  discoveryOverview,
}: ServicesDiscoveryOverviewCardProps) {
  const [collapsed, setCollapsed] = useState(readCollapsed);

  if (discoveryOverview.hostCount <= 0) {
    return null;
  }

  function toggleCollapsed() {
    setCollapsed((prev) => {
      const next = !prev;
      localStorage.setItem(STORAGE_KEY, next ? "1" : "0");
      return next;
    });
  }

  return (
    <Card className="border-[var(--line)]/80">
      <button
        type="button"
        onClick={toggleCollapsed}
        className="w-full flex flex-wrap items-start justify-between gap-3 cursor-pointer text-left"
      >
        <div className="flex items-start gap-2">
          <span className="mt-0.5 text-[var(--muted)]">
            {collapsed ? <ChevronRight size={14} /> : <ChevronDown size={14} />}
          </span>
          <div>
            <h2 className="text-sm font-semibold text-[var(--text)]">
              Discovery Cycle Stats
            </h2>
            {!collapsed && (
              <p className="mt-1 text-xs text-[var(--muted)]">
                Live discovery-source counters and stage timings from the latest agent
                cycles.
              </p>
            )}
          </div>
        </div>
        <div className="text-xs text-[var(--muted)] text-right">
          {collapsed ? (
            <p>
              {discoveryOverview.hostCount} hosts · {discoveryOverview.averageDiscoveredServices} services/cycle
            </p>
          ) : (
            <>
              <p>Reporting hosts: {discoveryOverview.hostCount}</p>
              <p>Avg cycle duration: {discoveryOverview.averageCycleDurationMs}ms</p>
              <p>
                Avg services/cycle: {discoveryOverview.averageDiscoveredServices}
              </p>
              {discoveryOverview.latestCollectedAt ? (
                <p>
                  Latest cycle:{" "}
                  {formatDiscoveryCollectedAt(discoveryOverview.latestCollectedAt)}
                </p>
              ) : null}
            </>
          )}
        </div>
      </button>

      {!collapsed && (
      <>
      <div className="mt-3 grid grid-cols-1 gap-2 md:grid-cols-2 xl:grid-cols-4">
        {discoveryOverview.sources.map((source) => {
          const averageDurationMs =
            source.hostsReported > 0
              ? Math.round(source.totalDurationMs / source.hostsReported)
              : 0;
          return (
            <div
              key={source.key}
              className="rounded-lg border border-[var(--line)] bg-[var(--surface)] p-2.5"
              data-testid={`discovery-source-${source.key}`}
            >
              <p className="text-[12px] font-medium text-[var(--text)]">
                {source.label}
              </p>
              <p className="mt-1 text-xs text-[var(--muted)]">
                Enabled on {source.enabledHosts}/{discoveryOverview.hostCount}{" "}
                hosts
              </p>
              <p className="text-xs text-[var(--muted)]">
                Found per cycle: {source.servicesFound}
              </p>
              <p className="text-xs text-[var(--muted)]">
                Avg stage time: {averageDurationMs}ms
              </p>
            </div>
          );
        })}
      </div>

      <div className="mt-3 divide-y divide-[var(--line)] rounded-lg border border-[var(--line)] bg-[var(--surface)]">
        {discoveryOverview.hostRows.slice(0, 5).map((entry) => (
          <div
            key={entry.hostAssetID}
            className="flex flex-wrap items-center justify-between gap-2 px-3 py-2 text-xs"
          >
            <span className="font-medium text-[var(--text)]">
              {entry.hostLabel}
            </span>
            <span className="text-[var(--muted)]">
              {entry.totalServices} services · {entry.cycleDurationMs}ms ·{" "}
              {formatDiscoveryCollectedAt(entry.collectedAt)}
            </span>
          </div>
        ))}
      </div>
      </>
      )}
    </Card>
  );
}
