type SpinnerProps = {
  size?: "sm" | "md" | "lg";
  className?: string;
};

const sizeMap = { sm: 14, md: 20, lg: 28 };

export function Spinner({ size = "md", className = "" }: SpinnerProps) {
  const s = sizeMap[size];
  return (
    <svg
      width={s}
      height={s}
      viewBox="0 0 24 24"
      fill="none"
      className={className}
      style={{ animation: "spin 1s linear infinite" }}
      role="status"
      aria-label="Loading"
    >
      <circle
        cx="12"
        cy="12"
        r="10"
        stroke="var(--muted)"
        strokeWidth="2.5"
        strokeLinecap="round"
        opacity="0.25"
      />
      <circle
        cx="12"
        cy="12"
        r="10"
        stroke="var(--accent)"
        strokeWidth="2.5"
        strokeLinecap="round"
        strokeDasharray="60"
        strokeDashoffset="40"
      />
    </svg>
  );
}
