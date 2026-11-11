"use client";

import { useTranslations } from "next-intl";
import { useAuth } from "../../../../contexts/AuthContext";
import { AccountSecurityCard } from "./AccountSecurityCard";

/**
 * Account security for the signed-in person. A single sign-on account has no LabTether password, and its sign-ins
 * never ask for a LabTether code, so it gets one line pointing to its identity provider instead of both forms.
 */
export function AccountSecuritySection() {
  const t = useTranslations("settings");
  const { user } = useAuth();
  const provider = user?.auth_provider ?? "local";
  if (provider === "local") return <AccountSecurityCard />;
  return (
    <section className="st st-sec st-acct" aria-labelledby="st-acct-h">
      <div className="st-sh">
        <h2 id="st-acct-h">{t("security.title")}</h2>
      </div>
      <div className="st-sheet pad">
        <p className="st-msg">{t("security.singleSignOn")}</p>
      </div>
    </section>
  );
}
