// Reconnecting websocket client with automatic token refresh on 401.

type WsStatusHandler = (status: WsStatus) => void;
type WsEventHandler = (event: WsEvent) => void;

class WsClient {
  private ws: WebSocket | null = null;
  private token = "";
  private attempt = 0;
  private closedByUs = false;
  private reconnectTimer: number | null = null;

  onStatus: WsStatusHandler = () => undefined;
  onEvent: WsEventHandler = () => undefined;

  connect(token: string): void {
    this.token = token;
    this.closedByUs = false;
    this.open();
  }

  private open(): void {
    if (this.ws && (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)) {
      return;
    }
    this.onStatus("connecting");
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const url = proto + "//" + window.location.host + "/ws?token=" + encodeURIComponent(this.token);

    let ws: WebSocket;
    try {
      ws = new WebSocket(url);
    } catch {
      this.scheduleReconnect();
      return;
    }
    this.ws = ws;

    ws.onopen = () => {
      this.attempt = 0;
      this.onStatus("open");
      this.startHeartbeat();
    };
    ws.onmessage = (ev: MessageEvent) => {
      if (typeof ev.data !== "string") {
        return;
      }
      try {
        const parsed = JSON.parse(ev.data) as WsEvent;
        if (parsed && typeof parsed.type === "string") {
          this.onEvent(parsed);
        }
      } catch {
        // Ignore malformed frames.
      }
    };
    ws.onclose = () => {
      this.ws = null;
      if (this.closedByUs) {
        this.onStatus("closed");
        return;
      }
      this.onStatus("closed");
      this.scheduleReconnect();
    };
    ws.onerror = () => {
      // onclose will follow.
    };
  }

  private heartbeat: number | null = null;

  private startHeartbeat(): void {
    if (this.heartbeat !== null) {
      window.clearInterval(this.heartbeat);
    }
    this.heartbeat = window.setInterval(() => this.send("ping"), 25000);
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimer !== null) {
      return;
    }
    this.attempt += 1;
    const delay = Math.min(15000, 500 * Math.pow(2, Math.min(this.attempt, 5)));
    this.reconnectTimer = window.setTimeout(() => {
      this.reconnectTimer = null;
      this.open();
    }, delay);
  }

  send(type: string, data?: Record<string, unknown>): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return;
    }
    if (type === "ping") {
      this.ws.send(JSON.stringify({ type: "ping" }));
      return;
    }
    this.ws.send(JSON.stringify({ type, data: data ?? {} }));
  }

  close(): void {
    this.closedByUs = true;
    if (this.reconnectTimer !== null) {
      window.clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.heartbeat !== null) {
      window.clearInterval(this.heartbeat);
      this.heartbeat = null;
    }
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }
}

const socket = new WsClient();
