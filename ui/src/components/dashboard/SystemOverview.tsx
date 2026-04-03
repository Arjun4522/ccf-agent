import React, { useMemo } from 'react';
import { Activity, AlertTriangle, AlertOctagon, Zap, Server } from 'lucide-react';
import { Card } from '../ui/Card';
import { Metric } from '../ui/Indicators';
import { useStore } from '../../store';
import {
  AreaChart, Area, ResponsiveContainer, Tooltip,
  ReferenceLine, ReferenceArea, CartesianGrid, YAxis,
} from 'recharts';

const MiniChart: React.FC<{ data: number[]; color: string }> = ({ data, color }) => (
  <ResponsiveContainer width="100%" height="100%">
    <AreaChart data={data.map((v, i) => ({ v, i }))}>
      <defs>
        <linearGradient id={`grad-${color.replace('#', '')}`} x1="0" y1="0" x2="0" y2="1">
          <stop offset="5%"  stopColor={color} stopOpacity={0.35} />
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
  const status    = useStore(s => s.status);
  const detections = useStore(s => s.detections);
  const timeSeries = useStore(s => s.timeSeries);

  const recentScores = useMemo(
    () => timeSeries.slice(-60).map(p => p.score ?? 0),
    [timeSeries]
  );

  const currentScore = recentScores[recentScores.length - 1] ?? 0;
  const peakScore    = useMemo(() => Math.max(...recentScores, 0), [recentScores]);
  const avgScore     = useMemo(() => recentScores.length
    ? recentScores.reduce((a, b) => a + b, 0) / recentScores.length
    : 0, [recentScores]);

  const threatLevel = currentScore >= 0.65 ? 'CRITICAL' : currentScore >= 0.40 ? 'WARNING' : 'NOMINAL';
  const scoreColor  = currentScore >= 0.65 ? 'text-neon-red'    : currentScore >= 0.40 ? 'text-neon-yellow' : 'text-neon-green';
  const scoreGlow   = currentScore >= 0.65 ? 'text-glow-red'    : '';

  const alertCount = detections.filter(d => d.severity === 'ALERT').length;
  const warnCount  = detections.filter(d => d.severity === 'WARNING').length;

  return (
    <div className="h-full flex flex-col gap-4">

      {/* ── Row 1: Metric cards — 1/4 height ─────────────────────────── */}
      <div className="flex-1 min-h-0 grid grid-cols-2 lg:grid-cols-4 gap-3">

        <Card glow={alertCount > 0 ? 'red' : 'none'} compact className="h-full">
          <div className="flex items-start justify-between">
            <Metric label="Total Alerts" value={alertCount} color={alertCount > 0 ? 'red' : 'white'} large />
            <AlertOctagon size={20} className="text-neon-red opacity-60 mt-1" />
          </div>
          <div className="flex-1 min-h-[20px]">
            <MiniChart
              data={Array.from({ length: 30 }, (_, i) => (i > 20 ? Math.random() * 0.9 + 0.1 : Math.random() * 0.1))}
              color="#ff3366"
            />
          </div>
        </Card>

        <Card glow={warnCount > 3 ? 'yellow' : 'none'} compact className="h-full">
          <div className="flex items-start justify-between">
            <Metric label="Warnings" value={warnCount} color={warnCount > 3 ? 'yellow' : 'white'} large />
            <AlertTriangle size={20} className="text-neon-yellow opacity-60 mt-1" />
          </div>
          <div className="flex-1 min-h-[20px]">
            <MiniChart
              data={Array.from({ length: 30 }, () => Math.random() * 0.6)}
              color="#ffd700"
            />
          </div>
        </Card>

        <Card compact className="h-full">
          <div className="flex items-start justify-between">
            <Metric label="Monitored PIDs" value={status.monitoredProcesses} color="blue" large />
            <Server size={20} className="text-neon-blue opacity-60 mt-1" />
          </div>
          <div className="flex-1 min-h-[20px]">
            <MiniChart
              data={Array.from({ length: 30 }, () => Math.random() * 0.3 + 0.5)}
              color="#00d4ff"
            />
          </div>
        </Card>

        <Card compact className="h-full">
          <div className="flex items-start justify-between">
            <Metric label="Field Nodes" value={status.fieldNodes} color="green" large />
            <Activity size={20} className="text-neon-green opacity-60 mt-1" />
          </div>
          <div className="flex-1 min-h-[20px]">
            <MiniChart
              data={Array.from({ length: 30 }, () => Math.random() * 0.4 + 0.3)}
              color="#00ff88"
            />
          </div>
        </Card>

      </div>

      {/* ── Row 2: Health cards — 1/4 height ─────────────────────────── */}
      <div className="flex-1 min-h-0 grid grid-cols-1 lg:grid-cols-3 gap-3">

        <Card title="Events/sec" compact className="h-full">
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

        <Card title="CPU Usage" compact className="h-full">
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

        <Card title="Memory" compact className="h-full">
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

      {/* ── Row 3: Threat Score — 1/2 height ─────────────────────────── */}
      <Card
        title="Threat Score — Last 60s"
        glow={currentScore >= 0.65 ? 'red' : currentScore >= 0.40 ? 'yellow' : 'none'}
        className="flex-[2] min-h-0"
      >
        {/* Chart + live score overlay */}
        <div className="relative flex-1 min-h-[60px]">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart
              data={recentScores.map((s, i) => ({ i, s }))}
              margin={{ top: 8, right: 8, left: -20, bottom: 0 }}
            >
              <defs>
                {/* Zone-aware area fill */}
                <linearGradient id="scoreAreaGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%"   stopColor="#ff3366" stopOpacity={0.55} />
                  <stop offset="35%"  stopColor="#ff3366" stopOpacity={0.30} />
                  <stop offset="60%"  stopColor="#ffd700" stopOpacity={0.15} />
                  <stop offset="100%" stopColor="#00ff88" stopOpacity={0.02} />
                </linearGradient>
                {/* Zone-aware stroke */}
                <linearGradient id="scoreLineGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%"   stopColor="#ff3366" stopOpacity={1} />
                  <stop offset="35%"  stopColor="#ff8c00" stopOpacity={1} />
                  <stop offset="60%"  stopColor="#ffd700" stopOpacity={1} />
                  <stop offset="100%" stopColor="#00ff88" stopOpacity={1} />
                </linearGradient>
              </defs>

              <CartesianGrid strokeDasharray="3 3" stroke="rgba(30,58,90,0.4)" />
              <YAxis
                domain={[0, 1]}
                tick={{ fontSize: 9, fill: '#475569' }}
                tickCount={6}
                width={28}
              />

              {/* Danger zone shading */}
              <ReferenceArea y1={0.65} y2={1}    fill="#ff3366" fillOpacity={0.05} />
              <ReferenceArea y1={0.40} y2={0.65} fill="#ffd700" fillOpacity={0.04} />

              {/* Threshold lines */}
              <ReferenceLine
                y={0.65} stroke="#ff3366" strokeDasharray="5 3" strokeOpacity={0.7}
                label={{ value: 'ALERT  0.65', position: 'insideTopLeft', fontSize: 9, fill: '#ff3366' }}
              />
              <ReferenceLine
                y={0.40} stroke="#ffd700" strokeDasharray="5 3" strokeOpacity={0.6}
                label={{ value: 'WARN  0.40', position: 'insideTopLeft', fontSize: 9, fill: '#ffd700' }}
              />

              <Area
                type="monotone"
                dataKey="s"
                stroke={`url(#scoreLineGrad)`}
                fill="url(#scoreAreaGrad)"
                strokeWidth={2}
                dot={false}
                isAnimationActive={false}
              />

              <Tooltip
                contentStyle={{ background: '#0d1117', border: '1px solid #1e3a5a', borderRadius: 6, fontSize: 11 }}
                formatter={(v: unknown) => [(v as number).toFixed(4), 'Score']}
                labelFormatter={(i: unknown) => `t-${60 - (i as number)}s`}
              />
            </AreaChart>
          </ResponsiveContainer>

          {/* Live score overlay */}
          <div className="absolute top-2 right-8 flex flex-col items-end gap-0.5 pointer-events-none">
            <span className="text-[9px] text-slate-600 font-mono uppercase tracking-widest">live</span>
            <span className={`text-3xl font-mono font-bold tabular-nums leading-none ${scoreColor} ${scoreGlow}`}>
              {currentScore.toFixed(3)}
            </span>
            <span className={`text-[10px] font-mono font-bold uppercase tracking-[0.15em] ${scoreColor}`}>
              {threatLevel}
            </span>
          </div>
        </div>

        {/* Footer stats */}
        <div className="flex items-center gap-6 text-[10px] font-mono shrink-0">
          <span className="text-slate-600">
            peak <span style={{ color: peakScore >= 0.65 ? '#ff3366' : peakScore >= 0.40 ? '#ffd700' : '#00ff88' }}>
              {peakScore.toFixed(3)}
            </span>
          </span>
          <span className="text-slate-600">
            avg <span className="text-slate-400">{avgScore.toFixed(3)}</span>
          </span>
          <span className="text-slate-700 ml-auto">
            stroke = score zone · fill = danger gradient
          </span>
        </div>
      </Card>

    </div>
  );
};
