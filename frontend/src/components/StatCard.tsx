import type { LucideIcon } from 'lucide-react';

interface Props {
  label: string;
  value: string | number;
  suffix?: string;
  icon?: LucideIcon;
}

export function StatCard({ label, value, suffix, icon: Icon }: Props) {
  return (
    <div className="stat-card">
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
        <div className="label">{label}</div>
        {Icon && <Icon size={18} style={{ color: 'var(--text-secondary)', opacity: 0.6 }} />}
      </div>
      <div className="value">{value}{suffix && <span style={{ fontSize: 14, fontWeight: 400, marginLeft: 4 }}>{suffix}</span>}</div>
    </div>
  );
}
