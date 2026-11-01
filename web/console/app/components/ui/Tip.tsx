"use client";

import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import type { ReactNode } from "react";

type TipProps = {
  content: string;
  side?: "top" | "right" | "bottom" | "left";
  children: ReactNode;
};

/** Tooltips wait 450 ms, then show at once while the pointer moves between tipped controls. */
export function TipProvider({ children }: { children: ReactNode }) {
  return (
    <TooltipPrimitive.Provider delayDuration={450} skipDelayDuration={500}>
      {children}
    </TooltipPrimitive.Provider>
  );
}

/** 2.0 tooltip (kit.css .kt-tip): a small dark pill, no arrow. */
export function Tip({ content, side = "top", children }: TipProps) {
  return (
    <TooltipPrimitive.Root>
      <TooltipPrimitive.Trigger asChild>{children}</TooltipPrimitive.Trigger>
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Content side={side} sideOffset={6} collisionPadding={8} className="kt-tip">
          {content}
        </TooltipPrimitive.Content>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  );
}
