import type {
  Detection, SystemStatus, QuarantinedFile, AgentConfig,
  TimeSeriesPoint, FieldSnapshot, Notification, NavPage, UserRole
} from '../types';
import { generateMockStatus, generateMockConfig } from '../utils/mockData';
import { create } from 'zustand';

const MAX_DETECTIONS = 1000;
const MAX_TIMESERIES = 300;

interface AppState {
  // ── Navigation ──────────────────────────────────────────────────────────────
  activePage: NavPage;
  setActivePage: (page: NavPage) => void;

  // ── Auth / Role ──────────────────────────────────────────────────────────────
  role: UserRole;
  setRole: (role: UserRole) => void;

  // ── Connection ───────────────────────────────────────────────────────────────
  wsConnected: boolean;
  setWsConnected: (v: boolean) => void;

  // ── System Status ────────────────────────────────────────────────────────────
  status: SystemStatus;
  setStatus: (s: SystemStatus) => void;

  // ── Detections ───────────────────────────────────────────────────────────────
  detections: Detection[];
  addDetection: (d: Detection) => void;
  clearDetections: () => void;

  // ── Stream control ───────────────────────────────────────────────────────────
  streamPaused: boolean;
  toggleStreamPause: () => void;
  severityFilter: 'ALL' | 'WARNING' | 'ALERT';
  setSeverityFilter: (f: 'ALL' | 'WARNING' | 'ALERT') => void;
  pidSearch: string;
  setPidSearch: (s: string) => void;

  // ── Timeseries ───────────────────────────────────────────────────────────────
  timeSeries: TimeSeriesPoint[];
  addTimeSeriesPoint: (p: TimeSeriesPoint) => void;

  // ── Quarantine ───────────────────────────────────────────────────────────────
  quarantine: QuarantinedFile[];
  setQuarantine: (files: QuarantinedFile[]) => void;
  removeQuarantineFile: (id: string) => void;

  // ── Config ───────────────────────────────────────────────────────────────────
  config: AgentConfig;
  setConfig: (c: Partial<AgentConfig>) => void;

  // ── Field ────────────────────────────────────────────────────────────────────
  fieldSnapshot: FieldSnapshot | null;
  setFieldSnapshot: (f: FieldSnapshot) => void;

  // ── Notifications ────────────────────────────────────────────────────────────
  notifications: Notification[];
  addNotification: (n: Omit<Notification, 'id' | 'timestamp' | 'read'>) => void;
  markAllRead: () => void;
  clearNotifications: () => void;

  // ── Mock mode ────────────────────────────────────────────────────────────────
  mockMode: boolean;
  setMockMode: (v: boolean) => void;
}

export const useStore = create<AppState>((set) => ({
  // Navigation
  activePage: 'dashboard',
  setActivePage: (page) => set({ activePage: page }),

  // Auth
  role: 'admin',
  setRole: (role) => set({ role }),

  // Connection
  wsConnected: false,
  setWsConnected: (v) => set({ wsConnected: v }),

  // Status
  status: generateMockStatus(),
  setStatus: (s) => set({ status: s }),

  // Detections
  detections: [],
  addDetection: (d) =>
    set((state) => ({
      detections: [d, ...state.detections].slice(0, MAX_DETECTIONS),
    })),
  clearDetections: () => set({ detections: [] }),

  // Stream control
  streamPaused: false,
  toggleStreamPause: () => set((s) => ({ streamPaused: !s.streamPaused })),
  severityFilter: 'ALL',
  setSeverityFilter: (f) => set({ severityFilter: f }),
  pidSearch: '',
  setPidSearch: (s) => set({ pidSearch: s }),

  // Timeseries
  timeSeries: [],
  addTimeSeriesPoint: (p) =>
    set((state) => ({
      timeSeries: [...state.timeSeries, p].slice(-MAX_TIMESERIES),
    })),

  // Quarantine
  quarantine: [],
  setQuarantine: (files) => set({ quarantine: files }),
  removeQuarantineFile: (id) =>
    set((state) => ({
      quarantine: state.quarantine.filter((f) => f.id !== id),
    })),

  // Config
  config: generateMockConfig(),
  setConfig: (c) => set((state) => ({ config: { ...state.config, ...c } })),

  // Field
  fieldSnapshot: null,
  setFieldSnapshot: (f) => set({ fieldSnapshot: f }),

  // Notifications
  notifications: [],
  addNotification: (n) =>
    set((state) => ({
      notifications: [
        {
          ...n,
          id: crypto.randomUUID(),
          timestamp: Date.now(),
          read: false,
        },
        ...state.notifications,
      ].slice(0, 50),
    })),
  markAllRead: () =>
    set((state) => ({
      notifications: state.notifications.map((n) => ({ ...n, read: true })),
    })),
  clearNotifications: () => set({ notifications: [] }),

  // Mock
  mockMode: false,
  setMockMode: (v) => set({ mockMode: v }),
}));
