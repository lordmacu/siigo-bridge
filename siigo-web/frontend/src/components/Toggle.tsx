interface ToggleProps {
  checked: boolean;
  onChange: () => void;
  disabled?: boolean;
}

export default function Toggle({ checked, onChange, disabled }: ToggleProps) {
  return (
    <label className="toggle-switch">
      <input type="checkbox" checked={checked} onChange={onChange} disabled={disabled} />
      <span className="toggle-slider"></span>
    </label>
  );
}
