interface TabBarProps {
  tabs: { key: string; label: string }[];
  activeTab: string;
  onTabChange: (tab: string) => void;
  variant?: 'module' | 'sub';
  children?: React.ReactNode;
}

export default function TabBar({ tabs, activeTab, onTabChange, variant = 'module', children }: TabBarProps) {
  const containerClass = variant === 'sub' ? 'subtabs' : 'module-tabs';
  const tabClass = variant === 'sub' ? 'subtab' : 'module-tab';

  return (
    <div className={containerClass}>
      {tabs.map(tab => (
        <div
          key={tab.key}
          className={`${tabClass} ${activeTab === tab.key ? 'active' : ''}`}
          onClick={() => onTabChange(tab.key)}
        >
          {tab.label}
        </div>
      ))}
      {children}
    </div>
  );
}
