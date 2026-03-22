"use client";

import { Globe, RefreshCw } from "lucide-react";
import { Card } from "../../../components/ui/Card";
import { EmptyState } from "../../../components/ui/EmptyState";

interface ServicesResultStatesProps {
  loading: boolean;
  servicesCount: number;
  error: string | null;
  filteredCount: number;
}

export function ServicesResultStates({
  loading,
  servicesCount,
  error,
  filteredCount,
}: ServicesResultStatesProps) {
  return (
    <>
      {loading && servicesCount === 0 && (
        <Card>
          <div className="flex items-center justify-center gap-2 py-12">
            <RefreshCw size={16} className="text-[var(--muted)] animate-spin" />
            <span className="text-sm text-[var(--muted)]">
              Discovering services...
            </span>
          </div>
        </Card>
      )}

      {error && (
        <Card className="border-[var(--bad)]/30">
          <div className="flex items-start gap-3">
            <span className="text-[var(--bad)] text-lg flex-shrink-0">
              &#x26A0;
            </span>
            <div className="flex-1 min-w-0">
              <p className="text-sm text-[var(--bad)]">{error}</p>
            </div>
          </div>
        </Card>
      )}

      {!loading && !error && servicesCount === 0 && (
        <Card>
          <EmptyState
            icon={Globe}
            title="No Services Discovered"
            description="Services will appear here once agents detect running web applications on your nodes."
          />
        </Card>
      )}

      {!loading && servicesCount > 0 && filteredCount === 0 && (
        <Card>
          <EmptyState
            icon={Globe}
            title="No Matching Services"
            description="No services match the current filters. Try adjusting your selection."
          />
        </Card>
      )}
    </>
  );
}
