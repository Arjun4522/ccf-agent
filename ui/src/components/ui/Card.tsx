import React from 'react';
import { motion } from 'framer-motion';

interface CardProps {
  title?: string;
  subtitle?: string;
  children: React.ReactNode;
  className?: string;
  glow?: 'green' | 'red' | 'yellow' | 'blue' | 'none';
  actions?: React.ReactNode;
  compact?: boolean;
}

export const Card: React.FC<CardProps> = ({
  title, subtitle, children, className = '', glow = 'none', actions, compact = false,
}) => {
  const glowClass =
    glow === 'green' ? 'glow-green border-neon-green/20' :
    glow === 'red'   ? 'glow-red border-neon-red/30' :
    glow === 'yellow'? 'glow-yellow border-neon-yellow/20' :
    glow === 'blue'  ? 'glow-blue border-neon-blue/20' :
    'border-border';

  return (
    <motion.div
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.25 }}
      className={`glass rounded-xl ${glowClass} border ${compact ? 'p-3' : 'p-4'} flex flex-col gap-3 ${className}`}
    >
      {(title || actions) && (
        <div className="flex items-center justify-between gap-2 min-w-0">
          <div className="min-w-0">
            {title && (
              <h3 className="text-xs font-semibold uppercase tracking-widest text-slate-400 truncate">
                {title}
              </h3>
            )}
            {subtitle && (
              <p className="text-xs text-slate-500 mt-0.5 truncate">{subtitle}</p>
            )}
          </div>
          {actions && <div className="flex items-center gap-2 shrink-0">{actions}</div>}
        </div>
      )}
      {children}
    </motion.div>
  );
};
