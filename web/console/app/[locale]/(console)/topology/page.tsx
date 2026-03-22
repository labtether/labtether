"use client";

import dynamic from "next/dynamic";
import { useTranslations } from "next-intl";

const TopologyCanvasPage = dynamic(
  () => import("./TopologyCanvasPage"),
  {
    ssr: false,
    loading: () => (
      <div className="fixed inset-0 z-20 overflow-hidden md:left-52">
        <div className="flex h-full w-full items-center justify-center bg-[var(--surface)]/60 backdrop-blur-sm">
          <p className="text-sm text-[var(--muted)]">Loading topology canvas...</p>
        </div>
      </div>
    ),
  },
);

export default function TopologyPage() {
  const t = useTranslations('topology');

  return (
    <>
      <h1 className="sr-only">{t('title')}</h1>
      <TopologyCanvasPage />
    </>
  );
}
