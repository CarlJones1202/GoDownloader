import { useState, useEffect, useRef, useCallback } from 'react';

/** Shape of a single active queue item. */
export interface ActiveItem {
  id: number;
  url: string;
  type: string;
  started_at: number;
}

/** Aggregate queue counters. */
export interface QueueCounts {
  pending: number;
  active: number;
  completed: number;
  failed: number;
}

/** The payload broadcasted by the backend via WebSocket. */
export interface QueueStatus {
  type: 'queue_status';
  queue: QueueCounts;
  active: ActiveItem[];
  timestamp: number;
}

interface UseWebSocketOptions {
  /** Auto-connect on mount (default true). */
  enabled?: boolean;
}

/**
 * useWebSocket connects to the backend WebSocket at /ws and returns
 * the latest queue status. Automatically reconnects on disconnect.
 */
export function useWebSocket(opts: UseWebSocketOptions = {}) {
  const { enabled = true } = opts;
  const [status, setStatus] = useState<QueueStatus | null>(null);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  const connect = useCallback(() => {
    if (wsRef.current) return;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws`;
    const socket = new WebSocket(wsUrl);

    socket.onopen = () => {
      setConnected(true);
    };

    socket.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data) as QueueStatus;
        if (data.type === 'queue_status') {
          setStatus(data);
        }
      } catch {
        // Ignore malformed messages.
      }
    };

    socket.onclose = () => {
      setConnected(false);
      wsRef.current = null;
      // Reconnect after 3 seconds.
      reconnectTimer.current = setTimeout(connect, 3000);
    };

    socket.onerror = () => {
      socket.close();
    };

    wsRef.current = socket;
  }, []);

  useEffect(() => {
    if (!enabled) return;
    connect();
    return () => {
      clearTimeout(reconnectTimer.current);
      wsRef.current?.close();
      wsRef.current = null;
    };
  }, [enabled, connect]);

  return { status, connected };
}
