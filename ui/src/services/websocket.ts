import type { WSMessage } from '../types';

type MessageHandler = (msg: WSMessage) => void;
type StatusHandler = (connected: boolean) => void;

export class CCFWebSocket {
  private ws: WebSocket | null = null;
  private url: string;
  private handlers: Set<MessageHandler> = new Set();
  private statusHandlers: Set<StatusHandler> = new Set();
  private reconnectDelay = 1000;
  private maxDelay = 16000;
  private shouldReconnect = true;
  private pingInterval: ReturnType<typeof setInterval> | null = null;

  constructor(url = '/ws/detections') {
    // Convert relative WS path to absolute
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    this.url = url.startsWith('ws')
      ? url
      : `${proto}//${window.location.host}${url}`;
  }

  connect() {
    if (this.ws?.readyState === WebSocket.OPEN) return;
    try {
      this.ws = new WebSocket(this.url);
      this.ws.onopen = () => {
        this.reconnectDelay = 1000;
        this.statusHandlers.forEach(h => h(true));
        this.startPing();
      };
      this.ws.onmessage = (e) => {
        try {
          const msg = JSON.parse(e.data) as WSMessage;
          this.handlers.forEach(h => h(msg));
        } catch { /* ignore malformed */ }
      };
      this.ws.onclose = () => {
        this.statusHandlers.forEach(h => h(false));
        this.stopPing();
        if (this.shouldReconnect) {
          setTimeout(() => this.connect(), this.reconnectDelay);
          this.reconnectDelay = Math.min(this.reconnectDelay * 2, this.maxDelay);
        }
      };
      this.ws.onerror = () => {
        this.ws?.close();
      };
    } catch {
      if (this.shouldReconnect) {
        setTimeout(() => this.connect(), this.reconnectDelay);
      }
    }
  }

  disconnect() {
    this.shouldReconnect = false;
    this.stopPing();
    this.ws?.close();
    this.ws = null;
  }

  onMessage(handler: MessageHandler) {
    this.handlers.add(handler);
    return () => this.handlers.delete(handler);
  }

  onStatus(handler: StatusHandler) {
    this.statusHandlers.add(handler);
    return () => this.statusHandlers.delete(handler);
  }

  get isConnected() {
    return this.ws?.readyState === WebSocket.OPEN;
  }

  private startPing() {
    this.pingInterval = setInterval(() => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        this.ws.send(JSON.stringify({ type: 'ping' }));
      }
    }, 15000);
  }

  private stopPing() {
    if (this.pingInterval) {
      clearInterval(this.pingInterval);
      this.pingInterval = null;
    }
  }
}

// Singleton instance
export const wsClient = new CCFWebSocket('/ws/detections');
