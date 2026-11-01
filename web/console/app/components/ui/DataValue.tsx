type DataValueProps = {
  value: string | number;
  className?: string;
  mono?: boolean;
};

/** A live number in tabular figures. 2.0: no flash or glow when it changes; mono only when asked (paths, code). */
export function DataValue({ value, className = "", mono = false }: DataValueProps) {
  return <span className={`${mono ? "font-mono " : ""}kt-num ${className}`}>{value}</span>;
}
