interface AlertProps {
  variant: 'warning' | 'error' | 'danger' | 'info';
  children: React.ReactNode;
  style?: React.CSSProperties;
}

export default function Alert({ variant, children, style }: AlertProps) {
  return (
    <div className={`config-msg ${variant}`} style={style}>
      {children}
    </div>
  );
}
