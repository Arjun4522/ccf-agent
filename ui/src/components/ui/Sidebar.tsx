import React, { useMemo } from 'react';
import { motion } from 'framer-motion';
import {
  LayoutDashboard, Radio, BarChart3, Shield, Archive,
  Network, Settings, ChevronLeft, ChevronRight,
  Bell, Wifi, WifiOff,
  Eye, ShieldCheck,
} from 'lucide-react';
import { useStore } from '../../store';
import type { NavPage } from '../../types';
import { Badge } from '../ui/Badge';
import { StatusDot } from '../ui/Indicators';
import { useState } from 'react';

interface NavItem {
  page: NavPage;
  label: string;
  icon: React.FC<{ size?: number; strokeWidth?: number }>;
  badge?: string;
  badgeVariant?: 'red' | 'yellow' | 'green' | 'blue';
}

const NAV_ITEMS: NavItem[] = [
  { page: 'dashboard',  label: 'Overview',    icon: LayoutDashboard },
  { page: 'detections', label: 'Detections',  icon: Radio },
  { page: 'analytics',  label: 'Analytics',   icon: BarChart3 },
  { page: 'response',   label: 'Response',    icon: Shield },
  { page: 'quarantine', label: 'Quarantine',  icon: Archive },
  { page: 'field',      label: 'Field View',  icon: Network },
  { page: 'settings',   label: 'Settings',    icon: Settings },
];

export const Sidebar: React.FC = () => {
  const activePage = useStore(s => s.activePage);
  const setActivePage = useStore(s => s.setActivePage);
  const detections = useStore(s => s.detections);
  const quarantine = useStore(s => s.quarantine);
  const wsConnected = useStore(s => s.wsConnected);
  const status = useStore(s => s.status);
  const notifications = useStore(s => s.notifications);
  const markAllRead = useStore(s => s.markAllRead);
  const role = useStore(s => s.role);
  const [collapsed, setCollapsed] = useState(false);
  const [showNotif, setShowNotif] = useState(false);

  const alertCount = useMemo(() => detections.filter(d => d.severity === 'ALERT').length, [detections]);
  const warnCount  = useMemo(() => detections.filter(d => d.severity === 'WARNING').length, [detections]);
  const unreadCount = useMemo(() => notifications.filter(n => !n.read).length, [notifications]);

  const navWithBadges = NAV_ITEMS.map(item => {
    if (item.page === 'detections' && alertCount > 0) {
      return { ...item, badge: String(alertCount), badgeVariant: 'red' as const };
    }
    if (item.page === 'detections' && warnCount > 0) {
      return { ...item, badge: String(warnCount), badgeVariant: 'yellow' as const };
    }
    if (item.page === 'quarantine' && quarantine.length > 0) {
      return { ...item, badge: String(quarantine.length), badgeVariant: 'red' as const };
    }
    return item;
  });

  const statusColor =
    status.status === 'ALERT'   ? 'red' as const :
    status.status === 'RUNNING' ? 'green' as const :
    'yellow' as const;

  return (
    <motion.aside
      animate={{ width: collapsed ? 56 : 200 }}
      transition={{ type: 'spring', stiffness: 300, damping: 30 }}
      className="flex flex-col h-full bg-surface-800 border-r border-border relative shrink-0 overflow-hidden"
    >
      {/* Logo */}
      <div className={`flex items-center gap-2.5 p-3 border-b border-border h-12 ${collapsed ? 'justify-center' : ''}`}>
        <div className="w-7 h-7 rounded-lg bg-neon-red/20 border border-neon-red/40 flex items-center justify-center shrink-0 glow-red">
          <ShieldCheck size={14} className="text-neon-red" />
        </div>
        {!collapsed && (
          <div className="min-w-0">
            <div className="text-xs font-bold text-slate-100 tracking-tight leading-none">CCF Agent</div>
            <div className="text-[9px] text-slate-500 mt-0.5">Ransomware Detection</div>
          </div>
        )}
      </div>

      {/* Status pill */}
      {!collapsed && (
        <div className="mx-3 mt-2.5 mb-1 flex items-center gap-2 bg-surface-700 rounded-lg p-2 border border-border">
          <StatusDot status={statusColor} size={7} />
          <div className="flex-1 min-w-0">
            <div className="text-[9px] font-mono text-slate-300 font-semibold">{status.status}</div>
            <div className="text-[9px] text-slate-600">
              {status.eventsPerSecond.toLocaleString()} ev/s
            </div>
          </div>
          {wsConnected
            ? <Wifi size={10} className="text-neon-green shrink-0" />
            : <WifiOff size={10} className="text-slate-600 shrink-0" />}
        </div>
      )}

      {/* Nav items */}
      <nav className="flex flex-col gap-0.5 px-2 py-2 flex-1 overflow-y-auto">
        {navWithBadges.map((item, idx) => {
          const isActive = activePage === item.page;
          const Icon = item.icon;
          return (
            <button
              key={item.page}
              onClick={() => setActivePage(item.page)}
              title={collapsed ? `${item.label} [${idx + 1}]` : undefined}
              className={`
                flex items-center gap-2.5 rounded-lg transition-all duration-150 relative
                ${collapsed ? 'px-0 py-2 justify-center' : 'px-2.5 py-2'}
                ${isActive
                  ? 'bg-neon-blue/15 text-neon-blue border border-neon-blue/25'
                  : 'text-slate-500 hover:text-slate-200 hover:bg-surface-600 border border-transparent'
                }
              `}
            >
              {isActive && (
                <motion.div
                  layoutId="activeIndicator"
                  className="absolute left-0 w-0.5 h-5 bg-neon-blue rounded-r-full"
                />
              )}
              <Icon size={14} strokeWidth={isActive ? 2.5 : 2} />
              {!collapsed && (
                <>
                  <span className="text-xs font-medium flex-1 text-left">{item.label}</span>
                  {item.badge && (
                    <Badge label={item.badge} variant={item.badgeVariant ?? 'gray'} size="xs" />
                  )}
                  <span className="text-[9px] text-slate-700 font-mono">{idx + 1}</span>
                </>
              )}
              {collapsed && item.badge && (
                <span className="absolute top-0.5 right-0.5 w-3.5 h-3.5 rounded-full bg-neon-red text-[8px] text-white flex items-center justify-center font-bold leading-none">
                  {Number(item.badge) > 9 ? '9+' : item.badge}
                </span>
              )}
            </button>
          );
        })}
      </nav>

      {/* Bottom bar */}
      <div className={`border-t border-border p-2 flex items-center ${collapsed ? 'justify-center flex-col gap-2' : 'gap-2'}`}>
        {/* Notifications */}
        <div className="relative">
          <button
            onClick={() => { setShowNotif(s => !s); if (unreadCount > 0) markAllRead(); }}
            className="flex items-center justify-center w-7 h-7 rounded-lg hover:bg-surface-600 text-slate-500 hover:text-slate-300 transition-colors relative"
            title="Notifications"
          >
            <Bell size={13} />
            {unreadCount > 0 && (
              <span className="absolute top-0 right-0 w-3 h-3 rounded-full bg-neon-red text-[7px] text-white flex items-center justify-center font-bold leading-none animate-pulse-red">
                {unreadCount > 9 ? '9' : unreadCount}
              </span>
            )}
          </button>

          {showNotif && !collapsed && (
            <motion.div
              initial={{ opacity: 0, y: 6 }}
              animate={{ opacity: 1, y: 0 }}
              className="absolute bottom-full left-0 mb-1 w-64 glass rounded-xl border border-border shadow-xl z-50 overflow-hidden"
            >
              <div className="p-2 border-b border-border flex items-center justify-between">
                <span className="text-[10px] font-semibold text-slate-400 uppercase tracking-widest">Notifications</span>
                <button onClick={() => useStore.getState().clearNotifications()} className="text-[9px] text-slate-600 hover:text-slate-400">
                  Clear all
                </button>
              </div>
              <div className="max-h-64 overflow-y-auto">
                {notifications.length === 0 ? (
                  <div className="py-6 text-center text-xs text-slate-600">No notifications</div>
                ) : (
                  notifications.slice(0, 15).map(n => (
                    <div key={n.id} className="flex items-start gap-2 p-2.5 border-b border-border/50 hover:bg-surface-700">
                      <div className={`w-1.5 h-1.5 rounded-full mt-1 shrink-0 ${
                        n.type === 'error' ? 'bg-neon-red' :
                        n.type === 'warning' ? 'bg-neon-yellow' :
                        n.type === 'success' ? 'bg-neon-green' :
                        'bg-neon-blue'
                      }`} />
                      <div className="flex-1 min-w-0">
                        <p className="text-[10px] font-medium text-slate-300 leading-tight">{n.title}</p>
                        <p className="text-[9px] text-slate-500 mt-0.5 font-mono break-all leading-tight">{n.message}</p>
                      </div>
                    </div>
                  ))
                )}
              </div>
            </motion.div>
          )}
        </div>

        {/* Role badge */}
        {!collapsed && (
          <div className="flex items-center gap-1.5 text-[9px] text-slate-600 flex-1">
            {role === 'admin' ? <ShieldCheck size={10} className="text-neon-green" /> : <Eye size={10} />}
            <span className="font-mono">{role}</span>
          </div>
        )}

        {/* Collapse toggle */}
        <button
          onClick={() => setCollapsed(c => !c)}
          className="flex items-center justify-center w-7 h-7 rounded-lg hover:bg-surface-600 text-slate-600 hover:text-slate-400 transition-colors"
          title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
        >
          {collapsed ? <ChevronRight size={12} /> : <ChevronLeft size={12} />}
        </button>
      </div>
    </motion.aside>
  );
};
