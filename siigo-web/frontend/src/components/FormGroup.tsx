interface FormGroupProps {
  label: string;
  hint?: string;
  children: React.ReactNode;
  style?: React.CSSProperties;
}

export default function FormGroup({ label, hint, children, style }: FormGroupProps) {
  return (
    <div className="form-group" style={style}>
      <label>{label}</label>
      {children}
      {hint && <small className="form-hint">{hint}</small>}
    </div>
  );
}
