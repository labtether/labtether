import type { AgentTokenSummary, EnrollmentToken } from "../../../../console/models";

/**
 * A time as the 2.0 rows show it: "21:12" today, "Fri 14:20" within six days either side, else "3 Oct 2027"
 * ("3 Oct" in this year). Empty for a missing or unreadable time.
 */
export function formatWhen(iso: string | null | undefined, now: Date = new Date(), locale?: string): string {
  if (!iso) return "";
  const when = new Date(iso);
  if (Number.isNaN(when.valueOf())) return "";
  const time = when.toLocaleTimeString(locale, { hour: "2-digit", minute: "2-digit", hour12: false });
  if (when.toDateString() === now.toDateString()) return time;
  const days = Math.abs(when.getTime() - now.getTime()) / 86_400_000;
  if (days < 6) return `${when.toLocaleDateString(locale, { weekday: "short" })} ${time}`;
  return when.toLocaleDateString(locale, {
    day: "numeric",
    month: "short",
    ...(when.getFullYear() === now.getFullYear() ? {} : { year: "numeric" }),
  });
}

export type EnrollmentTokenState = "active" | "revoked" | "expired" | "usedUp";

/** Whether an enrollment token can still enroll an agent, and if not, why. */
export function enrollmentTokenState(token: EnrollmentToken, now: Date = new Date()): EnrollmentTokenState {
  if (token.revoked_at) return "revoked";
  if (new Date(token.expires_at).getTime() < now.getTime()) return "expired";
  if (token.max_uses > 0 && token.use_count >= token.max_uses) return "usedUp";
  return "active";
}

/**
 * Tokens the hub's cleanup deletes: enrollment tokens that can no longer enroll, and agent keys revoked before the
 * agent ever used them.
 */
export function deadTokenCount(enrollmentTokens: EnrollmentToken[], agentTokens: AgentTokenSummary[], now: Date = new Date()): number {
  const deadEnrollment = enrollmentTokens.filter((token) => enrollmentTokenState(token, now) !== "active").length;
  const deadAgent = agentTokens.filter((token) => token.status === "revoked" && !token.last_used_at).length;
  return deadEnrollment + deadAgent;
}
