"use client";

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";

/** After a successful register the API creates the user only — the first
 * organization is created on the dashboard onboarding card. */
export function RegisterForm() {
  const router = useRouter();
  const [fullName, setFullName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const res = await fetch("/api/auth/register", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, password, full_name: fullName }),
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        const details = body?.error?.details as { password?: string[] } | undefined;
        const msg = details?.password?.[0] ?? body?.error?.message ?? `Registration failed (${res.status})`;
        throw new Error(msg);
      }
      router.replace("/dashboard?welcome=1");
      router.refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Registration failed");
      setSubmitting(false);
    }
  }

  const field =
    "block w-full rounded-md border border-border bg-surface px-3 py-2 text-sm focus:border-accent focus:outline-none focus:ring-2 focus:ring-accent/20";

  return (
    <form onSubmit={onSubmit} className="space-y-4" noValidate>
      <div>
        <label htmlFor="full_name" className="mb-1.5 block text-xs font-medium text-ink">Full name</label>
        <input id="full_name" type="text" required autoComplete="name" value={fullName}
          onChange={(e) => setFullName(e.target.value)} className={field} />
      </div>
      <div>
        <label htmlFor="email" className="mb-1.5 block text-xs font-medium text-ink">Email</label>
        <input id="email" type="email" required autoComplete="email" value={email}
          onChange={(e) => setEmail(e.target.value)} className={field} />
      </div>
      <div>
        <label htmlFor="password" className="mb-1.5 block text-xs font-medium text-ink">
          Password <span className="font-normal text-ink-faint">(min. 12 characters)</span>
        </label>
        <input id="password" type="password" required minLength={12} autoComplete="new-password" value={password}
          onChange={(e) => setPassword(e.target.value)} className={field} aria-describedby="password-hint" />
        <p id="password-hint" className="mt-1 text-2xs text-ink-faint">
          At least 12 characters. Stored with Argon2id hashing.
        </p>
      </div>
      {error ? (
        <p role="alert" className="rounded-md bg-down/10 px-3 py-2 text-xs text-down">{error}</p>
      ) : null}
      <button
        type="submit" disabled={submitting}
        className="w-full rounded-md bg-accent py-2 text-sm font-semibold text-accent-ink
          transition-colors hover:bg-accent-hover disabled:cursor-not-allowed disabled:opacity-60"
      >
        {submitting ? "Creating account…" : "Create account"}
      </button>
    </form>
  );
}
