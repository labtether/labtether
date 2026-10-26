import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const navigation = vi.hoisted(() => ({ router: { push: vi.fn() } }));
vi.mock("../../../i18n/navigation", () => ({ useRouter: () => navigation.router }));

import { AuthProvider, useAuth } from "../AuthContext";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}

function json(value: unknown, status = 200): Response {
  return new Response(JSON.stringify(value), { status });
}

function deferredBody(body: Promise<unknown>): Response {
  return { ok: true, status: 200, json: () => body } as Response;
}

const signedIn = { id: "owner", username: "Owner", role: "admin" };
let container: HTMLDivElement;
let root: Root;
let current: ReturnType<typeof useAuth>;
let fetchMock: ReturnType<typeof vi.fn>;

function Consumer() {
  current = useAuth();
  return <span>{current.user?.username ?? "Signed out"}</span>;
}

async function render() {
  await act(async () => {
    root.render(<AuthProvider><Consumer /></AuthProvider>);
  });
}

beforeEach(() => {
  navigation.router = { push: vi.fn() };
  fetchMock = vi.fn(async () => json({ user: signedIn }));
  vi.stubGlobal("fetch", fetchMock);
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.unstubAllGlobals();
});

describe("AuthProvider request ownership", () => {
  it("keeps logout in force when an earlier user refresh finishes parsing", async () => {
    await render();
    expect(current.user).toEqual(signedIn);
    const oldBody = deferred<unknown>();
    const logoutResponse = deferred<Response>();
    fetchMock.mockImplementation(async (url: string) => url === "/api/auth/logout"
      ? logoutResponse.promise : deferredBody(oldBody.promise));
    let refresh!: Promise<void>;
    await act(async () => { refresh = current.refreshUser(); });
    let logout!: Promise<void>;
    await act(async () => { logout = current.logout(); });
    expect(current.user).toBeNull();

    await act(async () => {
      oldBody.resolve({ user: signedIn });
      await refresh;
      await current.refreshUser();
    });
    expect(current.user).toBeNull();
    expect(fetchMock).toHaveBeenCalledTimes(3);
    await act(async () => {
      logoutResponse.resolve(json({}));
      await logout;
    });
    expect(container.textContent).toBe("Signed out");
    expect(navigation.router.push).toHaveBeenCalledWith("/login");
  });

  it("discards an earlier bootstrap body after a newer auth check finishes", async () => {
    const oldBody = deferred<unknown>();
    fetchMock.mockResolvedValueOnce(deferredBody(oldBody.promise));
    await render();
    navigation.router = { push: vi.fn() };
    await render();
    expect(current.user).toEqual(signedIn);

    await act(async () => { oldBody.resolve({ user: { id: "old", username: "Old session" } }); });
    expect(current.user).toEqual(signedIn);
    expect(container.textContent).toBe("Owner");
  });

  it.each([true, false])("discards a delayed setup decision after the provider leaves (setup=%s)", async (setupRequired) => {
    const setupBody = deferred<unknown>();
    fetchMock.mockImplementation(async (url: string) => url === "/api/auth/me"
      ? json({}, 401) : deferredBody(setupBody.promise));
    await render();
    expect(fetchMock).toHaveBeenCalledWith("/api/auth/bootstrap/status", { cache: "no-store" });
    await act(async () => { root.render(null); });
    await act(async () => { setupBody.resolve({ setup_required: setupRequired }); });
    expect(navigation.router.push).not.toHaveBeenCalled();
  });
});
