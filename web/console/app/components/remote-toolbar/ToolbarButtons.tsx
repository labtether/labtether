"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { Check } from "lucide-react";
import { createContext, forwardRef, useContext, type ButtonHTMLAttributes, type ReactNode } from "react";

/**
 * Where the tool menus open. In full screen only the viewer is on screen, so menus must open inside it; elsewhere they
 * open in the page body (null).
 */
export const ToolbarPortalContext = createContext<HTMLElement | null>(null);

type ToolButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & { label: string; pressed?: boolean; children: ReactNode };

/** A quiet 32 px icon button. label is its name and its tooltip. */
export const ToolButton = forwardRef<HTMLButtonElement, ToolButtonProps>(function ToolButton(
  { label, pressed, className = "", children, ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type="button"
      className={`rv-ib${className ? ` ${className}` : ""}`}
      aria-label={label}
      title={label}
      aria-pressed={pressed}
      {...rest}
    >
      {children}
    </button>
  );
});

/**
 * An icon button that opens a menu. keepFocus: the menu's items act on the remote screen, so focus goes back to the
 * viewer (the item's handler moves it) instead of to this button.
 */
export function ToolMenu({
  icon,
  label,
  disabled,
  keepFocus = false,
  onOpenChange,
  children,
}: {
  icon: ReactNode;
  label: string;
  disabled?: boolean;
  keepFocus?: boolean;
  onOpenChange?: (open: boolean) => void;
  children: ReactNode;
}) {
  const container = useContext(ToolbarPortalContext);
  return (
    <Menu.Root modal={false} onOpenChange={onOpenChange}>
      <Menu.Trigger asChild disabled={disabled}>
        <ToolButton label={label} disabled={disabled}>
          {icon}
        </ToolButton>
      </Menu.Trigger>
      <Menu.Portal container={container ?? undefined}>
        <Menu.Content
          className="lt-menu rv-menu"
          align="end"
          sideOffset={6}
          collisionPadding={8}
          onCloseAutoFocus={keepFocus ? (event) => event.preventDefault() : undefined}
        >
          {children}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}

/** A menu item that runs an action. */
export function ToolItem({ children, onSelect, disabled, title }: { children: ReactNode; onSelect: () => void; disabled?: boolean; title?: string }) {
  return (
    <Menu.Item className="lt-menu-item" onSelect={onSelect} disabled={disabled} title={title}>
      {children}
    </Menu.Item>
  );
}

/** An on/off menu item with a check mark. */
export function ToolCheck({
  children,
  checked,
  onChange,
  disabled,
  hint,
}: {
  children: ReactNode;
  checked: boolean;
  onChange: () => void;
  disabled?: boolean;
  hint?: string;
}) {
  return (
    <Menu.CheckboxItem className="lt-menu-item" checked={checked} onCheckedChange={() => onChange()} disabled={disabled}>
      {children}
      {hint ? <span className="lt-menu-hint">{hint}</span> : null}
      <Menu.ItemIndicator className={hint ? "rv-mcheck" : "lt-menu-check"}>
        <Check aria-hidden="true" />
      </Menu.ItemIndicator>
    </Menu.CheckboxItem>
  );
}

/** A labelled group of choices, one of which is on. */
export function ToolChoice<T extends string>({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: T;
  options: Array<{ value: T; label: string }>;
  onChange: (value: T) => void;
}) {
  return (
    <>
      <Menu.Label className="rv-mh">{label}</Menu.Label>
      <Menu.RadioGroup aria-label={label} value={value} onValueChange={(next) => onChange(next as T)}>
        {options.map((option) => (
          <Menu.RadioItem key={option.value} value={option.value} className="lt-menu-item">
            {option.label}
            <Menu.ItemIndicator className="lt-menu-check">
              <Check aria-hidden="true" />
            </Menu.ItemIndicator>
          </Menu.RadioItem>
        ))}
      </Menu.RadioGroup>
    </>
  );
}

export const ToolSep = () => <Menu.Separator className="lt-menu-sep" />;
export const ToolLabel = ({ children }: { children: ReactNode }) => <Menu.Label className="rv-mh">{children}</Menu.Label>;
