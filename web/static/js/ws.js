"use strict";
// Reconnecting websocket client with automatic token refresh on 401.
class WsClient {
    constructor() {
        this.ws = null;
        this.token = "";
        this.attempt = 0;
        this.closedByUs = false;
        this.reconnectTimer = null;
        this.onStatus = () => undefined;
        this.onEvent = () => undefined;
        this.heartbeat = null;
    }
    connect(token) {
        this.token = token;
        this.closedByUs = false;
        this.open();
    }
    open() {
        if (this.ws && (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)) {
            return;
        }
        this.onStatus("connecting");
        const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
        const url = proto + "//" + window.location.host + "/ws?token=" + encodeURIComponent(this.token);
        let ws;
        try {
            ws = new WebSocket(url);
        }
        catch {
            this.scheduleReconnect();
            return;
        }
        this.ws = ws;
        ws.onopen = () => {
            this.attempt = 0;
            this.onStatus("open");
            this.startHeartbeat();
        };
        ws.onmessage = (ev) => {
            if (typeof ev.data !== "string") {
                return;
            }
            try {
                const parsed = JSON.parse(ev.data);
                if (parsed && typeof parsed.type === "string") {
                    this.onEvent(parsed);
                }
            }
            catch {
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
    startHeartbeat() {
        if (this.heartbeat !== null) {
            window.clearInterval(this.heartbeat);
        }
        this.heartbeat = window.setInterval(() => this.send("ping"), 25000);
    }
    scheduleReconnect() {
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
    send(type, data) {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            return;
        }
        if (type === "ping") {
            this.ws.send(JSON.stringify({ type: "ping" }));
            return;
        }
        this.ws.send(JSON.stringify({ type, data: data ?? {} }));
    }
    close() {
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
