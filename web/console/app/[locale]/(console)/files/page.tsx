"use client";

import { useTranslations } from "next-intl";
import { OperatorAccessRequired } from "../../../components/OperatorAccessRequired";
import { useAuth } from "../../../contexts/AuthContext";
import { canAccessHostData } from "../../../lib/roles";
import { FilesWorkspace } from "./FilesWorkspace";

export default function FilesPage() {
  const { user } = useAuth();
  const tNav = useTranslations("nav");
  // The Hub serves host file browsing and downloads to operators and above only.
  if (!canAccessHostData(user?.role)) {
    return <OperatorAccessRequired title={tNav("files")} feature="files" />;
  }
  return <FilesWorkspace />;
}
