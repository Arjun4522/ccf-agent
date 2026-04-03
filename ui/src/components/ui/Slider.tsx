import React from 'react';

interface SliderProps {
  value: number;
  onChange: (v: number) => void;
  min: number;
  max: number;
  step?: number;
  label?: string;
  unit?: string;
  disabled?: boolean;
  color?: 'blue' | 'green' | 'yellow' | 'red' | 'purple';
  formatValue?: (v: number) => string;
}

const THUMB_COLORS = {
  blue:   'accent-neon-blue',
  green:  'accent-neon-green',
  yellow: 'accent-neon-yellow',
  red:    'accent-neon-red',
  purple: 'accent-neon-purple',
};

export const Slider: React.FC<SliderProps> = ({
  value, onChange, min, max, step = 0.01, label, unit = '', disabled = false,
  color = 'blue', formatValue,
}) => {
  const pct = ((value - min) / (max - min)) * 100;
  const display = formatValue ? formatValue(value) : `${value}${unit}`;

  return (
    <div className={`flex flex-col gap-1.5 ${disabled ? 'opacity-40' : ''}`}>
      {label && (
        <div className="flex items-center justify-between">
          <span className="text-xs text-slate-400 font-medium">{label}</span>
          <span className="text-xs font-mono text-neon-blue font-semibold">{display}</span>
        </div>
      )}
      <div className="relative flex items-center h-5">
        {/* Track fill */}
        <div
          className="absolute left-0 h-1 rounded-full pointer-events-none z-10"
          style={{
            width: `${pct}%`,
            background: color === 'blue'   ? '#00d4ff' :
                        color === 'green'  ? '#00ff88' :
                        color === 'yellow' ? '#ffd700' :
                        color === 'purple' ? '#b44fff' : '#ff3366',
            opacity: 0.7,
          }}
        />
        <input
          type="range"
          min={min}
          max={max}
          step={step}
          value={value}
          disabled={disabled}
          onChange={e => onChange(Number(e.target.value))}
          className={`w-full relative z-20 ${THUMB_COLORS[color]}`}
          style={{ background: 'transparent' }}
        />
      </div>
      <div className="flex justify-between">
        <span className="text-[10px] text-slate-600">{min}{unit}</span>
        <span className="text-[10px] text-slate-600">{max}{unit}</span>
      </div>
    </div>
  );
};
