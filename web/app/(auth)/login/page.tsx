import { Suspense } from "react";
import { LoginForm } from "./form";

export default function LoginPage() {
  return (
    <main className="flex min-h-screen items-center justify-center bg-surface-sunken px-4">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex items-center justify-center gap-2.5">
          <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-accent font-mono text-sm font-bold text-accent-ink">24</span>
          <h1 className="text-xl font-semibold">site24x7</h1>
        </div>
        <div className="rounded-xl border border-border bg-surface-raised p-6 shadow-card">
          <h2 className="text-base font-semibold">Sign in</h2>
          <p className="mb-5 mt-1 text-xs text-ink-muted">Monitor your websites' availability and uptime.</p>
          <Suspense>
            <LoginForm />
          </Suspense>
        </div>
        <p className="mt-4 text-center text-xs text-ink-muted">
          No account?{" "}
          <a href="/register" className="font-medium text-accent hover:underline">
            Create one
          </a>
        </p>
      </div>
    </main>
  );
}
