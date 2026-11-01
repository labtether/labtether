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

/** 2.0 tab row (kit.css .kt-tabs, the same look as the shell's .lt-tabs): quiet words, the open one inked. */
export function SubTabBar({ tabs, activeTab, onTabChange }: SubTabBarProps) {
  return (
    <div className="kt-tabs">
      {tabs.map((tab) => (
        <button
          key={tab.id}
          type="button"
          aria-current={activeTab === tab.id ? "true" : undefined}
          onClick={() => onTabChange(tab.id)}
        >
          {tab.label}
        </button>
      ))}
    </div>
  );
}
