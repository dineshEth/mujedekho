import { RegisterForm } from "./form";

export default function RegisterPage() {
  return (
    <main className="flex min-h-screen items-center justify-center bg-surface-sunken px-4">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex items-center justify-center gap-2.5">
          <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-accent font-mono text-sm font-bold text-accent-ink">24</span>
          <h1 className="text-xl font-semibold">site24x7</h1>
        </div>
        <div className="rounded-xl border border-border bg-surface-raised p-6 shadow-card">
          <h2 className="text-base font-semibold">Create your account</h2>
          <p className="mb-5 mt-1 text-xs text-ink-muted">
            You'll start a free organization; invite teammates later.
          </p>
          <RegisterForm />
        </div>
        <p className="mt-4 text-center text-xs text-ink-muted">
          Already registered?{" "}
          <a href="/login" className="font-medium text-accent hover:underline">
            Sign in
          </a>
        </p>
      </div>
    </main>
  );
}
