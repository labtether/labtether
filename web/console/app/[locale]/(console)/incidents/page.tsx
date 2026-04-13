"use client";

import { useState, useEffect } from "react";
import { useTranslations } from "next-intl";
import { ChevronLeft } from "lucide-react";
import { PageHeader } from "../../../components/PageHeader";
import { Badge } from "../../../components/ui/Badge";
import { Card } from "../../../components/ui/Card";
import { Button } from "../../../components/ui/Button";
import { SkeletonRow } from "../../../components/ui/Skeleton";
import { useIncidents, useIncidentDetail } from "../../../hooks/useIncidents";
import type { Incident } from "../../../console/models";
import { IncidentsListView, type IncidentStatusFilter } from "./IncidentsListView";
import { IncidentCockpitCard } from "./IncidentCockpitCard";
import { IncidentPostmortemCard } from "./IncidentPostmortemCard";
import { useIncidentPostmortemEditor } from "./useIncidentPostmortemEditor";

type IncidentView = "list" | "detail";

export default function IncidentsPage() {
  const t = useTranslations('incidents');
  const { incidents, loading, updateIncident } = useIncidents();
  const [view, setView] = useState<IncidentView>("list");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<IncidentStatusFilter>("all");
  const [noteText, setNoteText] = useState("");

  const {
    pmRootCause,
    pmActionItems,
    pmLessonsLearned,
    pmSaving,
    pmDirty,
    loadPostmortemFields,
    resetPmDirty,
    handleSavePostmortem,
    handleAddActionItem,
    handleRemoveActionItem,
    handleUpdateActionItem,
    handleRootCauseChange,
    handleLessonsLearnedChange,
  } = useIncidentPostmortemEditor({ updateIncident });

  const { incident: selectedIncident, events, loading: detailLoading, replaceIncident } = useIncidentDetail(
    view === "detail" ? selectedId : null
  );

  useEffect(() => {
    if (selectedIncident && !detailLoading) {
      loadPostmortemFields(selectedIncident);
    }
  }, [detailLoading, loadPostmortemFields, selectedIncident]);

  const handleSelectIncident = (incident: Incident) => {
    setSelectedId(incident.id);
    setView("detail");
    loadPostmortemFields(incident);
  };

  const handleBack = () => {
    setSelectedId(null);
    setView("list");
    resetPmDirty();
  };

  const handleStatusChange = async (id: string, newStatus: string) => {
    const updatedIncident = await updateIncident(id, { status: newStatus });
    replaceIncident(updatedIncident);
  };

  return (
    <>
      <PageHeader title={t('title')} subtitle={t('subtitle')} />

      {view === "list" ? (
        <IncidentsListView
          incidents={incidents}
          loading={loading}
          statusFilter={statusFilter}
          onStatusFilterChange={setStatusFilter}
          onSelectIncident={handleSelectIncident}
        />
      ) : null}

      {view === "detail" && (selectedIncident || detailLoading) ? (
        <>
          <Card className="flex items-center justify-between mb-4">
            <Button variant="ghost" onClick={handleBack}>
              <ChevronLeft size={14} />
              {t('back')}
            </Button>
            {selectedIncident ? (
              <div className="flex items-center gap-2 ml-auto">
                <Badge status={selectedIncident.severity} />
                <Badge status={selectedIncident.status} />
                {selectedIncident.status === "open" ? (
                  <Button size="sm" onClick={() => void handleStatusChange(selectedIncident.id, "investigating")}>
                    {t('actions.startInvestigating')}
                  </Button>
                ) : null}
                {selectedIncident.status === "investigating" ? (
                  <Button size="sm" onClick={() => void handleStatusChange(selectedIncident.id, "mitigated")}>
                    {t('actions.markMitigated')}
                  </Button>
                ) : null}
                {(selectedIncident.status === "mitigated" || selectedIncident.status === "investigating") ? (
                  <Button size="sm" onClick={() => void handleStatusChange(selectedIncident.id, "resolved")}>
                    {t('actions.markResolved')}
                  </Button>
                ) : null}
              </div>
            ) : null}
          </Card>

          {detailLoading ? (
            <Card className="mb-4">
              <div className="space-y-1">
                <SkeletonRow />
                <SkeletonRow />
                <SkeletonRow />
              </div>
            </Card>
          ) : selectedIncident ? (
            <>
              <IncidentCockpitCard incident={selectedIncident} events={events} />

              <Card className="mb-4">
                <h2>{t('notes.title')}</h2>
                <div className="space-y-3">
                  {selectedIncident.summary ? (
                    <p className="text-xs text-[var(--muted)]">{selectedIncident.summary}</p>
                  ) : (
                    <p className="text-xs text-[var(--muted)]">{t('notes.emptyHint')}</p>
                  )}
                  <textarea
                    className="w-full bg-transparent border border-[var(--line)] rounded-lg px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--muted)] focus:outline-none focus:border-[var(--muted)] transition-colors duration-150 resize-y"
                    placeholder={t('notes.placeholder')}
                    rows={3}
                    value={noteText}
                    onChange={(e) => setNoteText(e.target.value)}
                  />
                  <Button
                    disabled={!noteText.trim()}
                    onClick={() => {
                      void (async () => {
                        const updatedIncident = await updateIncident(selectedIncident.id, {
                          summary: (selectedIncident.summary ? selectedIncident.summary + "\n" : "") + noteText.trim()
                        });
                        replaceIncident(updatedIncident);
                        setNoteText("");
                      })();
                    }}
                  >
                    {t('notes.add')}
                  </Button>
                </div>
              </Card>

              {(selectedIncident.status === "resolved" || selectedIncident.status === "closed") ? (
                <IncidentPostmortemCard
                  incident={selectedIncident}
                  pmRootCause={pmRootCause}
                  pmActionItems={pmActionItems}
                  pmLessonsLearned={pmLessonsLearned}
                  pmSaving={pmSaving}
                  pmDirty={pmDirty}
                  onRootCauseChange={handleRootCauseChange}
                  onAddActionItem={handleAddActionItem}
                  onRemoveActionItem={handleRemoveActionItem}
                  onUpdateActionItem={handleUpdateActionItem}
                  onLessonsLearnedChange={handleLessonsLearnedChange}
                  onSave={async () => {
                    const updatedIncident = await handleSavePostmortem(selectedIncident.id);
                    replaceIncident(updatedIncident);
                  }}
                />
              ) : null}
            </>
          ) : null}
        </>
      ) : null}
    </>
  );
}
