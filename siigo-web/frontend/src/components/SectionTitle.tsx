interface SectionTitleProps {
  children: React.ReactNode;
  danger?: boolean;
  style?: React.CSSProperties;
}

export default function SectionTitle({ children, danger, style }: SectionTitleProps) {
  return (
    <h3 className={`config-section-title${danger ? ' danger' : ''}`} style={style}>
      {children}
    </h3>
  );
}
