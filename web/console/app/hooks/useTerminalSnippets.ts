"use client";

import { useState, useEffect, useCallback, useRef } from "react";

export interface TerminalSnippet {
  id: string;
  name: string;
  command: string;
  description: string;
  scope: string;
  shortcut: string;
  sort_order: number;
}

export function useTerminalSnippets(scope?: string) {
  const [snippets, setSnippets] = useState<TerminalSnippet[]>([]);
  const [loading, setLoading] = useState(true);
  const abortRef = useRef<AbortController | null>(null);
  const mutationAbortRef = useRef<AbortController | null>(null);
  const mountedRef = useRef(true);

  const fetchSnippets = useCallback(async () => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    try {
      const url = scope
        ? `/api/terminal/snippets?scope=${encodeURIComponent(scope)}`
        : "/api/terminal/snippets";
      const res = await fetch(url, {
        cache: "no-store",
        signal: controller.signal,
      });
      if (res.ok) {
        const data = (await res.json()) as
          | { snippets?: TerminalSnippet[] }
          | TerminalSnippet[];
        if (Array.isArray(data)) {
          if (mountedRef.current) {
            setSnippets(data);
          }
        } else {
          if (mountedRef.current) {
            setSnippets(data.snippets ?? []);
          }
        }
      }
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      // Keep existing snippets on error
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null;
      }
      if (mountedRef.current) {
        setLoading(false);
      }
    }
  }, [scope]);

  useEffect(() => {
    mountedRef.current = true;
    void fetchSnippets();
    return () => {
      mountedRef.current = false;
      abortRef.current?.abort();
      abortRef.current = null;
      mutationAbortRef.current?.abort();
      mutationAbortRef.current = null;
    };
  }, [fetchSnippets]);

  const createSnippet = useCallback(
    async (data: Omit<TerminalSnippet, "id">) => {
      mutationAbortRef.current?.abort();
      const controller = new AbortController();
      mutationAbortRef.current = controller;
      try {
        const res = await fetch("/api/terminal/snippets", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(data),
          signal: controller.signal,
        });
        if (!res.ok) {
          throw new Error(await readSnippetError(res, "Failed to create snippet"));
        }
        await fetchSnippets();
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError") {
          return;
        }
        throw error;
      } finally {
        if (mutationAbortRef.current === controller) {
          mutationAbortRef.current = null;
        }
      }
    },
    [fetchSnippets]
  );

  const updateSnippet = useCallback(
    async (id: string, data: Partial<Omit<TerminalSnippet, "id">>) => {
      mutationAbortRef.current?.abort();
      const controller = new AbortController();
      mutationAbortRef.current = controller;
      try {
        const res = await fetch(`/api/terminal/snippets/${encodeURIComponent(id)}`, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(data),
          signal: controller.signal,
        });
        if (!res.ok) {
          throw new Error(await readSnippetError(res, "Failed to update snippet"));
        }
        await fetchSnippets();
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError") {
          return;
        }
        throw error;
      } finally {
        if (mutationAbortRef.current === controller) {
          mutationAbortRef.current = null;
        }
      }
    },
    [fetchSnippets]
  );

  const deleteSnippet = useCallback(
    async (id: string) => {
      mutationAbortRef.current?.abort();
      const controller = new AbortController();
      mutationAbortRef.current = controller;
      try {
        const res = await fetch(`/api/terminal/snippets/${encodeURIComponent(id)}`, {
          method: "DELETE",
          signal: controller.signal,
        });
        if (!res.ok) {
          throw new Error(await readSnippetError(res, "Failed to delete snippet"));
        }
        await fetchSnippets();
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError") {
          return;
        }
        throw error;
      } finally {
        if (mutationAbortRef.current === controller) {
          mutationAbortRef.current = null;
        }
      }
    },
    [fetchSnippets]
  );

  return { snippets, createSnippet, updateSnippet, deleteSnippet, loading };
}

async function readSnippetError(response: Response, fallback: string): Promise<string> {
  const body = (await response.json().catch(() => ({ error: "unknown error" }))) as {
    error?: string;
  };
  return body.error ?? fallback;
}
