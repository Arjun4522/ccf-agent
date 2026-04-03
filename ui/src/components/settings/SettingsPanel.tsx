import React, { useEffect } from 'react';
import { Save, RotateCcw } from 'lucide-react';
import { Card } from '../ui/Card';
import { Toggle } from '../ui/Toggle';
import { Slider } from '../ui/Slider';
import { Button } from '../ui/Button';
import { useStore } from '../../store';
import { postConfig, getConfig } from '../../services/api';

export const SettingsPanel: React.FC = () => {
  const config = useStore(s => s.config);
  const setConfig = useStore(s => s.setConfig);
  const role = useStore(s => s.role);
  const setRole = useStore(s => s.setRole);
  const mockMode = useStore(s => s.mockMode);
  const setMockMode = useStore(s => s.setMockMode);
  const addNotification = useStore(s => s.addNotification);
  const isAdmin = role === 'admin';

  // Load config from backend on mount (real mode only).
  useEffect(() => {
    if (mockMode) return;
    getConfig().then(res => {
      if (res.ok) setConfig(res.data);
    });
  }, [mockMode]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleSave = async () => {
    if (!mockMode) {
      const res = await postConfig(config);
      if (!res.ok) {
        addNotification({ type: 'error', title: 'Config Save Failed', message: res.error ?? '' });
        return;
      }
    }
    addNotification({ type: 'success', title: 'Configuration Saved', message: 'Settings applied to agent' });
  };

  const handleReset = () => {
    setConfig({
      decayRate: 0.85,
      windowSize: 30,
      snapshotIntervalMs: 500,
      jsonLogging: true,
      debugMode: false,
      dryRun: false,
    });
    addNotification({ type: 'info', title: 'Settings Reset', message: 'Field parameters restored to defaults' });
  };

  return (
    <div className="flex flex-col gap-4">
      {/* Field Parameters */}
      <Card
        title="Field Parameters"
        subtitle="Core CCF algorithm configuration"
        actions={
          <div className="flex gap-2">
            <Button variant="ghost" size="xs" icon={RotateCcw} onClick={handleReset} disabled={!isAdmin}>
              Reset
            </Button>
            <Button variant="primary" size="xs" icon={Save} onClick={handleSave} disabled={!isAdmin}>
              Save
            </Button>
          </div>
        }
      >
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-5 p-1">
          <Slider
            label="Decay Rate"
            value={config.decayRate}
            onChange={v => setConfig({ decayRate: v })}
            min={0.5} max={0.99} step={0.01}
            disabled={!isAdmin}
            color="blue"
            formatValue={v => v.toFixed(2)}
          />
          <Slider
            label="Window Size (snapshots)"
            value={config.windowSize}
            onChange={v => setConfig({ windowSize: v })}
            min={5} max={100} step={1}
            disabled={!isAdmin}
            color="green"
            formatValue={v => `${v} snapshots`}
          />
          <Slider
            label="Snapshot Interval"
            value={config.snapshotIntervalMs}
            onChange={v => setConfig({ snapshotIntervalMs: v })}
            min={100} max={2000} step={50}
            disabled={!isAdmin}
            color="yellow"
            formatValue={v => `${v} ms`}
          />
        </div>

        <div className="bg-surface-700 rounded-lg border border-border p-3 mt-1">
          <div className="text-[10px] text-slate-500 font-mono leading-relaxed">
            <div>Window history: <span className="text-slate-300">{((config.windowSize * config.snapshotIntervalMs) / 1000).toFixed(1)}s</span></div>
            <div>Decay per second: <span className="text-slate-300">{(Math.pow(config.decayRate, 1000 / config.snapshotIntervalMs) * 100).toFixed(1)}%</span></div>
            <div>Effective resolution: <span className="text-slate-300">{config.snapshotIntervalMs}ms per snapshot</span></div>
          </div>
        </div>
      </Card>

      {/* Feature Thresholds */}
      <Card title="Feature Normalization Thresholds">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-5 p-1">
          <Slider
            label="CFER Threshold"
            value={config.cferThreshold}
            onChange={v => setConfig({ cferThreshold: v })}
            min={0.05} max={2.0} step={0.05}
            disabled={!isAdmin}
            color="blue"
            formatValue={v => v.toFixed(2)}
          />
          <Slider
            label="Turbulence Threshold"
            value={config.turbulenceThreshold}
            onChange={v => setConfig({ turbulenceThreshold: v })}
            min={1.0} max={20.0} step={0.5}
            disabled={!isAdmin}
            color="purple"
            formatValue={v => v.toFixed(1)}
          />
          <Slider
            label="Shockwave Threshold"
            value={config.shockwaveThreshold}
            onChange={v => setConfig({ shockwaveThreshold: v })}
            min={0.5} max={10.0} step={0.25}
            disabled={!isAdmin}
            color="yellow"
            formatValue={v => v.toFixed(2)}
          />
          <Slider
            label="Entropy Threshold"
            value={config.entropyThreshold}
            onChange={v => setConfig({ entropyThreshold: v })}
            min={0.5} max={8.0} step={0.25}
            disabled={!isAdmin}
            color="green"
            formatValue={v => `${v.toFixed(2)} bits`}
          />
        </div>
      </Card>

      {/* Logging & Debug */}
      <Card title="Logging & Diagnostics">
        <div className="flex flex-col gap-4">
          <Toggle
            checked={config.jsonLogging}
            onChange={v => setConfig({ jsonLogging: v })}
            label="JSON Structured Logging"
            description="Output logs as JSON for SIEM/ELK integration"
            disabled={!isAdmin}
            color="blue"
          />
          <Toggle
            checked={config.debugMode}
            onChange={v => setConfig({ debugMode: v })}
            label="Debug Mode"
            description="Verbose field state logging (high CPU overhead)"
            disabled={!isAdmin}
            color="yellow"
          />
          <Toggle
            checked={config.dryRun}
            onChange={v => setConfig({ dryRun: v })}
            label="Dry-Run Mode"
            description="Simulate responses without sending signals"
            disabled={!isAdmin}
            color="green"
          />
        </div>
      </Card>

      {/* UI Settings */}
      <Card title="UI Settings">
        <div className="flex flex-col gap-4">
          <Toggle
            checked={mockMode}
            onChange={setMockMode}
            label="Mock/Demo Mode"
            description="Use simulated data instead of live backend"
            color="purple"
          />
          <div className="flex items-center justify-between gap-2">
            <div>
              <span className="text-sm text-slate-200 font-medium">User Role</span>
              <p className="text-xs text-slate-500">Switch between admin and viewer for testing</p>
            </div>
            <div className="flex rounded-lg overflow-hidden border border-border">
              {(['admin', 'viewer'] as const).map(r => (
                <button
                  key={r}
                  onClick={() => setRole(r)}
                  className={`px-3 py-1.5 text-xs font-mono transition-all ${
                    role === r
                      ? 'bg-neon-blue/20 text-neon-blue'
                      : 'text-slate-500 hover:text-slate-300 hover:bg-surface-600'
                  }`}
                >
                  {r}
                </button>
              ))}
            </div>
          </div>
        </div>
      </Card>

      {/* About */}
      <Card title="About CCF Agent">
        <div className="grid grid-cols-2 gap-3 text-xs font-mono">
          {[
            ['Agent Version', 'v2.1.0'],
            ['Detection Engine', 'Threshold + Multi-Scale'],
            ['eBPF Backend', 'Cilium/ebpf v0.16'],
            ['Tracepoints', 'openat, write, rename, unlink, exec, setuid'],
            ['Algorithm', 'Cognitive Capability Field (CCF)'],
            ['Response', 'SIGSTOP → SIGKILL → Quarantine'],
          ].map(([label, value]) => (
            <div key={label} className="flex flex-col bg-surface-700 rounded p-2 border border-border">
              <span className="text-[9px] uppercase tracking-widest text-slate-600">{label}</span>
              <span className="text-slate-300 mt-0.5">{value}</span>
            </div>
          ))}
        </div>
      </Card>
    </div>
  );
};
