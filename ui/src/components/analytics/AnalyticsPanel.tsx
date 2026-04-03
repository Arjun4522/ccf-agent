import React, { useMemo } from 'react';
import {
  ResponsiveContainer,
  LineChart, Line,
  BarChart, Bar,
  AreaChart, Area,
  XAxis, YAxis,
  CartesianGrid, Tooltip, Legend,
  ReferenceLine,
} from 'recharts';
import { Card } from '../ui/Card';
import { useStore } from '../../store';
import type { Severity } from '../../types';

const TOOLTIP_STYLE = {
  contentStyle: {
    background: '#0d1117',
    border: '1px solid #1e3a5a',
    borderRadius: 8,
    fontSize: 11,
    fontFamily: 'monospace',
  },
  labelStyle: { color: '#64748b' },
};

// ─── Feature Trend Chart ──────────────────────────────────────────────────────

export const FeatureTrendChart: React.FC = () => {
  const timeSeries = useStore(s => s.timeSeries);
  const data = useMemo(() => timeSeries.slice(-120), [timeSeries]);

  return (
    <Card title="Feature Trends — CFER · Turbulence · Shockwave · Entropy">
      <ResponsiveContainer width="100%" height={220}>
        <LineChart data={data} margin={{ top: 4, right: 8, left: -20, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" />
          <XAxis dataKey="time" tick={{ fontSize: 9, fill: '#64748b' }} interval="preserveStartEnd" />
          <YAxis tick={{ fontSize: 9, fill: '#64748b' }} />
          <Tooltip {...TOOLTIP_STYLE} />
          <Legend wrapperStyle={{ fontSize: 10, paddingTop: 8 }} />
          <Line
            type="monotone" dataKey="cfer" name="CFER"
            stroke="#00d4ff" strokeWidth={1.5} dot={false} isAnimationActive={false}
          />
          <Line
            type="monotone" dataKey="turbulence" name="Turbulence"
            stroke="#b44fff" strokeWidth={1.5} dot={false} isAnimationActive={false}
          />
          <Line
            type="monotone" dataKey="shockwave" name="Shockwave"
            stroke="#ffd700" strokeWidth={1.5} dot={false} isAnimationActive={false}
          />
          <Line
            type="monotone" dataKey="entropy" name="Entropy"
            stroke="#00ff88" strokeWidth={1.5} dot={false} isAnimationActive={false}
          />
        </LineChart>
      </ResponsiveContainer>
    </Card>
  );
};

// ─── Threat Score Over Time ───────────────────────────────────────────────────

export const ThreatScoreChart: React.FC = () => {
  const timeSeries = useStore(s => s.timeSeries);
  const data = useMemo(() => timeSeries.slice(-180), [timeSeries]);

  return (
    <Card title="Threat Score — Live Timeline" glow={
      data.some(d => (d.score ?? 0) >= 0.65) ? 'red' : 'none'
    }>
      <ResponsiveContainer width="100%" height={200}>
        <AreaChart data={data} margin={{ top: 4, right: 8, left: -20, bottom: 0 }}>
          <defs>
            <linearGradient id="scoreGrad2" x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor="#ff3366" stopOpacity={0.4} />
              <stop offset="95%" stopColor="#ff3366" stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid strokeDasharray="3 3" />
          <XAxis dataKey="time" tick={{ fontSize: 9, fill: '#64748b' }} interval="preserveStartEnd" />
          <YAxis domain={[0, 1]} tick={{ fontSize: 9, fill: '#64748b' }} tickFormatter={v => v.toFixed(1)} />
          <Tooltip
            {...TOOLTIP_STYLE}
            formatter={(v: unknown) => [(v as number).toFixed(3), 'Score']}
          />
          <ReferenceLine y={0.40} stroke="#ffd700" strokeDasharray="4 4" strokeOpacity={0.6}
            label={{ value: 'WARN', position: 'insideTopRight', fontSize: 9, fill: '#ffd700' }} />
          <ReferenceLine y={0.65} stroke="#ff3366" strokeDasharray="4 4" strokeOpacity={0.7}
            label={{ value: 'ALERT', position: 'insideTopRight', fontSize: 9, fill: '#ff3366' }} />
          <Area
            type="monotone" dataKey="score" name="Score"
            stroke="#ff3366" fill="url(#scoreGrad2)"
            strokeWidth={2} dot={false} isAnimationActive={false}
          />
        </AreaChart>
      </ResponsiveContainer>
    </Card>
  );
};

// ─── Severity Distribution Bar Chart ─────────────────────────────────────────

export const SeverityDistributionChart: React.FC = () => {
  const detections = useStore(s => s.detections);

  const data = useMemo(() => {
    const buckets: Record<string, Record<Severity, number>> = {};
    detections.forEach(d => {
      const bucket = new Date(d.timestamp).toLocaleTimeString('en-GB', {
        hour: '2-digit', minute: '2-digit', hour12: false,
      });
      if (!buckets[bucket]) buckets[bucket] = { NONE: 0, WARNING: 0, ALERT: 0 };
      buckets[bucket][d.severity]++;
    });
    return Object.entries(buckets)
      .sort(([a], [b]) => a.localeCompare(b))
      .slice(-30)
      .map(([time, counts]) => ({ time, ...counts }));
  }, [detections]);

  return (
    <Card title="Severity Distribution — 30-Minute Buckets">
      <ResponsiveContainer width="100%" height={200}>
        <BarChart data={data} margin={{ top: 4, right: 8, left: -20, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" />
          <XAxis dataKey="time" tick={{ fontSize: 9, fill: '#64748b' }} interval="preserveStartEnd" />
          <YAxis tick={{ fontSize: 9, fill: '#64748b' }} allowDecimals={false} />
          <Tooltip {...TOOLTIP_STYLE} />
          <Legend wrapperStyle={{ fontSize: 10, paddingTop: 8 }} />
          <Bar dataKey="WARNING" name="WARNING" fill="#ffd700" fillOpacity={0.7} radius={[2, 2, 0, 0]} />
          <Bar dataKey="ALERT"   name="ALERT"   fill="#ff3366" fillOpacity={0.8} radius={[2, 2, 0, 0]} />
        </BarChart>
      </ResponsiveContainer>
    </Card>
  );
};

// ─── CFER Detail Chart ────────────────────────────────────────────────────────

export const CFERDetailChart: React.FC = () => {
  const timeSeries = useStore(s => s.timeSeries);
  const data = useMemo(() => timeSeries.slice(-120), [timeSeries]);

  return (
    <Card title="CFER (Capability Field Expansion Rate)" subtitle="Linear regression slope of ||F|| over time window">
      <ResponsiveContainer width="100%" height={160}>
        <AreaChart data={data} margin={{ top: 4, right: 8, left: -20, bottom: 0 }}>
          <defs>
            <linearGradient id="cferGrad" x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor="#00d4ff" stopOpacity={0.35} />
              <stop offset="95%" stopColor="#00d4ff" stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid strokeDasharray="3 3" />
          <XAxis dataKey="time" tick={{ fontSize: 9, fill: '#64748b' }} interval="preserveStartEnd" />
          <YAxis tick={{ fontSize: 9, fill: '#64748b' }} />
          <Tooltip {...TOOLTIP_STYLE}           formatter={(v: unknown) => [(v as number).toFixed(4), 'CFER']} />
          <ReferenceLine y={0.3} stroke="#ffd700" strokeDasharray="4 4" strokeOpacity={0.5}
            label={{ value: 'thresh', position: 'insideTopRight', fontSize: 8, fill: '#ffd700' }} />
          <Area
            type="monotone" dataKey="cfer" stroke="#00d4ff"
            fill="url(#cferGrad)" strokeWidth={1.5} dot={false} isAnimationActive={false}
          />
        </AreaChart>
      </ResponsiveContainer>
    </Card>
  );
};

// ─── Entropy + Turbulence ─────────────────────────────────────────────────────

export const EntropyTurbulenceChart: React.FC = () => {
  const timeSeries = useStore(s => s.timeSeries);
  const data = useMemo(() => timeSeries.slice(-120), [timeSeries]);

  return (
    <Card title="Entropy & Turbulence" subtitle="Spread indicator vs activity variance">
      <ResponsiveContainer width="100%" height={160}>
        <LineChart data={data} margin={{ top: 4, right: 8, left: -20, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" />
          <XAxis dataKey="time" tick={{ fontSize: 9, fill: '#64748b' }} interval="preserveStartEnd" />
          <YAxis tick={{ fontSize: 9, fill: '#64748b' }} />
          <Tooltip {...TOOLTIP_STYLE} />
          <Legend wrapperStyle={{ fontSize: 10, paddingTop: 8 }} />
          <Line
            type="monotone" dataKey="entropy" name="Entropy (bits)"
            stroke="#00ff88" strokeWidth={1.5} dot={false} isAnimationActive={false}
          />
          <Line
            type="monotone" dataKey="turbulence" name="Turbulence (variance)"
            stroke="#b44fff" strokeWidth={1.5} dot={false} isAnimationActive={false}
          />
        </LineChart>
      </ResponsiveContainer>
    </Card>
  );
};

// ─── Analytics Page Composition ───────────────────────────────────────────────

export const AnalyticsPanel: React.FC = () => (
  <div className="flex flex-col gap-4">
    <ThreatScoreChart />
    <FeatureTrendChart />
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
      <CFERDetailChart />
      <EntropyTurbulenceChart />
    </div>
    <SeverityDistributionChart />
  </div>
);
