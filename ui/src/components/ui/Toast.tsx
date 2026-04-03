import React from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { X, AlertTriangle, CheckCircle, Info, AlertCircle } from 'lucide-react';
import type { Notification } from '../../types';
import { useStore } from '../../store';

const ICONS = {
  info:    { icon: Info,          color: 'text-neon-blue',   bg: 'bg-neon-blue/10',   border: 'border-neon-blue/20' },
  success: { icon: CheckCircle,   color: 'text-neon-green',  bg: 'bg-neon-green/10',  border: 'border-neon-green/20' },
  warning: { icon: AlertTriangle, color: 'text-neon-yellow', bg: 'bg-neon-yellow/10', border: 'border-neon-yellow/20' },
  error:   { icon: AlertCircle,   color: 'text-neon-red',    bg: 'bg-neon-red/10',    border: 'border-neon-red/20' },
};

interface ToastItemProps {
  notification: Notification;
  onDismiss: (id: string) => void;
}

const ToastItem: React.FC<ToastItemProps> = ({ notification, onDismiss }) => {
  const { icon: Icon, color, bg, border } = ICONS[notification.type];

  React.useEffect(() => {
    const t = setTimeout(() => onDismiss(notification.id), 6000);
    return () => clearTimeout(t);
  }, [notification.id, onDismiss]);

  return (
    <motion.div
      layout
      initial={{ opacity: 0, x: 60, scale: 0.95 }}
      animate={{ opacity: 1, x: 0, scale: 1 }}
      exit={{ opacity: 0, x: 60, scale: 0.9 }}
      transition={{ type: 'spring', stiffness: 300, damping: 30 }}
      className={`flex items-start gap-3 p-3 rounded-xl border ${bg} ${border} glass max-w-sm w-full shadow-xl`}
    >
      <Icon size={16} className={`${color} mt-0.5 shrink-0`} />
      <div className="flex-1 min-w-0">
        <p className="text-xs font-semibold text-slate-200 leading-tight">{notification.title}</p>
        {notification.message && (
          <p className="text-[10px] text-slate-400 mt-0.5 leading-tight font-mono break-all">
            {notification.message}
          </p>
        )}
      </div>
      <button
        onClick={() => onDismiss(notification.id)}
        className="text-slate-500 hover:text-slate-300 transition-colors shrink-0"
      >
        <X size={12} />
      </button>
    </motion.div>
  );
};

export const ToastContainer: React.FC = () => {
  const allNotifications = useStore(s => s.notifications);
  const notifications = React.useMemo(
    () => allNotifications.filter(n => !n.read).slice(0, 5),
    [allNotifications]
  );

  const dismiss = React.useCallback((id: string) => {
    useStore.setState(state => ({
      notifications: state.notifications.map(n =>
        n.id === id ? { ...n, read: true } : n
      ),
    }));
  }, []);

  return (
    <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2 pointer-events-none">
      <AnimatePresence mode="popLayout">
        {notifications.map(n => (
          <div key={n.id} className="pointer-events-auto">
            <ToastItem notification={n} onDismiss={dismiss} />
          </div>
        ))}
      </AnimatePresence>
    </div>
  );
};
