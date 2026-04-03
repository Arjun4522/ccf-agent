import React, { useMemo, useEffect, useState } from 'react';
import { motion } from 'framer-motion';
import { Pause, Play, Filter, Search, Download, Trash2, ChevronDown, ChevronRight, ChevronLeft } from 'lucide-react';
import { Card } from '../ui/Card';
import { SeverityBadge, ActionBadge } from '../ui/Badge';
import { Button } from '../ui/Button';
import { ScoreBar } from '../ui/Indicators';
import { useStore } from '../../store';
import { useExport } from '../../hooks/useExport';
import { formatTimestamp } from '../../utils/mockData';
import type { Detection } from '../../types';

const PAGE_SIZE = 25;

const DetectionRow: React.FC<{ detection: Detection; isNew?: boolean }> = React.memo(
  ({ detection: d, isNew }) => {
    const [expanded, setExpanded] = useState(false);

    const rowBg =
      d.severity === 'ALERT'   ? 'hover:bg-neon-red/5 border-l-2 border-l-neon-red/50' :
      d.severity === 'WARNING' ? 'hover:bg-neon-yellow/5 border-l-2 border-l-neon-yellow/40' :
      'hover:bg-white/3 border-l-2 border-l-transparent';

    return (
      <>
        <motion.tr
          initial={isNew ? { opacity: 0, backgroundColor: 'rgba(0,212,255,0.08)' } : false}
          animate={{ opacity: 1, backgroundColor: 'rgba(0,0,0,0)' }}
          transition={{ duration: 0.8 }}
          className={`text-xs cursor-pointer ${rowBg} transition-colors`}
          onClick={() => setExpanded(e => !e)}
        >
          <td className="px-3 py-2 text-slate-500 font-mono whitespace-nowrap">
            {formatTimestamp(d.timestamp)}
          </td>
          <td className="px-3 py-2">
            <SeverityBadge severity={d.severity} />
          </td>
          <td className="px-3 py-2 w-28">
            <ScoreBar value={d.score} height={5} />
          </td>
          <td className="px-3 py-2 font-mono text-neon-blue whitespace-nowrap">
            {d.vector.cfer.toFixed(3)}
          </td>
          <td className="px-3 py-2 font-mono text-neon-purple whitespace-nowrap">
            {d.vector.turbulence.toFixed(3)}
          </td>
          <td className="px-3 py-2 font-mono text-neon-yellow whitespace-nowrap">
            {d.vector.shockwave.toFixed(3)}
          </td>
          <td className="px-3 py-2 font-mono text-slate-300 whitespace-nowrap">
            {d.vector.entropy.toFixed(3)}
          </td>
          <td className="px-3 py-2 font-mono text-slate-400">{d.vector.activeNodes}</td>
          <td className="px-3 py-2 font-mono text-slate-200 whitespace-nowrap">
            {d.vector.offenderPID}
            {d.processName && (
              <span className="ml-1.5 text-slate-500 text-[10px]">({d.processName})</span>
            )}
          </td>
          <td className="px-3 py-2">
            <ActionBadge action={d.action} />
          </td>
          <td className="px-3 py-2 text-slate-500">
            {expanded ? <ChevronDown size={10} /> : <ChevronRight size={10} />}
          </td>
        </motion.tr>
        {expanded && (
          <tr>
            <td colSpan={11} className="px-4 pb-3">
              <div className="bg-surface-700 rounded-lg p-3 font-mono text-[10px] text-slate-400 leading-relaxed border border-border">
                <span className="text-slate-500">REASON: </span>
                <span className="text-slate-300">{d.reason}</span>
                <br />
                <span className="text-slate-500">ID: </span>
                <span className="text-slate-400">{d.id}</span>
                <span className="ml-4 text-slate-500">PPID: </span>
                <span className="text-slate-400">{d.vector.parentPID}</span>
              </div>
            </td>
          </tr>
        )}
      </>
    );
  }
);
DetectionRow.displayName = 'DetectionRow';

export const DetectionStream: React.FC = () => {
  const detections = useStore(s => s.detections);
  const streamPaused = useStore(s => s.streamPaused);
  const toggleStreamPause = useStore(s => s.toggleStreamPause);
  const severityFilter = useStore(s => s.severityFilter);
  const setSeverityFilter = useStore(s => s.setSeverityFilter);
  const pidSearch = useStore(s => s.pidSearch);
  const setPidSearch = useStore(s => s.setPidSearch);
  const clearDetections = useStore(s => s.clearDetections);
  const { exportJSON, exportCSV } = useExport();

  const [page, setPage] = useState(1);

  // Reset to page 1 whenever filters change
  useEffect(() => {
    setPage(1);
  }, [severityFilter, pidSearch]);

  const filtered = useMemo(() => {
    let list = detections;
    if (severityFilter !== 'ALL') {
      list = list.filter(d => d.severity === severityFilter);
    }
    if (pidSearch.trim()) {
      const q = pidSearch.trim().toLowerCase();
      list = list.filter(d =>
        String(d.vector.offenderPID).includes(q) ||
        (d.processName ?? '').toLowerCase().includes(q)
      );
    }
    return list;
  }, [detections, severityFilter, pidSearch]);

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, totalPages);
  const pageItems = filtered.slice((safePage - 1) * PAGE_SIZE, safePage * PAGE_SIZE);

  const filterButtons: { label: string; value: typeof severityFilter }[] = [
    { label: 'ALL', value: 'ALL' },
    { label: 'ALERT', value: 'ALERT' },
    { label: 'WARNING', value: 'WARNING' },
  ];

  return (
    <Card
      title="Real-Time Detection Stream"
      subtitle={`${filtered.length} events ${streamPaused ? '(PAUSED)' : '(LIVE)'}`}
      glow={detections.some(d => d.severity === 'ALERT') ? 'red' : 'none'}
      className="h-full"
      actions={
        <div className="flex items-center gap-2">
          <Button
            variant={streamPaused ? 'warning' : 'ghost'}
            size="xs"
            icon={streamPaused ? Play : Pause}
            onClick={toggleStreamPause}
          >
            {streamPaused ? 'Resume' : 'Pause'}
          </Button>
          <Button variant="ghost" size="xs" icon={Download} onClick={exportCSV}>CSV</Button>
          <Button variant="ghost" size="xs" icon={Download} onClick={exportJSON}>JSON</Button>
          <Button variant="danger" size="xs" icon={Trash2} onClick={clearDetections}>Clear</Button>
        </div>
      }
    >
      {/* Filters */}
      <div className="flex items-center gap-3 flex-wrap">
        <div className="flex items-center gap-1.5 bg-surface-700 rounded-lg p-1 border border-border">
          <Filter size={10} className="text-slate-500 ml-1" />
          {filterButtons.map(({ label, value }) => (
            <button
              key={value}
              onClick={() => setSeverityFilter(value)}
              className={`px-2 py-0.5 text-[10px] rounded font-mono font-medium transition-all ${
                severityFilter === value
                  ? value === 'ALERT'   ? 'bg-neon-red/20 text-neon-red'
                  : value === 'WARNING' ? 'bg-neon-yellow/20 text-neon-yellow'
                  : 'bg-neon-blue/20 text-neon-blue'
                  : 'text-slate-500 hover:text-slate-300'
              }`}
            >
              {label}
            </button>
          ))}
        </div>

        <div className="flex items-center gap-1.5 bg-surface-700 rounded-lg px-2 py-1 border border-border flex-1 max-w-48">
          <Search size={10} className="text-slate-500 shrink-0" />
          <input
            type="text"
            value={pidSearch}
            onChange={e => setPidSearch(e.target.value)}
            placeholder="Search PID / process..."
            className="bg-transparent text-[11px] text-slate-300 placeholder-slate-600 outline-none w-full font-mono"
          />
        </div>

        {streamPaused && (
          <span className="text-[10px] text-neon-yellow font-mono animate-pulse-yellow">
            ⏸ Stream paused — press SPACE to resume
          </span>
        )}
      </div>

      {/* Table */}
      <div className="flex-1 min-h-0 overflow-x-auto rounded-lg border border-border">
        <table className="w-full border-collapse" style={{ minWidth: 860 }}>
          <thead className="sticky top-0 z-10">
            <tr className="bg-surface-800 border-b border-border">
              {[
                'Timestamp', 'Severity', 'Score', 'CFER', 'Turbulence',
                'Shockwave', 'Entropy', 'Nodes', 'PID / Process', 'Action', '',
              ].map(h => (
                <th
                  key={h}
                  className="px-3 py-2 text-left text-[10px] uppercase tracking-widest text-slate-500 font-medium whitespace-nowrap"
                >
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {pageItems.length === 0 ? (
              <tr>
                <td colSpan={11} className="text-center py-12 text-slate-600 text-sm">
                  No detections match current filters
                </td>
              </tr>
            ) : (
              pageItems.map((d, i) => (
                <DetectionRow key={d.id} detection={d} isNew={i === 0 && safePage === 1 && !streamPaused} />
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Footer — pagination */}
      <div className="flex items-center justify-between text-[10px] text-slate-600 font-mono">
        <span>
          {filtered.length === 0
            ? 'No detections'
            : `${(safePage - 1) * PAGE_SIZE + 1}–${Math.min(safePage * PAGE_SIZE, filtered.length)} of ${filtered.length} detections`}
        </span>

        <div className="flex items-center gap-1">
          <button
            onClick={() => setPage(p => Math.max(1, p - 1))}
            disabled={safePage === 1}
            className="p-0.5 rounded text-slate-500 hover:text-slate-300 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
          >
            <ChevronLeft size={12} />
          </button>
          <span className="px-2 text-slate-400">
            {safePage} / {totalPages}
          </span>
          <button
            onClick={() => setPage(p => Math.min(totalPages, p + 1))}
            disabled={safePage === totalPages}
            className="p-0.5 rounded text-slate-500 hover:text-slate-300 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
          >
            <ChevronRight size={12} />
          </button>
        </div>

        <span>Click row for details</span>
      </div>
    </Card>
  );
};
