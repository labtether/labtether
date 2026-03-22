import type { CSSProperties, ReactNode } from "react";

type CardProps = {
  variant?: "default" | "flush";
  interactive?: boolean;
  highlight?: boolean;
  className?: string;
  style?: CSSProperties;
  children: ReactNode;
};

const glassStyle: CSSProperties = {
  boxShadow: "var(--shadow-panel)",
};

export function Card({
  variant = "default",
  interactive = false,
  highlight = false,
  className = "",
  style,
  children,
}: CardProps) {
  const padding = variant === "flush" ? "" : "p-4";
  const mergedStyle = style ? { ...glassStyle, ...style } : glassStyle;

  const inner = (
    <div
      className={`relative bg-[var(--panel-glass)] border border-[var(--panel-border)] rounded-lg transition-[border-color,box-shadow,transform] duration-[var(--dur-fast)] overflow-hidden ${
        interactive
          ? "hover:border-[var(--line)] hover:-translate-y-px hover:shadow-[var(--shadow-sm)] cursor-pointer"
          : ""
      } ${padding} ${className}`}
      style={mergedStyle}
    >
      {/* Top-edge specular highlight */}
      <div
        className="absolute top-0 left-[15%] right-[15%] h-px pointer-events-none"
        style={{
          background:
            "linear-gradient(90deg, transparent, var(--surface), transparent)",
        }}
      />
      {children}
    </div>
  );

  if (highlight) {
    return (
      <div
        className="rounded-[calc(var(--radius-lg)+1px)] p-px"
        style={{
          background:
            "linear-gradient(135deg, rgba(var(--accent-rgb),0.2), rgba(var(--accent-rgb),0.03) 40%, rgba(var(--accent-rgb),0.15) 60%, rgba(var(--accent-rgb),0.03))",
          backgroundSize: "200% 200%",
          animation: "border-travel 8s ease infinite",
        }}
      >
        {inner}
      </div>
    );
  }

  return inner;
}
