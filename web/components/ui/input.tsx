import { forwardRef, useId } from "react";
import type { InputHTMLAttributes, SelectHTMLAttributes, ReactNode } from "react";
import type { ChangeEvent } from "react";

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(
  ({ className = "", ...rest }, ref) => (
    <input
      ref={ref}
      className={`block w-full rounded-md border border-border bg-surface px-3 py-2 text-sm text-ink
        placeholder:text-ink-faint transition-colors
        focus:border-accent focus:outline-none focus:ring-2 focus:ring-accent/20
        disabled:cursor-not-allowed disabled:opacity-50 ${className}`}
      {...rest}
    />
  ),
);
Input.displayName = "Input";

export function Label({ htmlFor, children, hint }: { htmlFor: string; children: ReactNode; hint?: string }) {
  return (
    <label htmlFor={htmlFor} className="mb-1.5 block text-xs font-medium text-ink">
      {children}
      {hint ? <span className="ml-1 font-normal text-ink-faint">{hint}</span> : null}
    </label>
  );
}

export interface SelectOption {
  value: string;
  label: string;
}

export function Select({
  options,
  onValueChange,
  className = "",
  ...rest
}: Omit<SelectHTMLAttributes<HTMLSelectElement>, "onChange"> & {
  options: SelectOption[];
  onValueChange?: (value: string) => void;
}) {
  return (
    <select
      className={`block rounded-md border border-border bg-surface px-3 py-2 text-sm text-ink
        focus:border-accent focus:outline-none focus:ring-2 focus:ring-accent/20
        disabled:cursor-not-allowed disabled:opacity-50 ${className}`}
      onChange={(e: ChangeEvent<HTMLSelectElement>) => onValueChange?.(e.target.value)}
      {...rest}
    >
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  );
}
