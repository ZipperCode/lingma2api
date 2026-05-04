interface SkeletonProps {
  variant?: 'text' | 'text-lg' | 'rect' | 'circle';
  className?: string;
  style?: React.CSSProperties;
}

export function Skeleton({ variant = 'text', className = '', style }: SkeletonProps) {
  const base = 'skeleton';
  const map: Record<string, string> = {
    text: 'skeleton-text',
    'text-lg': 'skeleton-text-lg',
    rect: 'skeleton-rect',
    circle: 'skeleton-circle',
  };
  return <div className={`${base} ${map[variant] || ''} ${className}`} style={style} />;
}

interface SkeletonTableProps {
  rows?: number;
  cols?: number;
}

export function SkeletonTable({ rows = 5, cols = 6 }: SkeletonTableProps) {
  return (
    <div>
      {Array.from({ length: rows }).map((_, ri) => (
        <div key={ri} className="skeleton-table-row">
          {Array.from({ length: cols }).map((_, ci) => (
            <div key={ci} className="skeleton-table-cell skeleton" />
          ))}
        </div>
      ))}
    </div>
  );
}
