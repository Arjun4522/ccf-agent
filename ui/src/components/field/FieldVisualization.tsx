import React, { useRef, useEffect, useMemo, useCallback, useState } from 'react';
import { Card } from '../ui/Card';
import { useStore } from '../../store';
import type { FieldNode, FieldEdge } from '../../types';
import { Metric } from '../ui/Indicators';
import { Badge } from '../ui/Badge';

// ─── Canvas-based force-directed field visualization ─────────────────────────

interface SimNode extends FieldNode {
  x: number;
  y: number;
  vx: number;
  vy: number;
}

const TYPE_COLORS: Record<FieldNode['type'], string> = {
  process:   '#ff3366',
  file:      '#00d4ff',
  directory: '#00ff88',
};

const TYPE_LABELS: Record<FieldNode['type'], string> = {
  process:   'Process',
  file:      'File',
  directory: 'Directory',
};

function useForceSimulation(nodes: FieldNode[], edges: FieldEdge[]) {
  const simNodes = useRef<SimNode[]>([]);

  const init = useCallback((width: number, height: number) => {
    simNodes.current = nodes.map((n) => {
      const existing = simNodes.current.find(s => s.id === n.id);
      return {
        ...n,
        x: existing?.x ?? (Math.random() * (width - 60) + 30),
        y: existing?.y ?? (Math.random() * (height - 60) + 30),
        vx: existing?.vx ?? 0,
        vy: existing?.vy ?? 0,
      };
    });
  }, [nodes]);

  const tick = useCallback((width: number, height: number) => {
    const sn = simNodes.current;
    const alpha = 0.08;

    // Repulsion
    for (let i = 0; i < sn.length; i++) {
      for (let j = i + 1; j < sn.length; j++) {
        const dx = sn[j].x - sn[i].x;
        const dy = sn[j].y - sn[i].y;
        const dist = Math.sqrt(dx * dx + dy * dy) || 1;
        const force = (2000 / (dist * dist)) * alpha;
        sn[i].vx -= (dx / dist) * force;
        sn[i].vy -= (dy / dist) * force;
        sn[j].vx += (dx / dist) * force;
        sn[j].vy += (dy / dist) * force;
      }
    }

    // Attraction along edges
    edges.forEach(edge => {
      const src = sn.find(n => n.id === edge.source);
      const tgt = sn.find(n => n.id === edge.target);
      if (!src || !tgt) return;
      const dx = tgt.x - src.x;
      const dy = tgt.y - src.y;
      const dist = Math.sqrt(dx * dx + dy * dy) || 1;
      const target = 80 + edge.weight * 40;
      const force = ((dist - target) / dist) * 0.05 * alpha;
      src.vx += dx * force;
      src.vy += dy * force;
      tgt.vx -= dx * force;
      tgt.vy -= dy * force;
    });

    // Center gravity
    sn.forEach(n => {
      n.vx += ((width / 2) - n.x) * 0.003 * alpha;
      n.vy += ((height / 2) - n.y) * 0.003 * alpha;
    });

    // Apply velocity with damping
    sn.forEach(n => {
      n.vx *= 0.85;
      n.vy *= 0.85;
      n.x = Math.max(20, Math.min(width - 20, n.x + n.vx));
      n.y = Math.max(20, Math.min(height - 20, n.y + n.vy));
    });
  }, [edges]);

  return { simNodes, init, tick };
}

function drawField(
  ctx: CanvasRenderingContext2D,
  nodes: SimNode[],
  edges: FieldEdge[],
  width: number,
  height: number,
  hoveredId: string | null
) {
  ctx.clearRect(0, 0, width, height);

  // Background grid
  ctx.strokeStyle = 'rgba(30,58,90,0.2)';
  ctx.lineWidth = 0.5;
  const gridSize = 40;
  for (let x = 0; x < width; x += gridSize) {
    ctx.beginPath(); ctx.moveTo(x, 0); ctx.lineTo(x, height); ctx.stroke();
  }
  for (let y = 0; y < height; y += gridSize) {
    ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(width, y); ctx.stroke();
  }

  // Draw edges
  edges.forEach(edge => {
    const src = nodes.find(n => n.id === edge.source);
    const tgt = nodes.find(n => n.id === edge.target);
    if (!src || !tgt) return;
    const alpha = edge.weight * 0.4;
    ctx.beginPath();
    ctx.moveTo(src.x, src.y);
    ctx.lineTo(tgt.x, tgt.y);
    ctx.strokeStyle = `rgba(30,90,130,${alpha})`;
    ctx.lineWidth = edge.weight * 1.5;
    ctx.stroke();
  });

  // Draw nodes
  nodes.forEach(node => {
    const radius = Math.max(6, Math.min(24, node.intensity * 22));
    const color = TYPE_COLORS[node.type];
    const isHovered = node.id === hoveredId;

    // Glow
    const grad = ctx.createRadialGradient(node.x, node.y, 0, node.x, node.y, radius * 2.5);
    grad.addColorStop(0, color.replace(')', `,${node.intensity * 0.4})`).replace('#', 'rgba(').replace(/^rgba\(/, 'rgba(').replace(/([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})/, (_, r, g, b) => `${parseInt(r, 16)},${parseInt(g, 16)},${parseInt(b, 16)}`));
    // Fallback glow
    ctx.beginPath();
    ctx.arc(node.x, node.y, radius * 2, 0, Math.PI * 2);
    const glowColor = color + '22';
    ctx.fillStyle = glowColor;
    ctx.fill();

    // Node circle
    ctx.beginPath();
    ctx.arc(node.x, node.y, radius + (isHovered ? 3 : 0), 0, Math.PI * 2);
    ctx.fillStyle = `${color}33`;
    ctx.fill();
    ctx.strokeStyle = color;
    ctx.lineWidth = isHovered ? 2.5 : 1.5;
    ctx.stroke();

    // Inner dot
    ctx.beginPath();
    ctx.arc(node.x, node.y, Math.min(radius * 0.4, 5), 0, Math.PI * 2);
    ctx.fillStyle = color;
    ctx.globalAlpha = 0.9;
    ctx.fill();
    ctx.globalAlpha = 1;

    // Label
    if (isHovered || radius > 10) {
      const label = node.label.split(':').pop()?.slice(0, 16) ?? node.label;
      ctx.font = '9px monospace';
      ctx.fillStyle = 'rgba(148,163,184,0.9)';
      ctx.textAlign = 'center';
      ctx.fillText(label, node.x, node.y + radius + 11);
    }

    // Intensity text
    if (isHovered) {
      ctx.font = 'bold 9px monospace';
      ctx.fillStyle = color;
      ctx.textAlign = 'center';
      ctx.fillText(node.intensity.toFixed(2), node.x, node.y + 3);
    }
  });
}

export const FieldVisualization: React.FC = () => {
  const fieldSnapshot = useStore(s => s.fieldSnapshot);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const animRef = useRef<number>(0);
  const { simNodes, init, tick } = useForceSimulation(
    fieldSnapshot?.nodes ?? [],
    fieldSnapshot?.edges ?? []
  );
  const [hoveredId, setHoveredId] = useState<string | null>(null);
  const [dims, setDims] = useState({ width: 700, height: 400 });

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const observer = new ResizeObserver(entries => {
      const { width, height } = entries[0].contentRect;
      setDims({ width: Math.floor(width), height: Math.floor(height) });
    });
    observer.observe(canvas.parentElement!);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    if (!fieldSnapshot) return;
    init(dims.width, dims.height);
  }, [fieldSnapshot, dims, init]);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const loop = () => {
      tick(dims.width, dims.height);
      drawField(ctx, simNodes.current, fieldSnapshot?.edges ?? [], dims.width, dims.height, hoveredId);
      animRef.current = requestAnimationFrame(loop);
    };
    animRef.current = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(animRef.current);
  }, [dims, fieldSnapshot, hoveredId, tick, simNodes]);

  const handleMouseMove = useCallback((e: React.MouseEvent<HTMLCanvasElement>) => {
    const rect = canvasRef.current?.getBoundingClientRect();
    if (!rect) return;
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;
    const hit = simNodes.current.find(n => {
      const dx = n.x - mx; const dy = n.y - my;
      const r = Math.max(6, Math.min(24, n.intensity * 22));
      return Math.sqrt(dx * dx + dy * dy) <= r + 4;
    });
    setHoveredId(hit?.id ?? null);
  }, [simNodes]);

  const typeCounts = useMemo(() => {
    const nodes = fieldSnapshot?.nodes ?? [];
    return {
      process:   nodes.filter(n => n.type === 'process').length,
      file:      nodes.filter(n => n.type === 'file').length,
      directory: nodes.filter(n => n.type === 'directory').length,
    };
  }, [fieldSnapshot]);

  const topNode = useMemo(() => {
    if (!fieldSnapshot?.nodes.length) return null;
    return fieldSnapshot.nodes.reduce((a, b) => a.intensity > b.intensity ? a : b);
  }, [fieldSnapshot]);

  return (
    <div className="h-full flex flex-col gap-4">
      {/* Stats */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
        <Card compact>
          <Metric label="Field Norm" value={fieldSnapshot?.norm.toFixed(3) ?? '—'} color="blue" />
        </Card>
        <Card compact>
          <Metric label="Total Nodes" value={fieldSnapshot?.nodes.length ?? 0} color="green" />
        </Card>
        <Card compact>
          <Metric label="Edges" value={fieldSnapshot?.edges.length ?? 0} color="purple" />
        </Card>
        <Card compact>
          <div className="flex flex-col gap-1">
            <span className="text-[10px] uppercase tracking-widest text-slate-500">Top Offender</span>
            {topNode ? (
              <>
                <span className="text-xs font-mono text-neon-red truncate">{topNode.label.split(':').pop()}</span>
                <span className="text-[10px] text-slate-500 font-mono">{topNode.intensity.toFixed(3)}</span>
              </>
            ) : <span className="text-slate-600 text-xs">none</span>}
          </div>
        </Card>
      </div>

      {/* Main canvas */}
      <Card
        title="Cognitive Capability Field"
        subtitle="Live node intensity propagation — hover nodes for details"
        className="flex-1 min-h-0"
        actions={
          <div className="flex items-center gap-2">
            {Object.entries(TYPE_COLORS).map(([type, color]) => (
              <div key={type} className="flex items-center gap-1 text-[9px] text-slate-500">
                <div className="w-2 h-2 rounded-full" style={{ background: color }} />
                <span>{TYPE_LABELS[type as FieldNode['type']]}</span>
                <span className="font-mono text-slate-600">
                  ({typeCounts[type as FieldNode['type']]})
                </span>
              </div>
            ))}
          </div>
        }
      >
        <div className="relative rounded-lg overflow-hidden border border-border flex-1 min-h-[200px]">
          <canvas
            ref={canvasRef}
            width={dims.width}
            height={dims.height}
            className="absolute inset-0 w-full h-full cursor-crosshair"
            onMouseMove={handleMouseMove}
            onMouseLeave={() => setHoveredId(null)}
          />
        </div>

        <div className="flex items-center gap-4 text-[10px] text-slate-600 font-mono flex-wrap">
          <span>Node size ∝ intensity</span>
          <span>Edge opacity ∝ propagation weight</span>
          <span>Decay rate: 0.85/tick</span>
          <span className="ml-auto">{new Date(fieldSnapshot?.at ?? '').toLocaleTimeString()}</span>
        </div>
      </Card>

      {/* Node List */}
      <Card title="Active Field Nodes" subtitle="Sorted by intensity (descending)" className="shrink-0">
        <div className="overflow-auto rounded-lg border border-border" style={{ maxHeight: 200 }}>
          <table className="w-full">
            <thead className="sticky top-0 bg-surface-800">
              <tr>
                {['Node ID', 'Type', 'Label', 'Intensity'].map(h => (
                  <th key={h} className="px-3 py-2 text-left text-[10px] uppercase tracking-widest text-slate-500">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {(fieldSnapshot?.nodes ?? [])
                .slice()
                .sort((a, b) => b.intensity - a.intensity)
                .map(node => (
                  <tr
                    key={node.id}
                    className={`text-xs border-b border-border/50 hover:bg-surface-600 transition-colors ${
                      node.id === hoveredId ? 'bg-surface-600' : ''
                    }`}
                  >
                    <td className="px-3 py-1.5 font-mono text-slate-500">{node.id}</td>
                    <td className="px-3 py-1.5">
                      <Badge
                        label={node.type}
                        variant={node.type === 'process' ? 'red' : node.type === 'file' ? 'blue' : 'green'}
                        size="xs"
                      />
                    </td>
                    <td className="px-3 py-1.5 font-mono text-slate-300 truncate max-w-48">{node.label}</td>
                    <td className="px-3 py-1.5">
                      <div className="flex items-center gap-2">
                        <div className="w-16 h-1.5 bg-surface-500 rounded-full overflow-hidden">
                          <div
                            className="h-full rounded-full"
                            style={{
                              width: `${node.intensity * 100}%`,
                              background: TYPE_COLORS[node.type],
                            }}
                          />
                        </div>
                        <span className="font-mono text-[10px]" style={{ color: TYPE_COLORS[node.type] }}>
                          {node.intensity.toFixed(3)}
                        </span>
                      </div>
                    </td>
                  </tr>
                ))}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  );
};
