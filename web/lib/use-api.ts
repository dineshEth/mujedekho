/** Tiny polling data hook (no external deps): loading/error/data + refresh. */
"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError, api } from "./api";

export function useApi<T>(path: string | null, opts?: { pollMs?: number; enabled?: boolean }) {
  const enabled = opts?.enabled !== false;
  const pollMs = opts?.pollMs ?? 0;
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(enabled);
  const [error, setError] = useState<ApiError | null>(null);
  const pathRef = useRef(path);
  pathRef.current = path;

  const refresh = useCallback(async () => {
    const p = pathRef.current;
    if (!p || !enabled) return;
    try {
      const d = await api<T>(p);
      setData(d);
      setError(null);
    } catch (e) {
      if (e instanceof ApiError) setError(e);
      else setError(new ApiError(0, "network_error", "Network error"));
    } finally {
      setLoading(false);
    }
  }, [enabled]);

  useEffect(() => {
    if (!path || !enabled) {
      setLoading(false);
      return;
    }
    setLoading(true);
    void refresh();

    if (!pollMs) return;
    const t = setInterval(() => void refresh(), pollMs);
    return () => clearInterval(t);
  }, [path, pollMs, enabled, refresh]);

  return { data, loading, error, refresh, setError };
}
