import type {
  Detection, SystemStatus, QuarantinedFile, AgentConfig,
  FieldSnapshot, FieldNode, FieldEdge, Severity, ActionTaken
} from '../types';

// ─── Helpers ──────────────────────────────────────────────────────────────────

const rnd = (min: number, max: number) => Math.random() * (max - min) + min;
const rndInt = (min: number, max: number) => Math.floor(rnd(min, max));
const pick = <T>(arr: T[]): T => arr[rndInt(0, arr.length)];

const PROCESS_NAMES = [
  'openssl', 'python3', 'node', 'bash', 'curl', 'wget',
  'tar', 'dd', 'find', 'rsync', 'cryptsetup', 'gpg',
  'suspicious_enc', 'ransom_dropper', 'enc_runner',
];

const FILE_PATHS = [
  '/home/user/documents/report.pdf',
  '/home/user/photos/vacation.jpg',
  '/var/lib/mysql/data.db',
  '/etc/passwd',
  '/home/user/.ssh/id_rsa',
  '/home/user/projects/src/main.go',
  '/tmp/.hidden_payload',
  '/home/user/documents/financial_2024.xlsx',
];

let detectionIdCounter = 1;

export function generateMockDetection(): Detection {
  const scoreRaw = rnd(0.1, 1.0);
  const severity: Severity =
    scoreRaw >= 0.65 ? 'ALERT' : scoreRaw >= 0.30 ? 'WARNING' : 'NONE';
  const actions: ActionTaken[] = severity === 'ALERT'
    ? ['SIGSTOP', 'SIGKILL', 'QUARANTINE']
    : severity === 'WARNING'
    ? ['SIGSTOP', 'NONE']
    : ['NONE'];

  const cfer = rnd(0, 2.5);
  const turbulence = rnd(0, 12);
  const shockwave = rnd(0, 5);
  const entropy = rnd(0, 6);

  return {
    id: `det-${detectionIdCounter++}`,
    timestamp: new Date(Date.now() - rndInt(0, 60000)).toISOString(),
    severity,
    score: Math.round(scoreRaw * 1000) / 1000,
    vector: {
      cfer: Math.round(cfer * 1000) / 1000,
      turbulence: Math.round(turbulence * 1000) / 1000,
      shockwave: Math.round(shockwave * 1000) / 1000,
      entropy: Math.round(entropy * 1000) / 1000,
      activeNodes: rndInt(1, 50),
      offenderPID: rndInt(1000, 65535),
      parentPID: rndInt(1, 1000),
    },
    action: pick(actions),
    reason: `composite score ${scoreRaw.toFixed(3)} | CFER=${cfer.toFixed(3)} turb=${turbulence.toFixed(3)} shock=${shockwave.toFixed(3)} entropy=${entropy.toFixed(3)}`,
    processName: pick(PROCESS_NAMES),
  };
}

export function generateMockStatus(): SystemStatus {
  return {
    status: 'RUNNING',
    uptime: rndInt(3600, 86400),
    totalWarnings: rndInt(5, 80),
    totalAlerts: rndInt(0, 12),
    monitoredProcesses: rndInt(80, 200),
    fieldNodes: rndInt(20, 150),
    eventsPerSecond: rndInt(200, 2000),
    cpuUsage: rnd(1, 15),
    memoryMb: rnd(30, 120),
    lastUpdated: new Date().toISOString(),
  };
}

let quarantineIdCounter = 1;
export function generateMockQuarantine(): QuarantinedFile[] {
  return Array.from({ length: rndInt(2, 6) }, () => ({
    id: `qfile-${quarantineIdCounter++}`,
    path: pick(FILE_PATHS),
    quarantinedAt: new Date(Date.now() - rndInt(60000, 3600000)).toISOString(),
    originPID: rndInt(1000, 65535),
    processName: pick(PROCESS_NAMES),
    size: rndInt(1024, 10 * 1024 * 1024),
    hash: Array.from({ length: 64 }, () =>
      Math.floor(Math.random() * 16).toString(16)
    ).join(''),
    status: 'quarantined' as const,
  }));
}

export function generateMockConfig(): AgentConfig {
  return {
    warningScore: 0.40,
    alertScore: 0.65,
    fastThreshold: 0.30,
    confirmMultiplier: 1.5,
    cferThreshold: 0.3,
    turbulenceThreshold: 8.0,
    shockwaveThreshold: 2.0,
    entropyThreshold: 3.0,
    enableSigstop: true,
    enableSigkill: true,
    enableQuarantine: true,
    decayRate: 0.85,
    windowSize: 30,
    snapshotIntervalMs: 500,
    jsonLogging: true,
    debugMode: false,
    dryRun: false,
  };
}

export function generateMockFieldSnapshot(): FieldSnapshot {
  const nodeCount = rndInt(8, 20);
  let nodeIdx = 0;
  const nodes: FieldNode[] = Array.from({ length: nodeCount }, () => {
    const isProcess = Math.random() > 0.4;
    return {
      id: `node-${nodeIdx++}`,
      label: isProcess ? `proc:${pick(PROCESS_NAMES)}` : `file:${pick(FILE_PATHS).split('/').pop()}`,
      intensity: rnd(0.05, 1.0),
      type: isProcess ? 'process' : (Math.random() > 0.5 ? 'file' : 'directory'),
    };
  });

  const edges: FieldEdge[] = [];
  nodes.forEach((n, i) => {
    if (i > 0 && Math.random() > 0.3) {
      edges.push({ source: n.id, target: nodes[rndInt(0, i)].id, weight: rnd(0.1, 1.0) });
    }
  });

  return {
    at: new Date().toISOString(),
    nodes,
    edges,
    norm: rnd(0.5, 8.0),
  };
}

export function generateTimeSeriesPoint(index: number) {
  const now = Date.now() - (300 - index) * 1000;
  const attackPhase = index > 200 && index < 280;
  const base = attackPhase ? rnd(0.4, 0.9) : rnd(0.0, 0.25);
  return {
    time: new Date(now).toLocaleTimeString('en-GB', { hour12: false }),
    timestamp: now,
    score: Math.round((base + rnd(-0.05, 0.05)) * 1000) / 1000,
    cfer: attackPhase ? rnd(0.3, 2.5) : rnd(0, 0.15),
    turbulence: attackPhase ? rnd(3, 12) : rnd(0, 0.8),
    shockwave: attackPhase && index < 220 ? rnd(1, 5) : rnd(0, 0.3),
    entropy: attackPhase ? rnd(3, 6) : rnd(0.5, 2),
    norm: attackPhase ? rnd(2, 8) : rnd(0.1, 1),
  };
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 ** 3) return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
  return `${(bytes / 1024 ** 3).toFixed(1)} GB`;
}

export function formatUptime(seconds: number): string {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  return `${h}h ${m}m ${s}s`;
}

export function formatTimestamp(iso: string): string {
  return new Date(iso).toLocaleTimeString('en-GB', {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}
