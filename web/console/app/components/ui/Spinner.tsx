type SpinnerProps = {
  size?: "sm" | "md" | "lg";
  className?: string;
};

const sizeMap = { sm: 14, md: 20, lg: 28 };

/** 2.0 spinner (kit.css .kt-spin): a thin track ring with a short turning arc in the quiet text colour. */
export function Spinner({ size = "md", className = "" }: SpinnerProps) {
  const s = sizeMap[size];
  return (
    <svg
      width={s}
      height={s}
      viewBox="0 0 24 24"
      className={`kt-spin${className ? ` ${className}` : ""}`}
      role="status"
      aria-label="Loading"
    >
      <circle className="t" cx="12" cy="12" r="10" />
      <circle className="a" cx="12" cy="12" r="10" strokeDasharray="16 48" />
    </svg>
  );
}
