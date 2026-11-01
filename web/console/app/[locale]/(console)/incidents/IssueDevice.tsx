import { useTranslations } from "next-intl";
import { useFastStatus } from "../../../contexts/StatusContext";
import { DeviceLine } from "./DeviceLine";

/** The device the issue is about, with its live status from the hub. */
export function IssueDevice({ id }: { id: string }) {
  const t = useTranslations("incidents.device");
  const status = useFastStatus();
  const asset = status?.assets?.find((a) => a.id === id);
  return (
    <section className="iu-sec" aria-labelledby="iu-dev-h">
      <div className="iu-sh"><h2 id="iu-dev-h">{t("title")}</h2></div>
      <div className="iu-list iu-hang">
        <DeviceLine id={id} asset={asset} />
      </div>
    </section>
  );
}
