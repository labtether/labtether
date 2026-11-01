import { Badge } from "./ui/Badge";

/** A status as a dot and a word. Same labels and tones as Badge. */
export function StatusBadge({ status }: { status: string }) {
  return <Badge status={status} />;
}
