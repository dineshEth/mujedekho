import { forwardRef } from "react";
import type { ButtonHTMLAttributes } from "react";

const variants = {
  primary:
    "bg-accent text-accent-ink hover:bg-accent-hover disabled:hover:bg-accent",
  secondary:
    "border border-border bg-surface text-ink hover:bg-surface-sunken disabled:hover:bg-surface",
  danger:
    "bg-down text-white hover:opacity-90 disabled:hover:opacity-100",
  ghost:
    "text-ink-muted hover:bg-surface-sunken hover:text-ink",
} as const;

const sizes = {
  sm: "h-8 px-3 text-xs gap-1.5",
  md: "h-9 px-4 text-sm gap-2",
} as const;

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: keyof typeof variants;
  size?: keyof typeof sizes;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ variant = "primary", size = "md", className = "", type = "button", ...rest }, ref) => (
    <button
      ref={ref}
      type={type}
      className={`inline-flex items-center justify-center rounded-md font-medium transition-colors
        focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent
        disabled:cursor-not-allowed disabled:opacity-50 ${variants[variant]} ${sizes[size]} ${className}`}
      {...rest}
    />
  ),
);
Button.displayName = "Button";
