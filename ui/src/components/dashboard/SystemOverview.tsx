import React, { useMemo } from 'react';
import { Activity, AlertTriangle, AlertOctagon, Zap, Server, Radio } from 'lucide-react';
import { Card } from '../ui/Card';
import { Metric, StatusDot } from '../ui/Indicators';
import { Badge } from '../ui/Badge';
import { useStore } from '../../store';
import { formatUptime } from '../../utils/mockData';
import {
  AreaChart, Area, ResponsiveContainer, Tooltip,
} from 'recharts';

const MiniChart: React.FC<{ data: number[]; color: string }> = ({ data, color }) => (
  <ResponsiveContainer width="100%" height={36}>
    <AreaChart data={data.map((v, i) => ({ v, i }))}>
      <defs>
        <linearGradient id={`grad-${color.replace('#', '')}`} x1="0" y1="0" x2="0" y2="1">
          <stop offset="5%" stopColor={color} stopOpacity={0.3} />
          <stop offset="95%" stopColor={color} stopOpacity={0} />
        </linearGradient>
      </defs>
      <Area
        type="monotone"
        dataKey="v"
        stroke={color}
        fill={`url(#grad-${color.replace('#', '')})`}
        strokeWidth={1.5}
        dot={false}
        isAnimationActive={false}
      />
    </AreaChart>
  </ResponsiveContainer>
);

export const SystemOverview: React.FC = () => {
  const status = useStore(s => s.status);
  const wsConnected = useStore(s => s.wsConnected);
  const detections = useStore(s => s.detections);
  const timeSeries = useStore(s => s.timeSeries);

  const recentScores = useMemo(
    () => timeSeries.slice(-60).map(p => p.score ?? 0),
    [timeSeries]
  );

  const statusColor =
    status.status === 'ALERT'   ? 'red' :
    status.status === 'RUNNING' ? 'green' :
    status.status === 'IDLE'    ? 'yellow' :
    'gray';

  const agentStatusVariant =
    status.status === 'ALERT'   ? 'red' :
    status.status === 'RUNNING' ? 'green' :
    status.status === 'IDLE'    ? 'yellow' :
    'gray' as const;

  const alertCount  = detections.filter(d => d.severity === 'ALERT').length;
  const warnCount   = detections.filter(d => d.severity === 'WARNING').length;
  const recentAlert = detections.find(d => d.severity === 'ALERT');

  return (
    <div className="flex flex-col gap-4">
      {/* Header status bar */}
      <div className="glass rounded-xl border border-border p-4 flex items-center gap-4 flex-wrap">
        <div className="flex items-center gap-2">
          <StatusDot status={statusColor} size={10} />
          <span className="text-sm font-semibold text-slate-200">CCF Agent</span>
          <Badge
            label={status.status}
            variant={agentStatusVariant}
            dot
            pulse={status.status !== 'IDLE'}
          />
        </div>
        <div className="flex items-center gap-1.5 text-xs text-slate-500">
          <Radio size={10} className={wsConnected ? 'text-neon-green' : 'text-slate-600'} />
          <span>{wsConnected ? 'WebSocket Connected' : 'Disconnected'}</span>
        </div>
        <div className="flex items-center gap-1.5 text-xs text-slate-500 ml-auto">
          <span>Uptime:</span>
          <span className="font-mono text-slate-300">{formatUptime(status.uptime)}</span>
        </div>
        {recentAlert && (
          <div className="flex items-center gap-1.5 text-xs">
            <AlertOctagon size={11} className="text-neon-red animate-pulse-red" />
            <span className="text-neon-red font-mono">
              Last alert: PID {recentAlert.vector.offenderPID} — {recentAlert.processName}
            </span>
          </div>
        )}
      </div>

      {/* Metric cards */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
        <Card glow={alertCount > 0 ? 'red' : 'none'} compact>
          <div className="flex items-start justify-between">
            <div>
              <Metric
                label="Total Alerts"
                value={alertCount}
                color={alertCount > 0 ? 'red' : 'white'}
                large
              />
            </div>
            <AlertOctagon size={20} className="text-neon-red opacity-60 mt-1" />
          </div>
          <MiniChart
            data={Array.from({ length: 30 }, (_, i) => (i > 20 ? Math.random() * 0.9 + 0.1 : Math.random() * 0.1))}
            color="#ff3366"
          />
        </Card>

        <Card glow={warnCount > 3 ? 'yellow' : 'none'} compact>
          <div className="flex items-start justify-between">
            <Metric
              label="Warnings"
              value={warnCount}
              color={warnCount > 3 ? 'yellow' : 'white'}
              large
            />
            <AlertTriangle size={20} className="text-neon-yellow opacity-60 mt-1" />
          </div>
          <MiniChart
            data={Array.from({ length: 30 }, () => Math.random() * 0.6)}
            color="#ffd700"
          />
        </Card>

        <Card compact>
          <div className="flex items-start justify-between">
            <Metric
              label="Monitored PIDs"
              value={status.monitoredProcesses}
              color="blue"
              large
            />
            <Server size={20} className="text-neon-blue opacity-60 mt-1" />
          </div>
          <MiniChart
            data={Array.from({ length: 30 }, () => Math.random() * 0.3 + 0.5)}
            color="#00d4ff"
          />
        </Card>

        <Card compact>
          <div className="flex items-start justify-between">
            <Metric
              label="Field Nodes"
              value={status.fieldNodes}
              color="green"
              large
            />
            <Activity size={20} className="text-neon-green opacity-60 mt-1" />
          </div>
          <MiniChart
            data={Array.from({ length: 30 }, () => Math.random() * 0.4 + 0.3)}
            color="#00ff88"
          />
        </Card>
      </div>

      {/* System health row */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-3">
        <Card title="Events/sec" compact>
          <div className="flex items-end justify-between">
            <div className="flex flex-col gap-0.5">
              <span className="text-3xl font-mono font-bold text-neon-blue text-glow-blue">
                {status.eventsPerSecond.toLocaleString()}
              </span>
              <span className="text-xs text-slate-500">kernel events processed</span>
            </div>
            <Zap size={24} className="text-neon-blue opacity-50 mb-1" />
          </div>
        </Card>

        <Card title="CPU Usage" compact>
          <div className="flex flex-col gap-2">
            <div className="flex items-end gap-2">
              <span className="text-3xl font-mono font-bold text-neon-green text-glow-green">
                {status.cpuUsage.toFixed(1)}%
              </span>
              <span className="text-xs text-slate-500 mb-1">agent overhead</span>
            </div>
            <div className="w-full h-2 bg-surface-500 rounded-full overflow-hidden">
              <div
                className="h-full rounded-full transition-all duration-500"
                style={{
                  width: `${Math.min(status.cpuUsage, 100)}%`,
                  background: status.cpuUsage > 30 ? '#ff3366' : status.cpuUsage > 15 ? '#ffd700' : '#00ff88',
                }}
              />
            </div>
          </div>
        </Card>

        <Card title="Memory" compact>
          <div className="flex flex-col gap-2">
            <div className="flex items-end gap-2">
              <span className="text-3xl font-mono font-bold text-neon-purple">
                {status.memoryMb.toFixed(0)}
              </span>
              <span className="text-xs text-slate-500 mb-1">MB RSS</span>
            </div>
            <div className="w-full h-2 bg-surface-500 rounded-full overflow-hidden">
              <div
                className="h-full rounded-full transition-all duration-500 bg-neon-purple"
                style={{ width: `${Math.min((status.memoryMb / 512) * 100, 100)}%`, opacity: 0.7 }}
              />
            </div>
          </div>
        </Card>
      </div>

      {/* Score Sparkline */}
      <Card title="Threat Score — Last 60s" glow={recentScores.some(s => s >= 0.65) ? 'red' : 'none'}>
        <ResponsiveContainer width="100%" height={100}>
          <AreaChart data={recentScores.map((s, i) => ({ i, s }))}>
            <defs>
              <linearGradient id="scoreGrad" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="#ff3366" stopOpacity={0.35} />
                <stop offset="95%" stopColor="#ff3366" stopOpacity={0} />
              </linearGradient>
            </defs>
            <Area
              type="monotone"
              dataKey="s"
              stroke="#ff3366"
              fill="url(#scoreGrad)"
              strokeWidth={1.5}
              dot={false}
              isAnimationActive={false}
            />
            <Tooltip
              contentStyle={{ background: '#0d1117', border: '1px solid #1e3a5a', borderRadius: 6, fontSize: 11 }}
              formatter={(v: unknown) => [(v as number).toFixed(3), 'Score']}
              labelFormatter={() => ''}
            />
          </AreaChart>
        </ResponsiveContainer>
        <div className="flex gap-4 text-[10px] text-slate-500 font-mono">
          <span>WARN threshold: <span className="text-neon-yellow">0.40</span></span>
          <span>ALERT threshold: <span className="text-neon-red">0.65</span></span>
        </div>
      </Card>
    </div>
  );
};
