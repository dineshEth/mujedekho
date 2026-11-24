import type { ReactNode } from "react";
import { Card, CardBody } from "./card";

export function Skeleton({ className = "" }: { className?: string }) {
  return <div className={`animate-pulse rounded-md bg-surface-sunken ${className}`} />;
}

export function TableSkeleton({ rows = 5 }: { rows?: number }) {
  return (
    <div className="space-y-3 p-4">
      {Array.from({ length: rows }).map((_, i) => (
        <Skeleton key={i} className="h-9 w-full" />
      ))}
    </div>
  );
}

export function EmptyState({
  title,
  message,
  action,
}: {
  title: string;
  message: string;
  action?: ReactNode;
}) {
  return (
    <Card>
      <CardBody className="flex flex-col items-center py-14 text-center">
        <div className="mb-3 flex h-10 w-10 items-center justify-center rounded-full bg-surface-sunken text-ink-faint">
          <svg viewBox="0 0 20 20" fill="none" className="h-5 w-5" aria-hidden="true">
            <path d="M10 4a6 6 0 0 1 6 6v4.5l1.5 2h-15L4 14.5V10a6 6 0 0 1 6-6Z" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" />
            <path d="M8.5 16.5a1.5 1.5 0 0 0 3 0" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
          </svg>
        </div>
        <p className="text-sm font-medium text-ink">{title}</p>
        <p className="mt-1 max-w-sm text-xs text-ink-muted">{message}</p>
        {action ? <div className="mt-4">{action}</div> : null}
      </CardBody>
    </Card>
  );
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <Card>
      <CardBody className="flex flex-col items-center py-12 text-center">
        <p className="text-sm font-medium text-down">Something went wrong</p>
        <p className="mt-1 max-w-md text-xs text-ink-muted">{message}</p>
        {onRetry ? (
          <button
            type="button"
            onClick={onRetry}
            className="mt-4 rounded-md border border-border px-3 py-1.5 text-xs font-medium text-ink hover:bg-surface-sunken"
          >
            Retry
          </button>
        ) : null}
      </CardBody>
    </Card>
  );
}
