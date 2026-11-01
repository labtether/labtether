"use client";

import { useParams } from "next/navigation";
import { IssueView } from "../IssueView";

/** One issue: /incidents/<id>. */
export default function IssuePage() {
  const params = useParams<{ id: string }>();
  const id = String(params?.id ?? "");
  return <IssueView key={id} id={id} />;
}
