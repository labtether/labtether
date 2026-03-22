"use client";

import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import type { ReactNode } from "react";

type TipProps = {
  content: string;
  side?: "top" | "right" | "bottom" | "left";
  children: ReactNode;
};

export function TipProvider({ children }: { children: ReactNode }) {
  return (
    <TooltipPrimitive.Provider delayDuration={200} skipDelayDuration={100}>
      {children}
    </TooltipPrimitive.Provider>
  );
}

export function Tip({ content, side = "top", children }: TipProps) {
  return (
    <TooltipPrimitive.Root>
      <TooltipPrimitive.Trigger asChild>{children}</TooltipPrimitive.Trigger>
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Content
          side={side}
          sideOffset={6}
          className="z-[100] rounded-lg border border-[var(--panel-border)] bg-[var(--panel-glass)] px-2.5 py-1.5 text-xs text-[var(--text)] shadow-[var(--shadow-sm)]"
          style={{
            backdropFilter: "blur(var(--blur-lg))",
            WebkitBackdropFilter: "blur(var(--blur-lg))",
          }}
        >
          {content}
          <TooltipPrimitive.Arrow className="fill-[var(--panel-border)]" width={10} height={5} />
        </TooltipPrimitive.Content>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  );
}
