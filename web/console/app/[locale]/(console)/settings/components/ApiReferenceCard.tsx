"use client";

import { useTranslations } from "next-intl";
import { Link } from "../../../../../i18n/navigation";
import { Facts, Section, Sheet } from "./SettingsKit";
import { useHubUrl } from "./useHubUrl";

/** Where the REST API lives and how to sign in to it; the full reference is the API docs page. */
export function ApiReferenceCard() {
  const t = useTranslations("settings");
  const { hubUrl } = useHubUrl();

  return (
    <Section
      title={t("apiReference.heading")}
      id="api-reference"
      action={<Link href="/api-docs" className="st-link">{t("apiReference.viewDocs")}</Link>}
    >
      <Sheet>
        <Facts
          items={[
            [t("apiReference.baseUrl"), <code key="b" className="st-mono">{hubUrl ? `${hubUrl}/api/v2` : "/api/v2"}</code>],
            [t("apiReference.auth"), <code key="a" className="st-mono">Authorization: Bearer &lt;API key&gt;</code>],
          ]}
        />
      </Sheet>
    </Section>
  );
}
