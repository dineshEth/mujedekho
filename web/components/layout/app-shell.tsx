"use client";

import { useState } from "react";
import { Sidebar } from "@/components/layout/sidebar";
import { Topbar } from "@/components/layout/topbar";
import { ErrorState } from "@/components/ui/feedback";
import { WorkspaceProvider, useWorkspace } from "@/lib/workspace";

export default function AppLayout({ children }: { children: React.ReactNode }) {
  return (
    <WorkspaceProvider>
      <Shell>{children}</Shell>
    </WorkspaceProvider>
  );
}

function Shell({ children }: { children: React.ReactNode }) {
  const ws = useWorkspace();
  const [menuOpen, setMenuOpen] = useState(false);

  if (ws.loading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="flex items-center gap-2 text-sm text-ink-muted">
          <span className="h-4 w-4 animate-spin rounded-full border-2 border-accent border-t-transparent" />
          Loading workspace…
        </div>
      </div>
    );
  }

  if (!ws.user) {
    return (
      <div className="p-6">
        <ErrorState
          message="Your session has expired. Please sign in again."
          onRetry={() => window.location.assign("/login")}
        />
      </div>
    );
  }

  return (
    <div className="min-h-screen">
      {/* Desktop sidebar */}
      <aside className="fixed inset-y-0 left-0 z-30 w-60 border-r border-border lg:block hidden">
        <Sidebar />
      </aside>

      {/* Mobile drawer */}
      {menuOpen ? (
        <div className="fixed inset-0 z-40 lg:hidden" role="dialog" aria-modal="true">
          <div className="absolute inset-0 bg-black/40" onClick={() => setMenuOpen(false)} />
          <div className="absolute inset-y-0 left-0 w-64 border-r border-border shadow-overlay animate-fade-in">
            <Sidebar onNavigate={() => setMenuOpen(false)} />
          </div>
        </div>
      ) : null}

      <div className="lg:pl-60">
        <Topbar onMenu={() => setMenuOpen(true)} />
        <main className="mx-auto max-w-7xl px-4 py-6 sm:px-6">{children}</main>
      </div>
    </div>
  );
}
