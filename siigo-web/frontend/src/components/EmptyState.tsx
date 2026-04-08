interface EmptyStateProps {
  title: string;
  message?: string;
  children?: React.ReactNode;
  style?: React.CSSProperties;
}

export default function EmptyState({ title, message, children, style }: EmptyStateProps) {
  return (
    <div className="empty-state" style={style}>
      <h3>{title}</h3>
      {message && <p>{message}</p>}
      {children}
    </div>
  );
}
