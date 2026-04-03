import React, { useState, useEffect } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { Archive, Trash2, RotateCcw, FileWarning, Clock, HardDrive } from 'lucide-react';
import { Card } from '../ui/Card';
import { Button } from '../ui/Button';
import { Badge } from '../ui/Badge';
import { useStore } from '../../store';
import { restoreFile, deleteFile, getQuarantine } from '../../services/api';
import { formatBytes, formatTimestamp } from '../../utils/mockData';
import type { QuarantinedFile } from '../../types';

const FileRow: React.FC<{
  file: QuarantinedFile;
  onRestore: (id: string) => void;
  onDelete: (id: string) => void;
  isAdmin: boolean;
}> = ({ file, onRestore, onDelete, isAdmin }) => {
  const [expanded, setExpanded] = useState(false);
  const isActing = file.status !== 'quarantined';

  return (
    <AnimatePresence>
      <motion.div
        layout
        initial={{ opacity: 0, x: -10 }}
        animate={{ opacity: 1, x: 0 }}
        exit={{ opacity: 0, x: 10, height: 0 }}
        className="bg-surface-700 rounded-lg border border-border overflow-hidden"
      >
        <div
          className="flex items-center gap-3 p-3 cursor-pointer hover:bg-surface-600 transition-colors"
          onClick={() => setExpanded(e => !e)}
        >
          <FileWarning size={16} className="text-neon-red shrink-0 opacity-80" />
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 flex-wrap">
              <span className="text-xs font-mono text-slate-200 truncate">{file.path}</span>
              <Badge label="QUARANTINED" variant="red" size="xs" />
            </div>
            <div className="flex items-center gap-3 mt-0.5 text-[10px] text-slate-500">
              <span className="flex items-center gap-1">
                <Clock size={9} />{formatTimestamp(file.quarantinedAt)}
              </span>
              <span className="flex items-center gap-1">
                <HardDrive size={9} />{formatBytes(file.size)}
              </span>
              <span className="font-mono">PID {file.originPID} ({file.processName})</span>
            </div>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <Button
              variant="primary"
              size="xs"
              icon={RotateCcw}
              disabled={!isAdmin || isActing}
              loading={file.status === 'restoring'}
              onClick={e => { e.stopPropagation(); onRestore(file.id); }}
            >
              Restore
            </Button>
            <Button
              variant="danger"
              size="xs"
              icon={Trash2}
              disabled={!isAdmin || isActing}
              loading={file.status === 'deleting'}
              onClick={e => { e.stopPropagation(); onDelete(file.id); }}
            >
              Delete
            </Button>
          </div>
        </div>

        {expanded && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            className="border-t border-border px-4 pb-3 pt-2"
          >
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-2">
              <div className="flex flex-col gap-1">
                <span className="text-[9px] uppercase tracking-widest text-slate-600">Full Path</span>
                <span className="font-mono text-[10px] text-slate-300 break-all">{file.path}</span>
              </div>
              <div className="flex flex-col gap-1">
                <span className="text-[9px] uppercase tracking-widest text-slate-600">SHA-256 Hash</span>
                <span className="font-mono text-[10px] text-slate-300 break-all">{file.hash}</span>
              </div>
              <div className="flex flex-col gap-1">
                <span className="text-[9px] uppercase tracking-widest text-slate-600">Origin Process</span>
                <span className="font-mono text-[10px] text-slate-300">
                  {file.processName} (PID {file.originPID})
                </span>
              </div>
              <div className="flex flex-col gap-1">
                <span className="text-[9px] uppercase tracking-widest text-slate-600">Quarantined At</span>
                <span className="font-mono text-[10px] text-slate-300">
                  {new Date(file.quarantinedAt).toLocaleString()}
                </span>
              </div>
            </div>
          </motion.div>
        )}
      </motion.div>
    </AnimatePresence>
  );
};

export const QuarantineManager: React.FC = () => {
  const quarantine = useStore(s => s.quarantine);
  const setQuarantine = useStore(s => s.setQuarantine);
  const removeQuarantineFile = useStore(s => s.removeQuarantineFile);
  const role = useStore(s => s.role);
  const mockMode = useStore(s => s.mockMode);
  const addNotification = useStore(s => s.addNotification);
  const isAdmin = role === 'admin';

  // Load quarantine list on mount (real mode only).
  useEffect(() => {
    if (mockMode) return;
    getQuarantine().then(res => {
      if (res.ok) setQuarantine(res.data ?? []);
    });
  }, [mockMode]); // eslint-disable-line react-hooks/exhaustive-deps

  const totalSize = quarantine.reduce((sum, f) => sum + f.size, 0);

  const handleRestore = async (id: string) => {
    setQuarantine(quarantine.map(f => f.id === id ? { ...f, status: 'restoring' as const } : f));
    if (!mockMode) {
      const res = await restoreFile(id);
      if (!res.ok) {
        setQuarantine(quarantine.map(f => f.id === id ? { ...f, status: 'quarantined' as const } : f));
        addNotification({ type: 'error', title: 'Restore Failed', message: res.error ?? '' });
        return;
      }
    } else {
      await delay(800);
    }
    removeQuarantineFile(id);
    addNotification({ type: 'success', title: 'File Restored', message: `File ${id} restored to original location` });
  };

  const handleDelete = async (id: string) => {
    const file = quarantine.find(f => f.id === id);
    if (!window.confirm(`Permanently delete ${file?.path}?\nThis action cannot be undone.`)) return;
    setQuarantine(quarantine.map(f => f.id === id ? { ...f, status: 'deleting' as const } : f));
    if (!mockMode) {
      const res = await deleteFile(id);
      if (!res.ok) {
        setQuarantine(quarantine.map(f => f.id === id ? { ...f, status: 'quarantined' as const } : f));
        addNotification({ type: 'error', title: 'Delete Failed', message: res.error ?? '' });
        return;
      }
    } else {
      await delay(600);
    }
    removeQuarantineFile(id);
    addNotification({ type: 'warning', title: 'File Deleted', message: `Permanently deleted: ${file?.path}` });
  };

  return (
    <div className="h-full flex flex-col gap-4">
      {/* Stats bar */}
      <div className="glass rounded-xl border border-neon-red/20 p-4 flex items-center gap-6 flex-wrap glow-red">
        <div className="flex items-center gap-2">
          <Archive size={16} className="text-neon-red" />
          <span className="text-sm font-semibold text-slate-200">Quarantine Vault</span>
        </div>
        <div className="flex items-center gap-1.5 text-xs">
          <span className="text-slate-500">Files:</span>
          <span className="font-mono text-neon-red font-bold">{quarantine.length}</span>
        </div>
        <div className="flex items-center gap-1.5 text-xs">
          <span className="text-slate-500">Total Size:</span>
          <span className="font-mono text-slate-300">{formatBytes(totalSize)}</span>
        </div>
        {!isAdmin && (
          <span className="text-xs text-neon-yellow ml-auto">Read-only — Admin required to restore/delete</span>
        )}
      </div>

      <Card
        title="Quarantined Files"
        subtitle="Isolated malicious files pending admin review"
        className="flex-1 min-h-0"
        actions={
          <span className="text-[10px] text-slate-500 font-mono">
            {quarantine.length} file{quarantine.length !== 1 ? 's' : ''}
          </span>
        }
      >
        {quarantine.length === 0 ? (
          <div className="flex flex-col items-center gap-3 py-12 text-slate-600">
            <Archive size={32} className="opacity-30" />
            <p className="text-sm">Quarantine vault is empty</p>
            <p className="text-xs text-slate-700">Files will appear here when the agent quarantines suspicious content</p>
          </div>
        ) : (
          <div className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-2">
            {quarantine.map(file => (
              <FileRow
                key={file.id}
                file={file}
                onRestore={handleRestore}
                onDelete={handleDelete}
                isAdmin={isAdmin}
              />
            ))}
          </div>
        )}
      </Card>

      {/* Warning */}
      <div className="shrink-0 glass rounded-lg border border-neon-yellow/20 p-3 text-xs text-slate-400 leading-relaxed">
        <span className="text-neon-yellow font-semibold">Warning: </span>
        Quarantined files may contain active ransomware payloads. Only restore files after forensic analysis confirms
        they are safe. Permanent deletion is irreversible. All actions are logged for audit.
      </div>
    </div>
  );
};

const delay = (ms: number) => new Promise(resolve => setTimeout(resolve, ms));
