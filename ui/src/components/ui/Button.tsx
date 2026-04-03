import React from 'react';
import { motion } from 'framer-motion';
import type { LucideIcon } from 'lucide-react';

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'danger' | 'warning' | 'ghost' | 'outline';
  size?: 'xs' | 'sm' | 'md';
  icon?: LucideIcon;
  iconRight?: LucideIcon;
  loading?: boolean;
  children?: React.ReactNode;
}

const VARIANTS = {
  primary: 'bg-neon-blue/15 hover:bg-neon-blue/25 text-neon-blue border-neon-blue/30 hover:border-neon-blue/60',
  danger:  'bg-neon-red/15  hover:bg-neon-red/25  text-neon-red  border-neon-red/30  hover:border-neon-red/60',
  warning: 'bg-neon-yellow/10 hover:bg-neon-yellow/20 text-neon-yellow border-neon-yellow/30 hover:border-neon-yellow/60',
  ghost:   'bg-transparent hover:bg-white/5 text-slate-400 hover:text-slate-200 border-transparent',
  outline: 'bg-transparent hover:bg-surface-600 text-slate-300 border-border hover:border-border-bright',
};

const SIZES = {
  xs: 'text-[10px] px-2 py-1 gap-1',
  sm: 'text-xs px-3 py-1.5 gap-1.5',
  md: 'text-sm px-4 py-2 gap-2',
};

export const Button: React.FC<ButtonProps> = ({
  variant = 'outline', size = 'sm', icon: Icon, iconRight: IconRight,
  loading = false, children, className = '', disabled, ...rest
}) => {
  return (
    <motion.button
      whileTap={{ scale: 0.96 }}
      disabled={disabled || loading}
      className={`
        inline-flex items-center justify-center rounded-lg border font-mono font-medium
        transition-all duration-150 cursor-pointer select-none
        disabled:opacity-40 disabled:cursor-not-allowed
        ${VARIANTS[variant]} ${SIZES[size]} ${className}
      `}
      {...(rest as React.ComponentProps<typeof motion.button>)}
    >
      {loading ? (
        <span className="w-3 h-3 border border-current border-t-transparent rounded-full animate-spin" />
      ) : (
        Icon && <Icon size={size === 'xs' ? 10 : size === 'sm' ? 12 : 14} strokeWidth={2} />
      )}
      {children}
      {IconRight && <IconRight size={size === 'xs' ? 10 : size === 'sm' ? 12 : 14} strokeWidth={2} />}
    </motion.button>
  );
};
