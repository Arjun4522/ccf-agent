// ─── Core Domain Types ────────────────────────────────────────────────────────

export type Severity = 'NONE' | 'NORMAL' | 'WARNING' | 'ALERT';

export type ActionTaken = 'SIGSTOP' | 'SIGKILL' | 'QUARANTINE' | 'NONE';

export interface FeatureVector {
  cfer: number;        // Capability Field Expansion Rate (regression slope)
  turbulence: number;  // Variance of field norm
  shockwave: number;   // Second derivative of field norm
  entropy: number;     // Shannon entropy of node intensity distribution
  activeNodes: number; // Number of nodes with non-zero intensity
  offenderPID: number; // PID of most active node's process
  parentPID: number;   // Parent PID of offender
}

export interface Detection {
  id: string;
  timestamp: string;       // ISO 8601
  severity: Severity;
  score: number;           // Composite [0, 1]
  vector: FeatureVector;
  action: ActionTaken;
  reason: string;
  processName?: string;
}

// ─── Status / Health ─────────────────────────────────────────────────────────

export type AgentStatus = 'RUNNING' | 'ALERT' | 'IDLE' | 'ERROR';

export interface SystemStatus {
  status: AgentStatus;
  uptime: number;          // seconds
  totalWarnings: number;
  totalAlerts: number;
  monitoredProcesses: number;
  fieldNodes: number;
  eventsPerSecond: number;
  cpuUsage: number;        // 0-100
  memoryMb: number;
  lastUpdated: string;
}

// ─── Quarantine ───────────────────────────────────────────────────────────────

export interface QuarantinedFile {
  id: string;
  path: string;
  quarantinedAt: string;   // ISO 8601
  originPID: number;
  processName: string;
  size: number;            // bytes
  hash: string;            // SHA-256
  status: 'quarantined' | 'restoring' | 'deleting';
}

// ─── Configuration ────────────────────────────────────────────────────────────

export interface AgentConfig {
  // Thresholds
  warningScore: number;
  alertScore: number;
  fastThreshold: number;
  confirmMultiplier: number;

  // Feature thresholds
  cferThreshold: number;
  turbulenceThreshold: number;
  shockwaveThreshold: number;
  entropyThreshold: number;

  // Response toggles
  enableSigstop: boolean;
  enableSigkill: boolean;
  enableQuarantine: boolean;

  // Field parameters
  decayRate: number;
  windowSize: number;
  snapshotIntervalMs: number;

  // Logging
  jsonLogging: boolean;
  debugMode: boolean;
  dryRun: boolean;
}

// ─── Field Visualization ──────────────────────────────────────────────────────

export interface FieldNode {
  id: string;
  label: string;
  intensity: number;
  x?: number;
  y?: number;
  type: 'process' | 'file' | 'directory';
}

export interface FieldEdge {
  source: string;
  target: string;
  weight: number;
}

export interface FieldSnapshot {
  at: string;
  nodes: FieldNode[];
  edges: FieldEdge[];
  norm: number;
}

// ─── Analytics / Chart Data ───────────────────────────────────────────────────

export interface TimeSeriesPoint {
  time: string;      // HH:MM:SS
  timestamp: number; // epoch ms
  score?: number;
  cfer?: number;
  turbulence?: number;
  shockwave?: number;
  entropy?: number;
  norm?: number;
}

export interface SeverityDistribution {
  severity: Severity;
  count: number;
}

// ─── WebSocket Events ─────────────────────────────────────────────────────────

export type WSEventType = 'detection' | 'status' | 'field' | 'config' | 'ping';

export interface WSMessage<T = unknown> {
  type: WSEventType;
  payload: T;
}

// ─── API Response Shapes ──────────────────────────────────────────────────────

export interface ApiResponse<T> {
  data: T;
  ok: boolean;
  error?: string;
}

export interface ActionRequest {
  action: 'resume' | 'kill' | 'quarantine' | 'restore' | 'delete' | 'clear_alerts';
  pid?: number;
  fileId?: string;
}

// ─── UI State ─────────────────────────────────────────────────────────────────

export type NavPage =
  | 'dashboard'
  | 'detections'
  | 'analytics'
  | 'response'
  | 'quarantine'
  | 'field'
  | 'settings';

export type UserRole = 'admin' | 'viewer';

export interface Notification {
  id: string;
  type: 'info' | 'warning' | 'error' | 'success';
  title: string;
  message: string;
  timestamp: number;
  read: boolean;
}
