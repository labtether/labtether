"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";

import { Card } from "../../../components/ui/Card";

interface Recording {
  id: string;
  session_id: string;
  asset_id: string;
  protocol: string;
  file_size: number;
  duration_ms: number;
  status: string;
  created_at: string;
}

export default function RecordingsPage() {
  const t = useTranslations('recordings');
  const [recordings, setRecordings] = useState<Recording[]>([]);

  useEffect(() => {
    fetch("/api/recordings", { cache: "no-store" })
      .then((response) => response.json())
      .then((payload: { recordings?: Recording[] }) => {
        setRecordings(Array.isArray(payload.recordings) ? payload.recordings : []);
      })
      .catch(() => {
        setRecordings([]);
      });
  }, []);

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold text-[var(--text)]">{t('title')}</h2>
      <Card>
        {recordings.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">{t('empty')}</p>
        ) : (
          <div className="overflow-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[var(--muted)] border-b border-[var(--line)]">
                  <th className="py-2 pr-4">{t('columns.asset')}</th>
                  <th className="py-2 pr-4">{t('columns.protocol')}</th>
                  <th className="py-2 pr-4">{t('columns.duration')}</th>
                  <th className="py-2 pr-4">{t('columns.size')}</th>
                  <th className="py-2 pr-4">{t('columns.status')}</th>
                  <th className="py-2">{t('columns.date')}</th>
                </tr>
              </thead>
              <tbody>
                {recordings.map((recording) => (
                  <tr key={recording.id} className="border-b border-[var(--line)]/40">
                    <td className="py-2 pr-4 text-[var(--text)]">{recording.asset_id}</td>
                    <td className="py-2 pr-4 text-[var(--text)] uppercase">{recording.protocol}</td>
                    <td className="py-2 pr-4 text-[var(--text)]">{Math.round(recording.duration_ms / 1000)}s</td>
                    <td className="py-2 pr-4 text-[var(--text)]">{Math.round(recording.file_size / 1024)} KB</td>
                    <td className="py-2 pr-4 text-[var(--text)]">{recording.status}</td>
                    <td className="py-2 text-[var(--text)]">{new Date(recording.created_at).toLocaleString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  );
}
