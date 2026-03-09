interface StatusBadgeProps {
  status: string;
}

export default function StatusBadge({ status }: StatusBadgeProps) {
  return <span className={`status ${status}`}>{status}</span>;
}
