import { useEffect, useRef } from 'react';
import { wsClient } from '../services/websocket';
import { useStore } from '../store';
import type { WSMessage, Detection, SystemStatus, FieldSnapshot } from '../types';
import { generateMockDetection, generateMockStatus, generateTimeSeriesPoint, generateMockFieldSnapshot } from '../utils/mockData';
import { getStatus, getDetections, getQuarantine, getConfig, getFieldSnapshot } from '../services/api';

export function useWebSocket() {
  const setWsConnected = useStore(s => s.setWsConnected);
  const addDetection = useStore(s => s.addDetection);
  const setStatus = useStore(s => s.setStatus);
  const setFieldSnapshot = useStore(s => s.setFieldSnapshot);
  const addTimeSeriesPoint = useStore(s => s.addTimeSeriesPoint);
  const addNotification = useStore(s => s.addNotification);
  const setQuarantine = useStore(s => s.setQuarantine);
  const setConfig = useStore(s => s.setConfig);
  const streamPaused = useStore(s => s.streamPaused);
  const mockMode = useStore(s => s.mockMode);
  const streamPausedRef = useRef(streamPaused);
  streamPausedRef.current = streamPaused;

  useEffect(() => {
    if (mockMode) {
      // Seed initial time series
      for (let i = 0; i < 120; i++) {
        addTimeSeriesPoint(generateTimeSeriesPoint(i));
      }

      // Simulate live WS stream in mock mode
      let tick = 120;
      const interval = setInterval(() => {
        if (streamPausedRef.current) return;
        tick++;

        // New detection every ~3s
        if (tick % 3 === 0) {
          const det = generateMockDetection();
          addDetection(det);
          if (det.severity === 'ALERT') {
            addNotification({
              type: 'error',
              title: 'ALERT — Ransomware Detected',
              message: `Score ${det.score.toFixed(3)} | PID ${det.vector.offenderPID} | ${det.processName}`,
            });
          } else if (det.severity === 'WARNING') {
            addNotification({
              type: 'warning',
              title: 'WARNING — Suspicious Activity',
              message: `Score ${det.score.toFixed(3)} | PID ${det.vector.offenderPID}`,
            });
          }
        }

        // Status update every 2s
        if (tick % 2 === 0) {
          setStatus(generateMockStatus());
        }

        // Timeseries point every tick (1s)
        addTimeSeriesPoint(generateTimeSeriesPoint(tick));

        // Field update every 5s
        if (tick % 5 === 0) {
          setFieldSnapshot(generateMockFieldSnapshot());
        }
      }, 1000);

      setWsConnected(true);
      return () => {
        clearInterval(interval);
        setWsConnected(false);
      };
    }

    // ── Real mode: load initial state from REST APIs ──────────────────────────
    let cancelled = false;

    async function loadInitialData() {
      const [statusRes, detectionsRes, quarantineRes, configRes, fieldRes] = await Promise.allSettled([
        getStatus(),
        getDetections(200),
        getQuarantine(),
        getConfig(),
        getFieldSnapshot(),
      ]);

      if (cancelled) return;

      if (statusRes.status === 'fulfilled' && statusRes.value.ok) {
        setStatus(statusRes.value.data);
      }
      if (detectionsRes.status === 'fulfilled' && detectionsRes.value.ok) {
        const dets = detectionsRes.value.data ?? [];
        // Add oldest-first so newest ends up at top after addDetection prepends.
        for (let i = dets.length - 1; i >= 0; i--) {
          const det = dets[i];
          addDetection(det);
          addTimeSeriesPoint({
            time: new Date(det.timestamp).toLocaleTimeString('en-GB'),
            timestamp: new Date(det.timestamp).getTime(),
            score: det.score,
            cfer: det.vector.cfer,
            turbulence: det.vector.turbulence,
            shockwave: det.vector.shockwave,
            entropy: det.vector.entropy,
          });
        }
      }
      if (quarantineRes.status === 'fulfilled' && quarantineRes.value.ok) {
        setQuarantine(quarantineRes.value.data ?? []);
      }
      if (configRes.status === 'fulfilled' && configRes.value.ok) {
        setConfig(configRes.value.data);
      }
      if (fieldRes.status === 'fulfilled' && fieldRes.value.ok) {
        setFieldSnapshot(fieldRes.value.data);
      }
    }

    loadInitialData();

    // ── Real WS mode ─────────────────────────────────────────────────────────
    wsClient.connect();

    const unsubStatus = wsClient.onStatus(setWsConnected);
    const unsubMsg = wsClient.onMessage((msg: WSMessage) => {
      if (streamPausedRef.current && msg.type === 'detection') return;
      switch (msg.type) {
        case 'detection': {
          const det = msg.payload as Detection;
          addDetection(det);
          addTimeSeriesPoint({
            time: new Date(det.timestamp).toLocaleTimeString('en-GB'),
            timestamp: new Date(det.timestamp).getTime(),
            score: det.score,
            cfer: det.vector.cfer,
            turbulence: det.vector.turbulence,
            shockwave: det.vector.shockwave,
            entropy: det.vector.entropy,
          });
          if (det.severity === 'ALERT') {
            addNotification({
              type: 'error',
              title: 'ALERT Detected',
              message: `Score ${det.score.toFixed(3)} PID ${det.vector.offenderPID}`,
            });
          }
          break;
        }
        case 'status':
          setStatus(msg.payload as SystemStatus);
          break;
        case 'field':
          setFieldSnapshot(msg.payload as FieldSnapshot);
          break;
      }
    });

    return () => {
      cancelled = true;
      unsubStatus();
      unsubMsg();
      wsClient.disconnect();
    };
  }, [mockMode]); // eslint-disable-line react-hooks/exhaustive-deps
}

