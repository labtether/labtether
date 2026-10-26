import { connection } from "next/server";
import { isDemoModeEnabled } from "../../lib/demoMode";
import { ConsoleClientLayout } from "./ConsoleClientLayout";

export default async function ConsoleLayout({ children }: { children: React.ReactNode }) {
  // Render per request so the demo flag reflects the running container's env.
  await connection();
  return <ConsoleClientLayout demoMode={isDemoModeEnabled()}>{children}</ConsoleClientLayout>;
}
