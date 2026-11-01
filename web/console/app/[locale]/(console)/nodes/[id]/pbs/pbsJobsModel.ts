// Shapes and normalizers for the PBS job and config lists (verify, prune and sync jobs, traffic control rules and
// certificates). PBS answers in kebab-case; the hub sometimes answers in snake_case, so both are read.

type Raw = Record<string, unknown>;

function rows(value: unknown, key: string): Raw[] {
  if (!value || typeof value !== "object") return [];
  const list = (value as Raw)[key];
  if (!Array.isArray(list)) return [];
  return list.map((entry) => (entry && typeof entry === "object" ? entry : {}) as Raw);
}

const str = (value: unknown): string | undefined => (typeof value === "string" ? value : undefined);
const num = (value: unknown): number | undefined => (typeof value === "number" && Number.isFinite(value) ? value : undefined);
const id = (value: unknown): string => (typeof value === "string" ? value : String(value ?? ""));
const either = (row: Raw, snake: string, kebab: string): unknown => row[snake] ?? row[kebab];

export type PBSVerifyJob = {
  id: string;
  store: string;
  schedule?: string;
  comment?: string;
  ignore_verified?: boolean;
  outdated_after?: number;
};

export function normalizeVerifyJobs(value: unknown): PBSVerifyJob[] {
  return rows(value, "jobs").map((job) => ({
    id: id(job.id),
    store: str(job.store) ?? "",
    schedule: str(job.schedule),
    comment: str(job.comment),
    ignore_verified: either(job, "ignore_verified", "ignore-verified") === true,
    outdated_after: num(either(job, "outdated_after", "outdated-after")),
  }));
}

export type PBSPruneJob = {
  id: string;
  store: string;
  ns?: string;
  schedule?: string;
  comment?: string;
  keep_last?: number;
  keep_hourly?: number;
  keep_daily?: number;
  keep_weekly?: number;
  keep_monthly?: number;
  keep_yearly?: number;
};

export function normalizePruneJobs(value: unknown): PBSPruneJob[] {
  return rows(value, "jobs").map((job) => ({
    id: id(job.id),
    store: str(job.store) ?? "",
    ns: str(job.ns),
    schedule: str(job.schedule),
    comment: str(job.comment),
    keep_last: num(either(job, "keep_last", "keep-last")),
    keep_hourly: num(either(job, "keep_hourly", "keep-hourly")),
    keep_daily: num(either(job, "keep_daily", "keep-daily")),
    keep_weekly: num(either(job, "keep_weekly", "keep-weekly")),
    keep_monthly: num(either(job, "keep_monthly", "keep-monthly")),
    keep_yearly: num(either(job, "keep_yearly", "keep-yearly")),
  }));
}

export type RetentionKind = "last" | "hourly" | "daily" | "weekly" | "monthly" | "yearly";

/** The keep-* rules of a prune job in PBS order, for "Last 8, 14 daily, 8 weekly". */
export function retentionParts(job: PBSPruneJob): Array<{ kind: RetentionKind; count: number }> {
  const parts: Array<[RetentionKind, number | undefined]> = [
    ["last", job.keep_last],
    ["hourly", job.keep_hourly],
    ["daily", job.keep_daily],
    ["weekly", job.keep_weekly],
    ["monthly", job.keep_monthly],
    ["yearly", job.keep_yearly],
  ];
  return parts
    .filter((part): part is [RetentionKind, number] => typeof part[1] === "number")
    .map(([kind, count]) => ({ kind, count }));
}

export type PBSSyncJob = {
  id: string;
  store: string;
  remote?: string;
  remote_store?: string;
  schedule?: string;
  comment?: string;
  remove_vanished?: boolean;
  verified_only?: boolean;
  transfer_last?: number;
};

export function normalizeSyncJobs(value: unknown): PBSSyncJob[] {
  return rows(value, "jobs").map((job) => ({
    id: id(job.id),
    store: str(job.store) ?? "",
    remote: str(job.remote),
    remote_store: str(either(job, "remote_store", "remote-store")),
    schedule: str(job.schedule),
    comment: str(job.comment),
    remove_vanished: either(job, "remove_vanished", "remove-vanished") === true,
    verified_only: either(job, "verified_only", "verified-only") === true,
    transfer_last: num(either(job, "transfer_last", "transfer-last")),
  }));
}

export type PBSTrafficRule = {
  name: string;
  rate_in?: string;
  rate_out?: string;
  network?: string[];
  comment?: string;
};

export function normalizeTrafficRules(value: unknown): PBSTrafficRule[] {
  return rows(value, "rules").map((rule) => {
    const network = rule.network;
    return {
      name: id(rule.name),
      rate_in: str(either(rule, "rate_in", "rate-in")),
      rate_out: str(either(rule, "rate_out", "rate-out")),
      network: Array.isArray(network)
        ? network.filter((entry): entry is string => typeof entry === "string")
        : typeof network === "string"
          ? [network]
          : undefined,
      comment: str(rule.comment),
    };
  });
}

export type PBSCertificate = {
  filename: string;
  subject?: string;
  issuer?: string;
  san?: string[];
  not_after?: number;
  fingerprint?: string;
};

export function normalizeCertificates(value: unknown): PBSCertificate[] {
  return rows(value, "certificates").map((cert) => ({
    filename: id(cert.filename),
    subject: str(cert.subject),
    issuer: str(cert.issuer),
    san: Array.isArray(cert.san) ? cert.san.filter((entry): entry is string => typeof entry === "string") : undefined,
    not_after: num(cert.not_after) ?? num(cert.notafter),
    fingerprint: str(cert.fingerprint),
  }));
}

/** Expiry tone of a certificate: bad once expired or within 7 days, warn within 30. */
export function certificateTone(notAfterEpoch: number | undefined, now = Date.now()): "bad" | "warn" | "" {
  if (!notAfterEpoch) return "";
  const days = (notAfterEpoch * 1000 - now) / 86_400_000;
  if (days < 7) return "bad";
  if (days < 30) return "warn";
  return "";
}
