import { useCallback } from 'react';
import { useStore } from '../store';
import type { Detection } from '../types';

export function useExport() {
  const detections = useStore(s => s.detections);

  const exportJSON = useCallback(() => {
    const blob = new Blob([JSON.stringify(detections, null, 2)], {
      type: 'application/json',
    });
    download(blob, `ccf-detections-${timestamp()}.json`);
  }, [detections]);

  const exportCSV = useCallback(() => {
    const headers = [
      'id', 'timestamp', 'severity', 'score', 'processName',
      'cfer', 'turbulence', 'shockwave', 'entropy',
      'activeNodes', 'offenderPID', 'action', 'reason',
    ];
    const rows = detections.map((d: Detection) => [
      d.id, d.timestamp, d.severity, d.score, d.processName ?? '',
      d.vector.cfer, d.vector.turbulence, d.vector.shockwave, d.vector.entropy,
      d.vector.activeNodes, d.vector.offenderPID, d.action,
      `"${d.reason.replace(/"/g, '""')}"`,
    ]);
    const csv = [headers.join(','), ...rows.map(r => r.join(','))].join('\n');
    const blob = new Blob([csv], { type: 'text/csv' });
    download(blob, `ccf-detections-${timestamp()}.csv`);
  }, [detections]);

  return { exportJSON, exportCSV };
}

function download(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

function timestamp() {
  return new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
}
