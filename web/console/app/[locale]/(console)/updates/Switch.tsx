/** An on/off switch with its label on the left, as one row of an options sheet. */
export function SwitchRow({ id, label, on, onChange, disabled }: { id: string; label: string; on: boolean; onChange: (on: boolean) => void; disabled?: boolean }) {
  return (
    <div className="iu-opt">
      <span id={`${id}-l`}>{label}</span>
      <button
        type="button"
        role="switch"
        id={id}
        className="iu-switch"
        aria-checked={on}
        aria-labelledby={`${id}-l`}
        disabled={disabled}
        onClick={() => onChange(!on)}
      >
        <i aria-hidden="true" />
      </button>
    </div>
  );
}
