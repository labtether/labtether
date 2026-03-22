"use client";

import { useCallback, useEffect, useState } from "react";
import { sanitizeErrorMessage } from "../lib/sanitizeErrorMessage";

export type HubUser = {
  id: string;
  username: string;
  role: string;
  auth_provider: string;
  totp_enabled?: boolean;
  created_at?: string;
};

type UsersPayload = {
  users?: HubUser[];
  error?: string;
};

type UserPayload = {
  error?: string;
};

export function useHubUsers() {
  const [users, setUsers] = useState<HubUser[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const refresh = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const response = await fetch("/api/auth/users", { cache: "no-store", signal: AbortSignal.timeout(15_000) });
      const payload = (await response.json()) as UsersPayload;
      if (!response.ok) {
        throw new Error(payload.error || `failed to load users (${response.status})`);
      }
      setUsers(payload.users ?? []);
    } catch (err) {
      setError(sanitizeErrorMessage(err instanceof Error ? err.message : "", "failed to load users"));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const createUser = useCallback(async (payload: Record<string, unknown>) => {
    const response = await fetch("/api/auth/users", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      signal: AbortSignal.timeout(15_000),
      body: JSON.stringify(payload),
    });
    const data = (await response.json()) as UserPayload;
    if (!response.ok) {
      throw new Error(data.error || `failed to create user (${response.status})`);
    }
    await refresh();
  }, [refresh]);

  const updateRole = useCallback(async (id: string, role: string) => {
    const response = await fetch(`/api/auth/users/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      signal: AbortSignal.timeout(15_000),
      body: JSON.stringify({ role }),
    });
    const data = (await response.json()) as UserPayload;
    if (!response.ok) {
      throw new Error(data.error || `failed to update user role (${response.status})`);
    }
    await refresh();
  }, [refresh]);

  const resetPassword = useCallback(async (id: string, password: string) => {
    const response = await fetch(`/api/auth/users/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      signal: AbortSignal.timeout(15_000),
      body: JSON.stringify({ password }),
    });
    const data = (await response.json()) as UserPayload;
    if (!response.ok) {
      throw new Error(data.error || `failed to reset password (${response.status})`);
    }
  }, []);

  const deleteUser = useCallback(async (id: string) => {
    const response = await fetch(`/api/auth/users/${encodeURIComponent(id)}`, {
      method: "DELETE",
      signal: AbortSignal.timeout(15_000),
    });
    const data = (await response.json()) as UserPayload;
    if (!response.ok) {
      throw new Error(data.error || `failed to delete user (${response.status})`);
    }
    await refresh();
  }, [refresh]);

  const revokeSessions = useCallback(async (id: string) => {
    const response = await fetch(`/api/auth/users/${encodeURIComponent(id)}/sessions`, {
      method: "DELETE",
      signal: AbortSignal.timeout(15_000),
    });
    const data = (await response.json()) as UserPayload;
    if (!response.ok) {
      throw new Error(data.error || `failed to revoke sessions (${response.status})`);
    }
  }, []);

  return { users, loading, error, refresh, createUser, updateRole, resetPassword, deleteUser, revokeSessions };
}
