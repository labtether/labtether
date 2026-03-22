"use client";

import { useState } from "react";

export type SessionCardProps = {
  type: "active" | "detached" | "archived" | "saved";
  title: string;
  subtitle: string;
  metadata?: string;
  warning?: string;
  isAssetLinked?: boolean;
  onClick: (e: React.MouseEvent) => void;
  onContextMenu: (e: React.MouseEvent) => void;
};

const TYPE_CONFIG: Record<
  SessionCardProps["type"],
  { borderColor: string; icon: string }
> = {
  active:   { borderColor: "#4ade80", icon: "●" },
  detached: { borderColor: "#facc15", icon: "◐" },
  archived: { borderColor: "#666",    icon: "▫" },
  saved:    { borderColor: "#60a5fa", icon: "☆" },
};

export default function SessionCard({
  type,
  title,
  subtitle,
  metadata,
  warning,
  isAssetLinked,
  onClick,
  onContextMenu,
}: SessionCardProps) {
  const [hovered, setHovered] = useState(false);
  const { borderColor, icon } = TYPE_CONFIG[type];

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onClick}
      onAuxClick={(e) => { if (e.button === 1) { e.preventDefault(); onClick(e); } }}
      onContextMenu={onContextMenu}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onClick(e as unknown as React.MouseEvent);
        }
      }}
      style={{
        display: "flex",
        alignItems: "flex-start",
        justifyContent: "space-between",
        padding: "6px 8px",
        borderRadius: 4,
        marginBottom: 2,
        borderLeft: `2px solid ${borderColor}`,
        backgroundColor: hovered ? "#2a2a2a" : "transparent",
        cursor: "pointer",
        userSelect: "none",
        transition: "background-color 0.12s",
        minWidth: 0,
      }}
    >
      {/* Left: icon + text block */}
      <div style={{ display: "flex", alignItems: "flex-start", gap: 6, minWidth: 0, flex: 1 }}>
        {/* Status icon */}
        <span
          style={{
            fontSize: 10,
            color: borderColor,
            lineHeight: "15px",
            flexShrink: 0,
          }}
        >
          {icon}
        </span>

        {/* Text block */}
        <div style={{ minWidth: 0, flex: 1 }}>
          {/* Title row */}
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: 4,
              flexWrap: "wrap",
            }}
          >
            <span
              style={{
                fontSize: 11,
                fontWeight: 500,
                color: "#e0e0e0",
                overflow: "hidden",
                textOverflow: "ellipsis",
                whiteSpace: "nowrap",
              }}
            >
              {title}
            </span>
            {isAssetLinked && (
              <span
                style={{
                  fontSize: 7,
                  fontWeight: 600,
                  textTransform: "uppercase",
                  letterSpacing: "0.05em",
                  color: "#93c5fd",
                  backgroundColor: "rgba(96, 165, 250, 0.18)",
                  border: "1px solid rgba(96, 165, 250, 0.35)",
                  borderRadius: 3,
                  padding: "1px 3px",
                  flexShrink: 0,
                  lineHeight: "11px",
                }}
              >
                ASSET
              </span>
            )}
          </div>

          {/* Subtitle */}
          <div
            style={{
              fontSize: 9,
              color: "#888",
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
              marginTop: 1,
            }}
          >
            {subtitle}
          </div>

          {/* Warning */}
          {warning && (
            <div
              style={{
                fontSize: 8,
                color: "var(--warn)",
                marginTop: 2,
                overflow: "hidden",
                textOverflow: "ellipsis",
                whiteSpace: "nowrap",
              }}
            >
              {warning}
            </div>
          )}
        </div>
      </div>

      {/* Right: metadata */}
      {metadata && (
        <span
          style={{
            fontSize: 9,
            color: "#888",
            flexShrink: 0,
            marginLeft: 6,
            paddingTop: 1,
            whiteSpace: "nowrap",
          }}
        >
          {metadata}
        </span>
      )}
    </div>
  );
}
