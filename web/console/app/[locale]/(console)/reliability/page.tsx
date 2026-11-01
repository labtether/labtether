"use client";

import { useRef, useState } from "react";
import { Clock } from "lucide-react";
import { useTranslations } from "next-intl";
import { PageHeader } from "../../../components/PageHeader";
import { Button } from "../../../components/ui/Button";
import type { MaintenanceWindow } from "../../../console/models";
import { useFastStatus, useStatusControls } from "../../../contexts/StatusContext";
import { useToast } from "../../../contexts/ToastContext";
import { useReliability } from "../../../hooks/useReliability";
import { groupPath } from "../groups/groupListModel";
import { DowntimeSection } from "./DowntimeSection";
import { EndDowntimeDialog, ScheduleDowntimeDialog } from "./DowntimeDialogs";
import { EventsSection } from "./EventsSection";
import { FailedJobsSection } from "./FailedJobsSection";
import { GroupHealthSection } from "./GroupHealthSection";
import { HubSection } from "./HubSection";
import { useDowntimeWindows } from "./useDowntimeWindows";

/**
 * Settings › Health: the hub's own parts and checks, downtime windows for every group, each group's health score,
 * one group's events, and failed background jobs. Schedule downtime is the page's main button.
 */
export default function ReliabilityPage() {
  const t = useTranslations("reliability");
  const fast = useFastStatus();
  const { fetchStatus } = useStatusControls();
  const { addToast } = useToast();
  const r = useReliability();
  const downtime = useDowntimeWindows(r.groupRows);
  const [scheduling, setScheduling] = useState(false);
  const [ending, setEnding] = useState<MaintenanceWindow | null>(null);
  const eventsRef = useRef<HTMLElement>(null);

  // Scores and the timeline count downtime too; a failed refresh here is retried by the next status poll.
  const afterChange = async () => {
    await Promise.all([fetchStatus(), r.reloadGroupTimeline()]).catch(() => undefined);
  };

  const pick = (groupId: string) => {
    r.setSelectedTimelineGroup(groupId);
    eventsRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
  };

  return (
    <>
      <PageHeader
        title={t("title")}
        action={r.groupRows.length > 0 ? (
          <Button variant="primary" onClick={() => setScheduling(true)}>
            <Clock size={14} aria-hidden="true" />
            {t("downtime.schedule")}
          </Button>
        ) : undefined}
      />

      <HubSection
        endpoints={fast?.endpoints ?? []}
        checkedAt={fast?.timestamp}
        analytics={r.deadLetterAnalytics}
        deadLetters={r.deadLetters}
        retentionError={r.retentionError}
      />

      <DowntimeSection
        windows={downtime.windows}
        groups={r.groupRows}
        loading={downtime.loading}
        partial={downtime.partial}
        onEnd={setEnding}
      />

      <GroupHealthSection rows={r.groupReliabilityRows} groups={r.groupRows} selected={r.selectedTimelineGroup} onPick={pick} />

      <EventsSection
        ref={eventsRef}
        groups={r.groupRows}
        groupId={r.selectedTimelineGroup}
        onGroup={r.setSelectedTimelineGroup}
        range={r.groupTimelineWindow}
        onRange={r.setGroupTimelineWindow}
        timeline={r.groupTimeline}
        loading={r.groupTimelineLoading}
        failed={Boolean(r.groupTimelineError)}
      />

      <FailedJobsSection deadLetters={r.deadLetters} analytics={r.deadLetterAnalytics} />

      <ScheduleDowntimeDialog
        open={scheduling}
        groups={r.groupRows}
        groupId={r.selectedTimelineGroup || r.groupRows[0]?.id || ""}
        onClose={() => setScheduling(false)}
        onSchedule={async (groupId, body) => {
          await downtime.create(groupId, body);
          setScheduling(false);
          addToast("success", t("toast.scheduled", { name: groupPath(r.groupRows, groupId) }));
          await afterChange();
        }}
      />
      <EndDowntimeDialog
        target={ending}
        groupName={ending ? groupPath(r.groupRows, ending.group_id) : ""}
        onClose={() => setEnding(null)}
        onEnd={async (target) => {
          await downtime.remove(target);
          setEnding(null);
          addToast("success", t("toast.ended", { name: groupPath(r.groupRows, target.group_id) }));
          await afterChange();
        }}
      />
    </>
  );
}
