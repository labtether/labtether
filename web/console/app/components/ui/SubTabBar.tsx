"use client";

export type SubTab = {
  id: string;
  label: string;
};

type SubTabBarProps = {
  tabs: SubTab[];
  activeTab: string;
  onTabChange: (tabId: string) => void;
};

export function SubTabBar({ tabs, activeTab, onTabChange }: SubTabBarProps) {
  return (
    <div className="flex gap-1 border-b border-[var(--line)] mb-4 overflow-x-auto">
      {tabs.map((tab) => (
        <button
          key={tab.id}
          onClick={() => onTabChange(tab.id)}
          className={`px-3 py-2 text-sm font-medium whitespace-nowrap transition-colors border-b-2 -mb-px ${
            activeTab === tab.id
              ? "border-[var(--accent)] text-[var(--text)]"
              : "border-transparent text-[var(--muted)] hover:text-[var(--text)] hover:border-[var(--line)]"
          }`}
        >
          {tab.label}
        </button>
      ))}
    </div>
  );
}
