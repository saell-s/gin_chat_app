// Shared API types. Type-only file: emits no runtime code.

interface User {
  id: number;
  email: string;
  name: string;
  role: string;
  status: string;
  created_at: string;
  updated_at: string;
}

interface Message {
  id: number;
  conversation_id: number;
  sender_id: number;
  kind: string;
  body: string;
  created_at: string;
}

interface Conversation {
  id: number;
  type: string;
  peer?: User;
  last_message?: Message;
  unread: number;
  last_read_message_id: number;
  updated_at: string;
  created_at: string;
}

interface CallRecord {
  id: number;
  caller_id: number;
  callee_id: number;
  conversation_id: number;
  kind: string;
  status: string;
  started_at: string;
  answered_at?: string;
  ended_at?: string;
  duration_seconds: number;
  created_at: string;
  peer_id: number;
  direction: string;
}

interface TokenPair {
  access_token: string;
  refresh_token: string;
  token_type: string;
  expires_in: number;
  expires_at: string;
  user: User;
}

interface PageMeta {
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

interface Envelope<T> {
  data: T;
  page?: PageMeta;
}

interface PresenceSnapshot {
  online_ids: number[];
}

interface WsEvent {
  type: string;
  data?: Record<string, any>;
}

type CallState = "idle" | "outgoing" | "incoming" | "active";
type WsStatus = "connecting" | "open" | "closed";
type PanelKind = "none" | "new" | "calls";

interface ActiveCall {
  state: CallState;
  callId: number;
  peerId: number;
  kind: string;
  muted: boolean;
  camOff: boolean;
  startedAt: number;
  ringSince: number;
}
