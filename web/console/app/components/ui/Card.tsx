import type { CSSProperties, ReactNode } from "react";

type CardProps = {
  variant?: "default" | "flush";
  interactive?: boolean;
  highlight?: boolean;
  className?: string;
  style?: CSSProperties;
  children: ReactNode;
};

/** 2.0 sheet: the raised surface that holds content (kit.css .kt-sheet). `highlight` adds a slightly stronger edge. */
export function Card({
  variant = "default",
  interactive = false,
  highlight = false,
  className = "",
  style,
  children,
}: CardProps) {
  const classes = [
    "kt-sheet",
    variant === "flush" ? "flush" : "",
    interactive ? "link" : "",
    highlight ? "mark" : "",
    className,
  ].filter(Boolean).join(" ");

  return (
    <div className={classes} style={style}>
      {children}
    </div>
  );
}
