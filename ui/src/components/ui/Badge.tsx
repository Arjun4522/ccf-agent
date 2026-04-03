import React from 'react';
import type { Severity, ActionTaken } from '../../types';

interface BadgeProps {
  label: string;
  variant?: 'green' | 'yellow' | 'red' | 'blue' | 'purple' | 'gray';
  size?: 'xs' | 'sm';
  pulse?: boolean;
  dot?: boolean;
}

const VARIANTS = {
  green:  'bg-neon-green/10  text-neon-green  border-neon-green/30',
  yellow: 'bg-neon-yellow/10 text-neon-yellow border-neon-yellow/30',
  red:    'bg-neon-red/10    text-neon-red    border-neon-red/30',
  blue:   'bg-neon-blue/10   text-neon-blue   border-neon-blue/30',
  purple: 'bg-neon-purple/10 text-neon-purple border-neon-purple/30',
  gray:   'bg-slate-700/40   text-slate-400   border-slate-600/30',
};

const DOT_COLORS = {
  green:  'bg-neon-green animate-pulse-green',
  yellow: 'bg-neon-yellow animate-pulse-yellow',
  red:    'bg-neon-red animate-pulse-red',
  blue:   'bg-neon-blue',
  purple: 'bg-neon-purple',
  gray:   'bg-slate-500',
};

export const Badge: React.FC<BadgeProps> = ({
  label, variant = 'gray', size = 'sm', pulse = false, dot = false,
}) => {
  const sizeClass = size === 'xs' ? 'text-[10px] px-1.5 py-0.5' : 'text-xs px-2 py-0.5';
  const pulseClass = pulse
    ? variant === 'red'    ? 'animate-pulse-red'
    : variant === 'yellow' ? 'animate-pulse-yellow'
    : variant === 'green'  ? 'animate-pulse-green'
    : ''
    : '';

  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-md border font-mono font-medium ${sizeClass} ${VARIANTS[variant]} ${pulseClass}`}
    >
      {dot && (
        <span className={`w-1.5 h-1.5 rounded-full ${DOT_COLORS[variant]}`} />
      )}
      {label}
    </span>
  );
};

// Convenience wrappers
export const SeverityBadge: React.FC<{ severity: Severity }> = ({ severity }) => {
  const map = {
    ALERT:   { variant: 'red'    as const, pulse: true, dot: true },
    WARNING: { variant: 'yellow' as const, pulse: true, dot: true },
    NONE:    { variant: 'gray'   as const, pulse: false, dot: false },
  };
  const { variant, pulse, dot } = map[severity];
  return <Badge label={severity} variant={variant} pulse={pulse} dot={dot} />;
};

export const ActionBadge: React.FC<{ action: ActionTaken }> = ({ action }) => {
  const map: Record<ActionTaken, { variant: BadgeProps['variant']; label: string }> = {
    SIGSTOP:    { variant: 'yellow', label: 'SIGSTOP' },
    SIGKILL:    { variant: 'red',    label: 'SIGKILL' },
    QUARANTINE: { variant: 'purple', label: 'QUARANTINE' },
    NONE:       { variant: 'gray',   label: 'NONE' },
  };
  const { variant, label } = map[action];
  return <Badge label={label} variant={variant} />;
};
