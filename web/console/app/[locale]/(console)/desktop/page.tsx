"use client";

import { useEffect } from "react";
import { useRouter } from "../../../../i18n/navigation";

/** The old Desktop page: remote screens live in Remote View now. */
export default function DesktopRedirect() {
  const router = useRouter();
  useEffect(() => {
    router.replace("/remote-view");
  }, [router]);
  return null;
}
