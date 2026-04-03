import React from 'react';
import { Shield, Skull, Archive, PlayCircle, XCircle, AlertTriangle } from 'lucide-react';
import { Card } from '../ui/Card';
import { Toggle } from '../ui/Toggle';
import { Slider } from '../ui/Slider';
import { useStore } from '../../store';
import { postAction, postConfig } from '../../services/api';

export const ResponseControlPanel: React.FC = () => {
  const config = useStore(s => s.config);
  const setConfig = useStore(s => s.setConfig);
  const addNotification = useStore(s => s.addNotification);
  const role = useStore(s => s.role);
  const clearDetections = useStore(s => s.clearDetections);
  const mockMode = useStore(s => s.mockMode);

  const isAdmin = role === 'admin';

  const handleToggle = async (key: keyof typeof config, value: boolean) => {
    setConfig({ [key]: value });
    if (!mockMode) {
      const res = await postConfig({ [key]: value });
      if (!res.ok) addNotification({ type: 'error', title: 'Config Error', message: String(res.error) });
    }
  };

  const handleSlider = async (key: keyof typeof config, value: number) => {
    setConfig({ [key]: value });
    if (!mockMode) {
      const res = await postConfig({ [key]: value });
      if (!res.ok) addNotification({ type: 'error', title: 'Config Error', message: String(res.error) });
    }
  };

  const handleResume = async () => {
    if (!mockMode) {
      const res = await postAction({ action: 'resume' });
      if (res.ok) addNotification({ type: 'success', title: 'Resumed', message: 'SIGCONT dispatched to all paused processes' });
      else addNotification({ type: 'error', title: 'Failed', message: String(res.error) });
    } else {
      addNotification({
        type: 'success',
        title: 'Resume Sent',
        message: 'SIGCONT dispatched to all paused processes',
      });
    }
  };

  const handleClearAlerts = async () => {
    clearDetections();
    if (!mockMode) {
      await postAction({ action: 'clear_alerts' });
    }
    useStore.getState().addNotification({
      type: 'info',
      title: 'Alerts Cleared',
      message: 'Detection history has been cleared',
    });
  };

  return (
    <div className="flex flex-col gap-4">
      {!isAdmin && (
        <div className="glass rounded-lg border border-neon-yellow/30 p-3 flex items-center gap-2 text-xs text-neon-yellow">
          <AlertTriangle size={12} />
          <span>Viewer mode — controls are read-only. Request admin access to modify settings.</span>
        </div>
      )}

      {/* Response Toggles */}
      <Card title="Automated Response Actions" subtitle="Configure graduated threat response">
        <div className="flex flex-col gap-4">
          <div className="flex items-center gap-3 p-3 bg-surface-700 rounded-lg border border-border">
            <Shield size={18} className="text-neon-yellow shrink-0" />
            <div className="flex-1">
              <Toggle
                checked={config.enableSigstop}
                onChange={v => handleToggle('enableSigstop', v)}
                label="Enable SIGSTOP"
                description="Pause suspicious processes at WARNING threshold (reversible)"
                disabled={!isAdmin}
                color="yellow"
              />
            </div>
          </div>

          <div className="flex items-center gap-3 p-3 bg-surface-700 rounded-lg border border-border">
            <Skull size={18} className="text-neon-red shrink-0" />
            <div className="flex-1">
              <Toggle
                checked={config.enableSigkill}
                onChange={v => handleToggle('enableSigkill', v)}
                label="Enable SIGKILL"
                description="Terminate confirmed ransomware processes at ALERT threshold"
                disabled={!isAdmin}
                color="red"
              />
            </div>
          </div>

          <div className="flex items-center gap-3 p-3 bg-surface-700 rounded-lg border border-border">
            <Archive size={18} className="text-neon-purple shrink-0" />
            <div className="flex-1">
              <Toggle
                checked={config.enableQuarantine}
                onChange={v => handleToggle('enableQuarantine', v)}
                label="Enable Quarantine"
                description="Move malicious files to isolated quarantine directory"
                disabled={!isAdmin}
                color="green"
              />
            </div>
          </div>

          <div className="flex items-center gap-3 p-3 bg-surface-700 rounded-lg border border-border">
            <Shield size={18} className="text-slate-500 shrink-0" />
            <div className="flex-1">
              <Toggle
                checked={config.dryRun}
                onChange={v => handleToggle('dryRun', v)}
                label="Dry-Run Mode"
                description="Log actions without actually sending signals or quarantining"
                disabled={!isAdmin}
                color="blue"
              />
            </div>
          </div>
        </div>
      </Card>

      {/* Detection Thresholds */}
      <Card title="Detection Thresholds" subtitle="Score values that trigger response actions">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-5 p-1">
          <Slider
            label="Warning Score Threshold"
            value={config.warningScore}
            onChange={v => handleSlider('warningScore', v)}
            min={0.1} max={0.8} step={0.01}
            disabled={!isAdmin}
            color="yellow"
            formatValue={v => v.toFixed(2)}
          />
          <Slider
            label="Alert Score Threshold"
            value={config.alertScore}
            onChange={v => handleSlider('alertScore', v)}
            min={0.3} max={1.0} step={0.01}
            disabled={!isAdmin}
            color="red"
            formatValue={v => v.toFixed(2)}
          />
          <Slider
            label="Fast Path Threshold"
            value={config.fastThreshold}
            onChange={v => handleSlider('fastThreshold', v)}
            min={0.1} max={0.6} step={0.01}
            disabled={!isAdmin}
            color="blue"
            formatValue={v => v.toFixed(2)}
          />
          <Slider
            label="Confirm Multiplier"
            value={config.confirmMultiplier}
            onChange={v => handleSlider('confirmMultiplier', v)}
            min={1.0} max={3.0} step={0.1}
            disabled={!isAdmin}
            color="green"
            formatValue={v => `×${v.toFixed(1)}`}
          />
        </div>

        {/* Visual threshold indicator */}
        <div className="mt-2 relative h-4 bg-surface-500 rounded-full overflow-visible">
          <div
            className="absolute h-full rounded-full opacity-30"
            style={{
              left: `${config.fastThreshold * 100}%`,
              width: `${(config.warningScore - config.fastThreshold) * 100}%`,
              background: '#ffd700',
            }}
          />
          <div
            className="absolute h-full rounded-full opacity-40"
            style={{
              left: `${config.warningScore * 100}%`,
              width: `${(config.alertScore - config.warningScore) * 100}%`,
              background: '#ff8c00',
            }}
          />
          <div
            className="absolute h-full rounded-full opacity-50"
            style={{
              left: `${config.alertScore * 100}%`,
              right: 0,
              background: '#ff3366',
            }}
          />
          {/* Markers */}
          {[
            { pct: config.fastThreshold * 100, label: 'FAST', color: '#64748b' },
            { pct: config.warningScore * 100, label: 'WARN', color: '#ffd700' },
            { pct: config.alertScore * 100, label: 'ALERT', color: '#ff3366' },
          ].map(({ pct, label, color }) => (
            <div
              key={label}
              className="absolute flex flex-col items-center"
              style={{ left: `${pct}%`, top: '100%', transform: 'translateX(-50%)' }}
            >
              <div className="w-px h-2" style={{ background: color }} />
              <span className="text-[9px] font-mono mt-0.5" style={{ color }}>{label}</span>
            </div>
          ))}
        </div>
        <div className="mt-6 text-[10px] text-slate-600 font-mono">
          Score range: 0.0 (clean) → 1.0 (confirmed ransomware)
        </div>
      </Card>

      {/* Quick Actions */}
      <Card title="Quick Actions">
        <div className="grid grid-cols-2 gap-3">
          <button
            onClick={handleResume}
            disabled={!isAdmin}
            className="flex items-center gap-3 p-3 bg-surface-700 hover:bg-neon-green/10 rounded-lg border border-border hover:border-neon-green/30 transition-all disabled:opacity-40 disabled:cursor-not-allowed group"
          >
            <PlayCircle size={18} className="text-neon-green group-hover:drop-shadow-[0_0_6px_rgba(0,255,136,0.8)]" />
            <div className="text-left">
              <div className="text-xs font-semibold text-slate-200">Resume All Processes</div>
              <div className="text-[10px] text-slate-500">Send SIGCONT to stopped PIDs</div>
            </div>
          </button>

          <button
            onClick={handleClearAlerts}
            disabled={!isAdmin}
            className="flex items-center gap-3 p-3 bg-surface-700 hover:bg-neon-red/10 rounded-lg border border-border hover:border-neon-red/30 transition-all disabled:opacity-40 disabled:cursor-not-allowed group"
          >
            <XCircle size={18} className="text-neon-red group-hover:drop-shadow-[0_0_6px_rgba(255,51,102,0.8)]" />
            <div className="text-left">
              <div className="text-xs font-semibold text-slate-200">Clear All Alerts</div>
              <div className="text-[10px] text-slate-500">Reset detection history</div>
            </div>
          </button>
        </div>
      </Card>

      {/* Score Weight Reference */}
      <Card title="Detector Weight Configuration" subtitle="Feature contribution to composite score">
        <div className="grid grid-cols-2 gap-3">
          {[
            { label: 'CFER Weight',        value: '0.45', color: '#00d4ff', desc: 'Primary signal — regression slope' },
            { label: 'Shockwave Weight',   value: '0.30', color: '#ffd700', desc: 'Onset detection — acceleration' },
            { label: 'Turbulence Weight',  value: '0.15', color: '#b44fff', desc: 'Secondary — variance indicator' },
            { label: 'Entropy Weight',     value: '0.10', color: '#00ff88', desc: 'Supporting — spread of activity' },
          ].map(({ label, value, color, desc }) => (
            <div key={label} className="flex items-center gap-2 p-2 bg-surface-700 rounded-lg border border-border">
              <div className="w-1 h-8 rounded-full" style={{ background: color, opacity: 0.7 }} />
              <div>
                <div className="text-xs font-mono font-semibold" style={{ color }}>{value}</div>
                <div className="text-[10px] text-slate-300">{label}</div>
                <div className="text-[9px] text-slate-500">{desc}</div>
              </div>
            </div>
          ))}
        </div>
        <p className="text-[10px] text-slate-600 mt-1">
          Weights sum to 1.0. Modify via config POST /api/config to adjust detection sensitivity.
        </p>
      </Card>
    </div>
  );
};
