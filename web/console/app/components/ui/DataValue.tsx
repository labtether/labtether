"use client";

import { useEffect, useRef, useState } from "react";

type DataValueProps = {
  value: string | number;
  className?: string;
  mono?: boolean;
};

export function DataValue({ value, className = "", mono = true }: DataValueProps) {
  const prevValue = useRef(value);
  const [flash, setFlash] = useState(false);

  useEffect(() => {
    if (prevValue.current !== value) {
      prevValue.current = value;
      setFlash(true);
      const timer = setTimeout(() => setFlash(false), 600);
      return () => clearTimeout(timer);
    }
  }, [value]);

  return (
    <span
      className={`${mono ? "font-mono" : ""} tabular-nums ${className}`}
      style={flash ? { animation: "value-flash 600ms ease-out" } : undefined}
    >
      {value}
    </span>
  );
}
