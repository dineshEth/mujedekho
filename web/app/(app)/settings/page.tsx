"use client";

export default function SettingsIndexPage() {
  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-1 text-lg font-semibold">Settings</h1>
      <p className="mb-6 text-xs text-ink-muted">Manage your project and account.</p>
      <div className="grid gap-4 sm:grid-cols-2">
        <a href="/settings/project" className="rounded-lg border border-border bg-surface-raised p-5 shadow-card transition-colors hover:bg-surface-sunken">
          <h2 className="text-sm font-semibold">Project</h2>
          <p className="mt-1 text-xs text-ink-muted">Name, members and monitor defaults for the selected project.</p>
        </a>
        <a href="/settings/account" className="rounded-lg border border-border bg-surface-raised p-5 shadow-card transition-colors hover:bg-surface-sunken">
          <h2 className="text-sm font-semibold">Account</h2>
          <p className="mt-1 text-xs text-ink-muted">Profile, sign-in sessions and password.</p>
        </a>
      </div>
    </div>
  );
}
