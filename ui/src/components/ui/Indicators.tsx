import React from 'react';

interface ScoreBarProps {
  value: number; // 0-1
  showLabel?: boolean;
  height?: number;
  animated?: boolean;
}

export const ScoreBar: React.FC<ScoreBarProps> = ({
  value, showLabel = true, height = 6, animated = true,
}) => {
  const clamped = Math.max(0, Math.min(1, value));
  const pct = clamped * 100;
  const color =
    clamped >= 0.65 ? '#ff3366' :
    clamped >= 0.30 ? '#ffd700' :
    '#00ff88';
  const glow =
    clamped >= 0.65 ? 'rgba(255,51,102,0.5)' :
    clamped >= 0.30 ? 'rgba(255,215,0,0.4)' :
    'rgba(0,255,136,0.4)';

  return (
    <div className="flex items-center gap-2 w-full">
      <div
        className="flex-1 rounded-full bg-surface-500 overflow-hidden"
        style={{ height }}
      >
        <div
          className={animated ? 'transition-all duration-500 ease-out' : ''}
          style={{
            width: `${pct}%`,
            height: '100%',
            background: color,
            boxShadow: `0 0 6px ${glow}`,
            borderRadius: 'inherit',
          }}
        />
      </div>
      {showLabel && (
        <span className="text-[10px] font-mono w-8 text-right" style={{ color }}>
          {clamped.toFixed(2)}
        </span>
      )}
    </div>
  );
};

interface StatusDotProps {
  status: 'green' | 'yellow' | 'red' | 'gray';
  size?: number;
}

export const StatusDot: React.FC<StatusDotProps> = ({ status, size = 8 }) => {
  const cls =
    status === 'green'  ? 'bg-neon-green animate-pulse-green' :
    status === 'yellow' ? 'bg-neon-yellow animate-pulse-yellow' :
    status === 'red'    ? 'bg-neon-red animate-pulse-red' :
    'bg-slate-500';
  return (
    <span
      className={`inline-block rounded-full ${cls}`}
      style={{ width: size, height: size }}
    />
  );
};

interface MetricProps {
  label: string;
  value: string | number;
  sub?: string;
  color?: 'green' | 'yellow' | 'red' | 'blue' | 'white' | 'purple';
  large?: boolean;
}

export const Metric: React.FC<MetricProps> = ({
  label, value, sub, color = 'white', large = false,
}) => {
  const colorClass =
    color === 'green'  ? 'text-neon-green text-glow-green' :
    color === 'yellow' ? 'text-neon-yellow text-glow-yellow' :
    color === 'red'    ? 'text-neon-red text-glow-red' :
    color === 'blue'   ? 'text-neon-blue text-glow-blue' :
    color === 'purple' ? 'text-neon-purple' :
    'text-slate-100';

  return (
    <div className="flex flex-col">
      <span className="text-[10px] uppercase tracking-widest text-slate-500 mb-1">{label}</span>
      <span className={`font-mono font-bold ${large ? 'text-2xl' : 'text-lg'} ${colorClass}`}>
        {value}
      </span>
      {sub && <span className="text-[10px] text-slate-500 mt-0.5">{sub}</span>}
    </div>
  );
};
