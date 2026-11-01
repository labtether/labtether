import type { ButtonHTMLAttributes } from "react";

type SwitchProps = Omit<ButtonHTMLAttributes<HTMLButtonElement>, "onChange" | "role"> & {
  checked: boolean;
  onChange: (next: boolean) => void;
  /** The 28×16 switch for rows. */
  size?: "sm" | "md";
};

/** 2.0 on/off switch (kit.css .kt-switch). Give it an aria-label or a <label htmlFor>. */
export function Switch({ checked, onChange, size = "md", className = "", type = "button", ...rest }: SwitchProps) {
  return (
    <button
      {...rest}
      type={type}
      role="switch"
      aria-checked={checked}
      className={`kt-switch${size === "sm" ? " sm" : ""}${className ? ` ${className}` : ""}`}
      onClick={(event) => {
        rest.onClick?.(event);
        if (!event.defaultPrevented) onChange(!checked);
      }}
    >
      <i aria-hidden="true" />
    </button>
  );
}
