import type {
  Detection, SystemStatus, QuarantinedFile, AgentConfig,
  ActionRequest, ApiResponse, FieldSnapshot
} from '../types';

const BASE_URL = '/api';

async function request<T>(
  path: string,
  options?: RequestInit
): Promise<ApiResponse<T>> {
  try {
    const res = await fetch(`${BASE_URL}${path}`, {
      headers: { 'Content-Type': 'application/json', ...options?.headers },
      ...options,
    });
    if (!res.ok) {
      const text = await res.text();
      return { ok: false, data: null as T, error: text || res.statusText };
    }
    const data = (await res.json()) as T;
    return { ok: true, data };
  } catch (err) {
    return { ok: false, data: null as T, error: String(err) };
  }
}

// ─── Status ───────────────────────────────────────────────────────────────────

export const getStatus = () => request<SystemStatus>('/status');

// ─── Detections ───────────────────────────────────────────────────────────────

export const getDetections = (limit = 200) =>
  request<Detection[]>(`/detections?limit=${limit}`);

// ─── Quarantine ───────────────────────────────────────────────────────────────

export const getQuarantine = () => request<QuarantinedFile[]>('/quarantine');

// ─── Actions ──────────────────────────────────────────────────────────────────

export const postAction = (body: ActionRequest) =>
  request<{ ok: boolean }>('/action', {
    method: 'POST',
    body: JSON.stringify(body),
  });

export const resumeProcess = (pid: number) =>
  postAction({ action: 'resume', pid });

export const killProcess = (pid: number) =>
  postAction({ action: 'kill', pid });

export const quarantineFile = (pid: number) =>
  postAction({ action: 'quarantine', pid });

export const restoreFile = (fileId: string) =>
  postAction({ action: 'restore', fileId });

export const deleteFile = (fileId: string) =>
  postAction({ action: 'delete', fileId });

export const clearAlerts = () =>
  postAction({ action: 'clear_alerts' });

// ─── Config ───────────────────────────────────────────────────────────────────

export const getConfig = () => request<AgentConfig>('/config');

export const postConfig = (cfg: Partial<AgentConfig>) =>
  request<AgentConfig>('/config', {
    method: 'POST',
    body: JSON.stringify(cfg),
  });

// ─── Field ────────────────────────────────────────────────────────────────────

export const getFieldSnapshot = () => request<FieldSnapshot>('/field');
