"use client";

import { useRouter } from "../../../i18n/navigation";
import { ThemeProvider } from "../../contexts/ThemeContext";
import { AuthProvider } from "../../contexts/AuthContext";
import { StatusProvider, useFastStatus } from "../../contexts/StatusContext";
import { ToastProvider } from "../../contexts/ToastContext";
import { TipProvider } from "../../components/ui/Tip";
import { ConnectedAgentsProvider } from "../../contexts/ConnectedAgentsContext";
import { DesktopSessionProvider } from "../../contexts/DesktopSessionContext";
import { PaletteContextProvider, usePaletteRegister } from "../../contexts/PaletteContext";
import { useAuth } from "../../contexts/AuthContext";
import { createNavigationProvider } from "../../components/palette/providers/navigation";
import { createDevicesProvider } from "../../components/palette/providers/devices";
import { createDeviceActionsProvider } from "../../components/palette/providers/device-actions";
import { createSettingsProvider } from "../../components/palette/providers/settings";
import { createQuickConnectProvider } from "../../components/palette/providers/quick-connect";
import { createRecentProvider } from "../../components/palette/providers/recent";
import { createSnippetsProvider } from "../../components/palette/providers/snippets";
import { DemoBanner } from "../../components/DemoBanner";
import { Sidebar } from "../../components/Sidebar";
import { PageSectionProvider } from "../../components/shell/PageSectionProvider";
import { CommandPalette } from "../../components/CommandPalette";
import { MobileNavOverlay, useMobileNav } from "../../components/MobileNav";
import { TopBar } from "../../components/shell/TopBar";
import { usePresenceToasts } from "../../hooks/usePresenceToasts";
import { hasWriteRole } from "../../lib/roles";
import { useCallback, useEffect, useMemo, useRef } from "react";

function BuiltInPaletteProviders() {
  const router = useRouter();
  const { user } = useAuth();
  const canWrite = hasWriteRole(user?.role);
  const statusRef = useRef<ReturnType<typeof useFastStatus>>(null);
  const status = useFastStatus();
  statusRef.current = status;

  // Snippets: fetched once eagerly, cached in ref
  const snippetsRef = useRef<import("../../hooks/useTerminalSnippets").TerminalSnippet[]>([]);
  const snippetsFetchedRef = useRef(false);
  useEffect(() => {
    if (!canWrite) return;
    if (snippetsFetchedRef.current) return;
    fetch("/api/terminal/snippets", { cache: "no-store" })
      .then((res) => (res.ok ? res.json() : Promise.reject()))
      .then((data: unknown) => {
        snippetsFetchedRef.current = true;
        if (Array.isArray(data)) {
          snippetsRef.current = data;
        } else if (data && typeof data === "object" && "snippets" in data) {
          snippetsRef.current = (data as { snippets: typeof snippetsRef.current }).snippets ?? [];
        }
      })
      .catch(() => {});
  }, [canWrite]);

  const push = useCallback((href: string) => router.push(href), [router]);
  const getStatus = useCallback(() => statusRef.current, []);
  const getSnippets = useCallback(() => snippetsRef.current, []);

  const handleSnippetInsert = useCallback(
    (command: string) => {
      void navigator.clipboard.writeText(command);
      router.push("/terminal");
    },
    [router],
  );

  const handleDeviceSelect = useCallback(
    (href: string) => router.push(href),
    [router],
  );

  const navProvider = useMemo(() => createNavigationProvider(push, user?.role), [push, user?.role]);
  const devicesProvider = useMemo(() => createDevicesProvider(getStatus, handleDeviceSelect), [getStatus, handleDeviceSelect]);
  const actionsProvider = useMemo(() => createDeviceActionsProvider(getStatus, push, canWrite), [getStatus, push, canWrite]);
  const settingsProvider = useMemo(() => createSettingsProvider(push, user?.role), [push, user?.role]);
  const quickConnectProvider = useMemo(() => createQuickConnectProvider(push, canWrite), [push, canWrite]);
  const recentProvider = useMemo(() => createRecentProvider(push, canWrite), [push, canWrite]);
  const snippetsProvider = useMemo(() => createSnippetsProvider(getSnippets, handleSnippetInsert, canWrite), [getSnippets, handleSnippetInsert, canWrite]);

  usePaletteRegister(navProvider);
  usePaletteRegister(devicesProvider);
  usePaletteRegister(actionsProvider);
  usePaletteRegister(settingsProvider);
  usePaletteRegister(quickConnectProvider);
  usePaletteRegister(recentProvider);
  usePaletteRegister(snippetsProvider);

  return null;
}

function ConsoleShell({ children }: { children: React.ReactNode }) {
  const { open, toggle, close } = useMobileNav();
  usePresenceToasts();

  // 2.0 shell: the floating sidebar, then the top bar and the page column on the plain canvas.
  return (
    <div className="lt-shell">
      <Sidebar />
      <MobileNavOverlay open={open} onClose={close} />
      <main className="lt-content">
        <TopBar navOpen={open} onToggleNav={toggle} />
        <div className="lt-page">
          <PageSectionProvider>{children}</PageSectionProvider>
        </div>
      </main>
      <BuiltInPaletteProviders />
      <CommandPalette />
    </div>
  );
}

// Client half of the console layout. Runtime-only values such as demo mode
// are read on the server by ./layout.tsx and passed in as props.
export function ConsoleClientLayout({
  children,
  demoMode,
}: {
  children: React.ReactNode;
  demoMode: boolean;
}) {
  return (
    <>
      <DemoBanner demoMode={demoMode} />
      <ThemeProvider>
        <AuthProvider>
          <StatusProvider>
            <PaletteContextProvider>
              <ConnectedAgentsProvider>
                <DesktopSessionProvider>
                  <ToastProvider>
                    <TipProvider>
                      <ConsoleShell>{children}</ConsoleShell>
                    </TipProvider>
                  </ToastProvider>
                </DesktopSessionProvider>
              </ConnectedAgentsProvider>
            </PaletteContextProvider>
          </StatusProvider>
        </AuthProvider>
      </ThemeProvider>
    </>
  );
}
