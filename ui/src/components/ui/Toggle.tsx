import React from 'react';
import { motion } from 'framer-motion';

interface ToggleProps {
  checked: boolean;
  onChange: (v: boolean) => void;
  label?: string;
  description?: string;
  disabled?: boolean;
  color?: 'green' | 'blue' | 'red' | 'yellow' | 'purple';
}

const COLORS = {
  green:  { on: 'bg-neon-green',  shadow: 'shadow-neon-green/40' },
  blue:   { on: 'bg-neon-blue',   shadow: 'shadow-neon-blue/40' },
  red:    { on: 'bg-neon-red',    shadow: 'shadow-neon-red/40' },
  yellow: { on: 'bg-neon-yellow', shadow: 'shadow-neon-yellow/40' },
  purple: { on: 'bg-neon-purple', shadow: 'shadow-neon-purple/40' },
};

export const Toggle: React.FC<ToggleProps> = ({
  checked, onChange, label, description, disabled = false, color = 'green',
}) => {
  const { on, shadow } = COLORS[color];

  return (
    <div
      className={`flex items-center justify-between gap-3 ${disabled ? 'opacity-40 cursor-not-allowed' : 'cursor-pointer'}`}
      onClick={() => !disabled && onChange(!checked)}
    >
      <div className="flex-1 min-w-0">
        {label && (
          <span className="text-sm text-slate-200 font-medium select-none block truncate">{label}</span>
        )}
        {description && (
          <span className="text-xs text-slate-500 select-none block truncate">{description}</span>
        )}
      </div>
      <div
        className={`relative flex-shrink-0 w-10 h-5 rounded-full transition-colors duration-200 ${
          checked ? on : 'bg-surface-400'
        } ${checked ? `shadow-md ${shadow}` : ''}`}
      >
        <motion.div
          layout
          transition={{ type: 'spring', stiffness: 500, damping: 40 }}
          className="absolute top-0.5 w-4 h-4 rounded-full bg-white shadow-sm"
          style={{ left: checked ? '22px' : '2px' }}
        />
      </div>
    </div>
  );
};
