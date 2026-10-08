"use client";

import { URLGroupingCard } from "./URLGroupingCard";
import { ServiceMergingCard } from "./ServiceMergingCard";

// =========================================================================
// Exported Tab Component
// =========================================================================

export default function GroupingMergeTab() {
  return (
    <div className="space-y-4">
      <URLGroupingCard />
      <ServiceMergingCard />
    </div>
  );
}
