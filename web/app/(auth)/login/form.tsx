"use client";

import { useState, type FormEvent } from "react";
import { useRouter, useSearchParams } from "next/navigation";

export function LoginForm() {
  const router = useRouter();
  const params = useSearchParams();
  const next = params.get("next") ?? "/dashboard";

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const res = await fetch("/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, password }),
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body?.error?.message ?? `Sign-in failed (${res.status})`);
      }
      // Full navigation so the middleware sees the freshly set cookie.
      window.location.assign(next.startsWith("/") && !next.startsWith("//") ? next : "/dashboard");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Sign-in failed");
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={onSubmit} className="space-y-4" noValidate>
      <div>
        <label htmlFor="email" className="mb-1.5 block text-xs font-medium text-ink">Email</label>
        <input
          id="email" type="email" required autoComplete="email"
          value={email} onChange={(e) => setEmail(e.target.value)}
          className="block w-full rounded-md border border-border bg-surface px-3 py-2 text-sm
            focus:border-accent focus:outline-none focus:ring-2 focus:ring-accent/20"
        />
      </div>
      <div>
        <label htmlFor="password" className="mb-1.5 block text-xs font-medium text-ink">Password</label>
        <input
          id="password" type="password" required autoComplete="current-password"
          value={password} onChange={(e) => setPassword(e.target.value)}
          className="block w-full rounded-md border border-border bg-surface px-3 py-2 text-sm
            focus:border-accent focus:outline-none focus:ring-2 focus:ring-accent/20"
        />
      </div>
      {error ? (
        <p role="alert" className="rounded-md bg-down/10 px-3 py-2 text-xs text-down">{error}</p>
      ) : null}
      <button
        type="submit" disabled={submitting}
        className="w-full rounded-md bg-accent py-2 text-sm font-semibold text-accent-ink
          transition-colors hover:bg-accent-hover disabled:cursor-not-allowed disabled:opacity-60"
      >
        {submitting ? "Signing in…" : "Sign in"}
      </button>
    </form>
  );
}
