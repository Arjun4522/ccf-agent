import React from 'react';
import { AnimatePresence, motion } from 'framer-motion';
import { AlertOctagon } from 'lucide-react';
import { useStore } from '../store';
import { SystemOverview } from '../components/dashboard/SystemOverview';
import { DetectionStream } from '../components/detection/DetectionStream';
import { AnalyticsPanel } from '../components/analytics/AnalyticsPanel';
import { ResponseControlPanel } from '../components/response/ResponseControlPanel';
import { QuarantineManager } from '../components/quarantine/QuarantineManager';
import { FieldVisualization } from '../components/field/FieldVisualization';
import { SettingsPanel } from '../components/settings/SettingsPanel';
import { formatUptime } from '../utils/mockData';

const PAGE_TITLES: Record<string, string> = {
  dashboard:  'System Overview',
  detections: 'Detection Stream',
  analytics:  'Behavioral Analytics',
  response:   'Response Control',
  quarantine: 'Quarantine Manager',
  field:      'Field Visualization',
  settings:   'Settings',
};

const pageVariants = {
  initial: { opacity: 0, y: 8 },
  animate: { opacity: 1, y: 0 },
  exit:    { opacity: 0, y: -8 },
};

export const DashboardPage: React.FC = () => {
  const activePage = useStore(s => s.activePage);
  const status = useStore(s => s.status);
  const detections = useStore(s => s.detections);
  const recentAlert = detections.find(d => d.severity === 'ALERT');

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      {/* Top bar */}
      <header className="flex items-center justify-between px-5 h-12 border-b border-border bg-surface-800 shrink-0">
        <h1 className="text-sm font-semibold text-slate-200 tracking-wide">
          {PAGE_TITLES[activePage]}
        </h1>
        <div className="flex items-center gap-3 text-[10px] font-mono">
          <span className="text-slate-500">
            Uptime: <span className="text-slate-300">{formatUptime(status.uptime)}</span>
          </span>
          {recentAlert && (
            <>
              <span className="text-slate-700">|</span>
              <span className="flex items-center gap-1 text-neon-red">
                <AlertOctagon size={10} className="animate-pulse-red" />
                Last alert: PID {recentAlert.vector.offenderPID} — {recentAlert.processName}
              </span>
            </>
          )}
          <span className="text-slate-700">|</span>
          <span className="text-slate-600">CCF Agent v2.1</span>
          <span className="text-slate-700">|</span>
          <span className="text-slate-600">SPACE=pause · 1-7=nav · Ctrl+L=clear</span>
        </div>
      </header>

      {/* Page content */}
      <main className="flex-1 overflow-y-auto p-4">
        <AnimatePresence mode="wait">
          <motion.div
            key={activePage}
            variants={pageVariants}
            initial="initial"
            animate="animate"
            exit="exit"
            transition={{ duration: 0.15 }}
            className="h-full"
          >
            {activePage === 'dashboard'  && <SystemOverview />}
            {activePage === 'detections' && <DetectionStream />}
            {activePage === 'analytics'  && <AnalyticsPanel />}
            {activePage === 'response'   && <ResponseControlPanel />}
            {activePage === 'quarantine' && <QuarantineManager />}
            {activePage === 'field'      && <FieldVisualization />}
            {activePage === 'settings'   && <SettingsPanel />}
          </motion.div>
        </AnimatePresence>
      </main>
    </div>
  );
};
