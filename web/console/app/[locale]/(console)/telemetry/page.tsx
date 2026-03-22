"use client";

import { useEffect } from "react";
import { useRouter } from "../../../../i18n/navigation";

export default function TelemetryRedirect() {
  const router = useRouter();
  useEffect(() => {
    router.replace("/nodes");
  }, [router]);
  return null;
}
