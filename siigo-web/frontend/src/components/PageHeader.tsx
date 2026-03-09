interface PageHeaderProps {
  title: string;
  children?: React.ReactNode;
}

export default function PageHeader({ title, children }: PageHeaderProps) {
  return (
    <div className="topbar">
      <h2>{title}</h2>
      {children && <div className="topbar-actions">{children}</div>}
    </div>
  );
}
