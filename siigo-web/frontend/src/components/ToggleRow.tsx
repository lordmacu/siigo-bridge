import Toggle from './Toggle';

interface ToggleRowProps {
  checked: boolean;
  onChange: () => void;
  label?: string;
  activeLabel?: string;
  inactiveLabel?: string;
  disabled?: boolean;
  style?: React.CSSProperties;
}

export default function ToggleRow({ checked, onChange, label, activeLabel, inactiveLabel, disabled, style }: ToggleRowProps) {
  const displayLabel = checked ? (activeLabel || label || '') : (inactiveLabel || label || '');
  return (
    <div className="send-toggle-row" style={style}>
      <Toggle checked={checked} onChange={onChange} disabled={disabled} />
      <span className={`send-toggle-label ${checked ? 'active' : 'inactive'}`}>
        {displayLabel}
      </span>
    </div>
  );
}
