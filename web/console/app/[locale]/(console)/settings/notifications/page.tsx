"use client";

import { useTranslations } from "next-intl";
import { PageHeader } from "../../../../components/PageHeader";
import { useAuth } from "../../../../contexts/AuthContext";
import { hasAdminRole } from "../../../../lib/roles";
import { NotificationChannelsCard } from "../components/NotificationChannelsCard";
import { Sheet } from "../components/SettingsKit";

export default function NotificationSettingsPage() {
  const t = useTranslations("notifications");
  const tSettings = useTranslations("settings");
  const { user } = useAuth();

  if (!hasAdminRole(user?.role)) {
    return (
      <>
        <PageHeader title={t("title")} />
        <Sheet pad>
          <p className="st-msg">{tSettings("general.adminOnly")}</p>
        </Sheet>
      </>
    );
  }

  return <NotificationChannelsCard />;
}
