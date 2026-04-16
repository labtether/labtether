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
import { CommandPalette } from "../../components/CommandPalette";
import { MobileNavToggle, MobileNavOverlay, useMobileNav } from "../../components/MobileNav";
import { usePresenceToasts } from "../../hooks/usePresenceToasts";
import { useCallback, useEffect, useMemo, useRef, type CSSProperties, type PointerEvent } from "react";

const GLOW_ORB_1_STYLE: CSSProperties = {
  background: 'radial-gradient(circle, rgba(var(--accent-rgb),0.06) 0%, transparent 70%)',
  animation: 'float-glow 20s ease-in-out infinite',
  willChange: 'transform',
};
const GLOW_ORB_2_STYLE: CSSProperties = {
  background: 'radial-gradient(circle, rgba(var(--accent-rgb),0.04) 0%, transparent 70%)',
  animation: 'float-glow 25s ease-in-out infinite reverse',
  willChange: 'transform',
};
const DOT_GRID_STYLE: CSSProperties = {
  backgroundImage: 'radial-gradient(circle, rgba(var(--accent-rgb),0.08) 1px, transparent 1px)',
  backgroundSize: '24px 24px',
  maskImage: 'radial-gradient(ellipse 60% 50% at 50% 50%, black 30%, transparent 100%)',
  WebkitMaskImage: 'radial-gradient(ellipse 60% 50% at 50% 50%, black 30%, transparent 100%)',
};
const CURSOR_GLOW_STYLE: CSSProperties = {
  left: 0,
  top: 0,
  width: 300,
  height: 300,
  borderRadius: '50%',
  background: 'radial-gradient(circle, rgba(var(--accent-rgb),0.04) 0%, transparent 70%)',
  transform: 'translate3d(-9999px, -9999px, 0)',
  transition: 'transform 120ms ease-out',
};

function BuiltInPaletteProviders() {
  const router = useRouter();
  const { user } = useAuth();
  const isAdmin = user?.role === "owner" || user?.role === "admin";
  const statusRef = useRef<ReturnType<typeof useFastStatus>>(null);
  const status = useFastStatus();
  statusRef.current = status;

  // Snippets: fetched once eagerly, cached in ref
  const snippetsRef = useRef<import("../../hooks/useTerminalSnippets").TerminalSnippet[]>([]);
  const snippetsFetchedRef = useRef(false);
  useEffect(() => {
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
  }, []);

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

  const navProvider = useMemo(() => createNavigationProvider(push, isAdmin), [push, isAdmin]);
  const devicesProvider = useMemo(() => createDevicesProvider(getStatus, handleDeviceSelect), [getStatus, handleDeviceSelect]);
  const actionsProvider = useMemo(() => createDeviceActionsProvider(getStatus, push), [getStatus, push]);
  const settingsProvider = useMemo(() => createSettingsProvider(push, isAdmin), [push, isAdmin]);
  const quickConnectProvider = useMemo(() => createQuickConnectProvider(push), [push]);
  const recentProvider = useMemo(() => createRecentProvider(push), [push]);
  const snippetsProvider = useMemo(() => createSnippetsProvider(getSnippets, handleSnippetInsert), [getSnippets, handleSnippetInsert]);

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
  const cursorGlowRef = useRef<HTMLDivElement | null>(null);
  const cursorFrameRef = useRef<number | null>(null);
  const pendingCursorRef = useRef<{ x: number; y: number } | null>(null);

  const flushCursorGlow = useCallback(() => {
    cursorFrameRef.current = null;
    const glow = cursorGlowRef.current;
    const cursor = pendingCursorRef.current;
    if (!glow || !cursor) {
      return;
    }
    glow.style.transform = `translate3d(${cursor.x - 150}px, ${cursor.y - 150}px, 0)`;
  }, []);

  const handlePointerMove = useCallback((event: PointerEvent<HTMLDivElement>) => {
    pendingCursorRef.current = { x: event.clientX, y: event.clientY };
    if (cursorFrameRef.current != null) {
      return;
    }
    cursorFrameRef.current = window.requestAnimationFrame(flushCursorGlow);
  }, [flushCursorGlow]);

  useEffect(() => {
    return () => {
      if (cursorFrameRef.current != null) {
        window.cancelAnimationFrame(cursorFrameRef.current);
      }
    };
  }, []);

  return (
    <div className="flex min-h-screen bg-[var(--bg)] text-[var(--text)] relative" onPointerMove={handlePointerMove}>
      {/* Subtle noise texture for surface depth */}
      <div className="noise-texture fixed inset-0 pointer-events-none z-0" aria-hidden="true" />
      <Sidebar />
      <MobileNavToggle onToggle={toggle} />
      <MobileNavOverlay open={open} onClose={close} />
      {/* Ambient glow layer */}
      <div className="fixed inset-0 pointer-events-none z-0" aria-hidden="true">
        <div className="absolute -top-[20%] -left-[10%] w-[60%] h-[60%] rounded-full" style={GLOW_ORB_1_STYLE} />
        <div className="absolute -bottom-[20%] -right-[10%] w-[50%] h-[50%] rounded-full" style={GLOW_ORB_2_STYLE} />
      </div>
      <main className="relative z-10 w-full md:ml-52 md:w-[calc(100%-13rem)] p-6 lg:p-8">
        {/* Dot grid */}
        <div className="fixed inset-0 pointer-events-none z-0" aria-hidden="true" style={DOT_GRID_STYLE} />
        {/* Cursor glow */}
        <div
          ref={cursorGlowRef}
          className="fixed pointer-events-none z-0 will-change-transform"
          aria-hidden="true"
          style={CURSOR_GLOW_STYLE}
        />
        <div className="mx-auto w-full max-w-5xl">
          {children}
        </div>
      </main>
      <BuiltInPaletteProviders />
      <CommandPalette />
    </div>
  );
}

export default function ConsoleLayout({ children }: { children: React.ReactNode }) {
  return (
    <>
      <DemoBanner demoMode={process.env.NEXT_PUBLIC_DEMO_MODE === "true" || process.env.LABTETHER_DEMO_MODE === "true"} />
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
