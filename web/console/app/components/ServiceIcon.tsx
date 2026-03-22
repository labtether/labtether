"use client";

import { Globe } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

interface ServiceIconProps {
  iconKey: string;
  size?: number;
  className?: string;
}

const dataImagePattern = /^data:image\/(png|jpe?g|gif|webp|avif);base64,[a-z0-9+/=\s]+$/i;
const iconKeyPattern = /^[a-z0-9][a-z0-9._-]{0,120}$/i;
const controlOrWhitespacePattern = /[\u0000-\u001f\u007f\s]/;
const maxDataURLLength = 256_000;
const maxURLLength = 2_048;

function isSafeRelativeIconPath(value: string): boolean {
  if (!value.startsWith("/") || value.startsWith("//")) {
    return false;
  }
  if (controlOrWhitespacePattern.test(value)) {
    return false;
  }
  try {
    const parsed = new URL(value, "https://labtether.local");
    return parsed.origin === "https://labtether.local" && !parsed.hash;
  } catch {
    return false;
  }
}

function sanitizeCustomIconSource(iconKey: string): string | null {
  const trimmed = iconKey.trim();
  if (!trimmed) {
    return null;
  }
  if (dataImagePattern.test(trimmed) && trimmed.length <= maxDataURLLength) {
    return trimmed;
  }
  if (isSafeRelativeIconPath(trimmed)) {
    return trimmed;
  }
  if (trimmed.length > maxURLLength) {
    return null;
  }
  let parsed: URL;
  try {
    parsed = new URL(trimmed);
  } catch {
    return null;
  }
  if ((parsed.protocol !== "http:" && parsed.protocol !== "https:") || parsed.username || parsed.password) {
    return null;
  }
  parsed.hash = "";
  return parsed.toString();
}

function sanitizeBuiltinIconKey(iconKey: string): string | null {
  const trimmed = iconKey.trim();
  if (!trimmed || !iconKeyPattern.test(trimmed)) {
    return null;
  }
  return trimmed;
}

export function isCustomServiceIconSource(iconKey: string): boolean {
  return sanitizeCustomIconSource(iconKey) !== null;
}

export function describeServiceIconSelection(iconKey: string): string {
  const trimmed = iconKey.trim();
  if (!trimmed) {
    return "";
  }
  const customSource = sanitizeCustomIconSource(trimmed);
  if (customSource && dataImagePattern.test(customSource)) {
    return "Custom Upload";
  }
  if (customSource) {
    return "Custom URL";
  }
  return sanitizeBuiltinIconKey(trimmed) ?? "";
}

function resolveServiceIconSource(iconKey: string): { src: string; custom: boolean } | null {
  const customSource = sanitizeCustomIconSource(iconKey);
  if (customSource) {
    return { src: customSource, custom: true };
  }
  const builtinKey = sanitizeBuiltinIconKey(iconKey);
  if (!builtinKey) {
    return null;
  }
  return { src: `/service-icons/${builtinKey}.svg`, custom: false };
}

export function ServiceIcon({ iconKey, size = 32, className = "" }: ServiceIconProps) {
  const [failed, setFailed] = useState(false);
  const resolved = useMemo(() => resolveServiceIconSource(iconKey), [iconKey]);

  useEffect(() => {
    setFailed(false);
  }, [iconKey]);

  if (!resolved || failed) {
    return <Globe size={size} className={`text-[var(--muted)] ${className}`} />;
  }

  if (resolved.custom) {
    return (
      <img
        src={resolved.src}
        alt="custom icon"
        width={size}
        height={size}
        className={className}
        onError={() => setFailed(true)}
        loading="lazy"
        decoding="async"
        referrerPolicy="no-referrer"
      />
    );
  }

  return (
    <img
      src={resolved.src}
      alt={iconKey}
      width={size}
      height={size}
      className={className}
      onError={() => setFailed(true)}
      loading="lazy"
      decoding="async"
      draggable={false}
    />
  );
}
