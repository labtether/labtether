// Demo mode is switched on per deployment (docker-compose.demo.yml sets it at
// container start), so it must be read on the server at request time.
// The keys are looked up dynamically on purpose: Next.js inlines literal
// process.env.NEXT_PUBLIC_* reads at build time, which would freeze the value
// of the image build instead of the running container.
const DEMO_MODE_ENV_KEYS = ["LABTETHER_DEMO_MODE", "NEXT_PUBLIC_DEMO_MODE"] as const;

export function isDemoModeEnabled(
  env: Record<string, string | undefined> = process.env,
): boolean {
  return DEMO_MODE_ENV_KEYS.some((key) => env[key]?.trim() === "true");
}
