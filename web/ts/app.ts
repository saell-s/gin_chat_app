// Alpine components: the login form and the chat/call application.

const AVATAR_COLORS = [
  "#00a884",
  "#027eb5",
  "#7f66ff",
  "#e8a33d",
  "#d45d7c",
  "#2e7d32",
  "#00838f",
  "#8d6e63",
];

function avatarColor(id: number): string {
  const idx = Math.abs(Math.floor(id)) % AVATAR_COLORS.length;
  return AVATAR_COLORS[idx];
}

function initialsOf(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) {
    return "?";
  }
  if (parts.length === 1) {
    return parts[0].substring(0, 2).toUpperCase();
  }
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

function elRef(name: string): HTMLElement | null {
  return document.querySelector('[x-ref="' + name + '"]');
}

function messageOf(err: unknown): string {
  if (err instanceof ApiError) {
    return err.message;
  }
  if (err instanceof Error) {
    return err.message;
  }
  return "Unexpected error";
}

function clockTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) {
    return "";
  }
  const now = new Date();
  const sameDay =
    d.getFullYear() === now.getFullYear() &&
    d.getMonth() === now.getMonth() &&
    d.getDate() === now.getDate();
  const hh = String(d.getHours()).padStart(2, "0");
  const mm = String(d.getMinutes()).padStart(2, "0");
  if (sameDay) {
    return hh + ":" + mm;
  }
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric" }) + " " + hh + ":" + mm;
}

function formatDuration(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds));
  const m = Math.floor(s / 60);
  const rest = s % 60;
  if (m >= 60) {
    const h = Math.floor(m / 60);
    return h + "h " + (m % 60) + "m";
  }
  return m > 0 ? m + "m " + rest + "s" : rest + "s";
}

class LoginForm {
  mode = "signin";
  email = "";
  password = "";
  name = "";
  error = "";
  loading = false;

  switchMode(mode: string): void {
    this.mode = mode;
    this.error = "";
  }

  fill(email: string, password: string): void {
    this.mode = "signin";
    this.email = email;
    this.password = password;
    this.error = "";
  }

  async submit(): Promise<void> {
    if (this.loading) {
      return;
    }
    this.error = "";
    this.loading = true;
    try {
      let pair: TokenPair;
      if (this.mode === "signup") {
        pair = await api.post<TokenPair>("/auth/register", {
          email: this.email,
          password: this.password,
          name: this.name || this.email.split("@")[0],
        });
      } else {
        pair = await api.post<TokenPair>("/auth/login", {
          email: this.email,
          password: this.password,
        });
      }
      api.setTokens(pair.access_token, pair.refresh_token);
      window.location.href = "/app";
    } catch (err) {
      this.error = messageOf(err);
    } finally {
      this.loading = false;
    }
  }
}

class ChatApp {
  me: User | null = null;
  users: User[] = [];
  conversations: Conversation[] = [];
  messages: Message[] = [];
  calls: CallRecord[] = [];
  onlineIds: number[] = [];
  typingMap: Record<string, boolean> = {};
  peerReads: Record<string, number> = {};
  activeId: number | null = null;
  draft = "";
  search = "";
  panel: PanelKind = "none";
  wsStatus: WsStatus = "connecting";
  toast = "";
  callNow = 0;
  call: ActiveCall = {
    state: "idle",
    callId: 0,
    peerId: 0,
    kind: "audio",
    muted: false,
    camOff: false,
    startedAt: 0,
    ringSince: 0,
  };

  private mgr: CallManager | null = null;
  private typingTimer: number | null = null;
  private typingSent = false;
  private peerTypingTimer: number | null = null;
  private toastTimer: number | null = null;
  private callTicker: number | null = null;
  private typingSentFor = 0;

  // ---- getters ----

  get meId(): number {
    return this.me ? this.me.id : -1;
  }

  get activeConversation(): Conversation | null {
    if (this.activeId === null) {
      return null;
    }
    return this.conversations.find((c) => c.id === this.activeId) ?? null;
  }

  get filteredConversations(): Conversation[] {
    const q = this.search.trim().toLowerCase();
    const list = q
      ? this.conversations.filter((c) => this.nameOf(this.peerIdOf(c)).toLowerCase().includes(q))
      : this.conversations;
    return list;
  }

  get directory(): User[] {
    return this.users.filter((u) => u.id !== this.meId);
  }

  get peerTyping(): boolean {
    if (this.activeId === null) {
      return false;
    }
    return !!this.typingMap[String(this.activeId)];
  }

  get unreadTotal(): number {
    return this.conversations.reduce((sum, c) => sum + c.unread, 0);
  }

  // ---- boot ----

  async boot(): Promise<void> {
    api.loadTokens();
    if (!api.hasSession()) {
      window.location.href = "/";
      return;
    }
    try {
      this.me = await api.get<User>("/auth/me");
    } catch {
      window.location.href = "/";
      return;
    }

    socket.onStatus = (status: WsStatus) => {
      this.wsStatus = status;
    };
    socket.onEvent = (ev: WsEvent) => {
      void this.onEvent(ev);
    };
    socket.connect(api.token() || "");

    try {
      const [convs, users, presence] = await Promise.all([
        api.get<Conversation[]>("/conversations"),
        api.get<User[]>("/directory"),
        api.get<PresenceSnapshot>("/presence"),
      ]);
      this.conversations = convs;
      this.users = users;
      this.onlineIds = presence.online_ids ?? [];
      this.sortConversations();
    } catch (err) {
      this.notify(messageOf(err));
    }
    void this.loadCalls();
  }

  // ---- websocket ----

  private async onEvent(ev: WsEvent): Promise<void> {
    const data = ev.data ?? {};
    switch (ev.type) {
      case "presence.snapshot":
        this.onlineIds = Array.isArray(data.online_ids) ? data.online_ids : [];
        break;
      case "presence.update": {
        const id = Number(data.user_id) || 0;
        const online = !!data.online;
        if (online && !this.onlineIds.includes(id)) {
          this.onlineIds = [...this.onlineIds, id];
        } else if (!online) {
          this.onlineIds = this.onlineIds.filter((x) => x !== id);
        }
        break;
      }
      case "message.created": {
        const convId = Number(data.conversation_id) || 0;
        const msg = data.message as Message | undefined;
        if (msg) {
          this.onMessage(convId, msg);
        }
        break;
      }
      case "typing": {
        const convId = Number(data.conversation_id) || 0;
        const from = Number(data.user_id) || 0;
        if (from === this.meId) {
          break;
        }
        this.typingMap[String(convId)] = !!data.is_typing;
        if (this.peerTypingTimer !== null) {
          window.clearTimeout(this.peerTypingTimer);
          this.peerTypingTimer = null;
        }
        if (data.is_typing) {
          this.peerTypingTimer = window.setTimeout(() => {
            this.typingMap[String(convId)] = false;
            this.peerTypingTimer = null;
          }, 3000);
        }
        break;
      }
      case "read.receipt": {
        const convId = Number(data.conversation_id) || 0;
        const last = Number(data.last_read_id) || 0;
        const current = this.peerReads[String(convId)] ?? 0;
        if (last > current) {
          this.peerReads = { ...this.peerReads, [String(convId)]: last };
        }
        break;
      }
      case "call.ringing": {
        const call = data.call as CallRecord | undefined;
        if (call && this.call.state === "outgoing") {
          this.call.callId = call.id;
        }
        break;
      }
      case "call.incoming": {
        const call = data.call as CallRecord | undefined;
        if (!call) {
          break;
        }
        if (this.call.state !== "idle") {
          socket.send("call.decline", { call_id: call.id });
          break;
        }
        this.call = {
          state: "incoming",
          callId: call.id,
          peerId: call.caller_id,
          kind: call.kind,
          muted: false,
          camOff: false,
          startedAt: 0,
          ringSince: Date.now(),
        };
        this.startCallTicker();
        break;
      }
      case "call.accepted":
      case "call.active": {
        if (this.call.state === "outgoing" || this.call.state === "incoming") {
          this.call.state = "active";
          this.call.startedAt = Date.now();
          this.callNow = this.call.startedAt;
          const isCaller = ev.type === "call.accepted";
          await this.beginMedia(isCaller);
        }
        break;
      }
      case "call.declined": {
        if (this.call.state !== "idle") {
          this.notify("Call declined");
          this.resetCall();
        }
        break;
      }
      case "call.ended": {
        const call = data.call as CallRecord | undefined;
        if (call && this.call.callId && call.id !== this.call.callId) {
          break;
        }
        if (this.call.state !== "idle") {
          const reason = String(data.reason || "");
          if (reason === "no answer") {
            this.notify("No answer");
          }
          this.resetCall();
        }
        break;
      }
      case "call.offer": {
        const sdp = String(data.sdp || "");
        if (!sdp) {
          break;
        }
        if (!this.mgr) {
          await this.beginMedia(false);
        }
        if (this.mgr) {
          await this.mgr.handleOffer(sdp);
        }
        break;
      }
      case "call.answer": {
        const sdp = String(data.sdp || "");
        if (sdp && this.mgr) {
          await this.mgr.handleAnswer(sdp);
        }
        break;
      }
      case "call.ice": {
        if (this.mgr && data.candidate) {
          await this.mgr.addIce(data.candidate);
        }
        break;
      }
      case "error":
        this.notify(String(data.message || "Something went wrong"));
        break;
      default:
        break;
    }
  }

  // ---- conversations ----

  private findConv(id: number): Conversation | undefined {
    return this.conversations.find((c) => c.id === id);
  }

  private sortConversations(): void {
    this.conversations = [...this.conversations].sort(
      (a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime(),
    );
  }

  private async refreshConversations(): Promise<void> {
    try {
      this.conversations = await api.get<Conversation[]>("/conversations");
      this.sortConversations();
    } catch (err) {
      this.notify(messageOf(err));
    }
  }

  private onMessage(convId: number, msg: Message): void {
    const conv = this.findConv(convId);
    if (conv) {
      conv.last_message = msg;
      conv.updated_at = msg.created_at;
      if (msg.sender_id !== this.meId && this.activeId !== convId) {
        conv.unread += 1;
      }
      this.sortConversations();
    } else {
      void this.refreshConversations();
    }

    if (this.activeId === convId) {
      this.upsertMessage(msg);
      this.scrollPane(false);
      if (msg.sender_id !== this.meId) {
        void this.markRead(convId);
      }
    }
  }

  private upsertMessage(msg: Message): void {
    const idx = this.messages.findIndex((m) => m.id === msg.id);
    if (idx >= 0) {
      this.messages[idx] = msg;
      return;
    }
    this.messages = [...this.messages, msg].sort((a, b) => a.id - b.id);
  }

  async selectConversation(id: number): Promise<void> {
    this.activeId = id;
    this.panel = "none";
    this.messages = [];
    this.typingMap[String(id)] = false;
    try {
      this.messages = await api.get<Message[]>(`/conversations/${id}/messages?limit=100`);
      await this.markRead(id);
    } catch (err) {
      this.notify(messageOf(err));
    }
    this.scrollPane(true);
    const composer = elRef("composer") as HTMLTextAreaElement | null;
    if (composer) {
      composer.focus();
    }
  }

  private async markRead(convId: number): Promise<void> {
    const conv = this.findConv(convId);
    if (!conv) {
      return;
    }
    let last = this.messages.length > 0 ? this.messages[this.messages.length - 1].id : 0;
    if (last === 0 && conv.last_message) {
      last = conv.last_message.id;
    }
    if (last <= conv.last_read_message_id && conv.unread === 0) {
      return;
    }
    try {
      await api.post(`/conversations/${convId}/read`, { last_read_id: last });
    } catch {
      return;
    }
    conv.unread = 0;
    if (last > conv.last_read_message_id) {
      conv.last_read_message_id = last;
    }
  }

  async startChat(userId: number): Promise<void> {
    try {
      const conv = await api.post<Conversation>("/conversations", { user_id: userId });
      await this.refreshConversations();
      this.panel = "none";
      await this.selectConversation(conv.id);
    } catch (err) {
      this.notify(messageOf(err));
    }
  }

  send(): void {
    const body = this.draft.trim();
    const convId = this.activeId;
    if (!body || convId === null) {
      return;
    }
    this.draft = "";
    this.stopTyping(true);
    this.resizeComposer();
    api
      .post<Message>(`/conversations/${convId}/messages`, { body })
      .then((msg) => this.onMessage(convId, msg))
      .catch((err: unknown) => {
        this.draft = body;
        this.notify(messageOf(err));
      });
  }

  onDraft(): void {
    this.resizeComposer();
    if (this.activeId !== null && this.draft.trim().length > 0) {
      if (!this.typingSent || this.typingSentFor !== this.activeId) {
        socket.send("typing.start", { conversation_id: this.activeId });
        this.typingSent = true;
        this.typingSentFor = this.activeId;
      }
      if (this.typingTimer !== null) {
        window.clearTimeout(this.typingTimer);
      }
      this.typingTimer = window.setTimeout(() => this.stopTyping(false), 1500);
    } else {
      this.stopTyping(false);
    }
  }

  private stopTyping(force: boolean): void {
    if (this.typingTimer !== null) {
      window.clearTimeout(this.typingTimer);
      this.typingTimer = null;
    }
    if (this.typingSent && this.activeId !== null && (force || this.draft.trim().length === 0)) {
      socket.send("typing.stop", { conversation_id: this.activeId });
      this.typingSent = false;
    }
  }

  private resizeComposer(): void {
    const ta = elRef("composer") as HTMLTextAreaElement | null;
    if (!ta) {
      return;
    }
    ta.style.height = "auto";
    ta.style.height = Math.min(ta.scrollHeight, 128) + "px";
  }

  private scrollPane(force: boolean): void {
    const pane = elRef("pane");
    if (!pane) {
      return;
    }
    const nearBottom = pane.scrollHeight - pane.scrollTop - pane.clientHeight < 200;
    if (force || nearBottom) {
      pane.scrollTop = pane.scrollHeight;
    }
  }

  // ---- calls ----

  async loadCalls(): Promise<void> {
    try {
      this.calls = await api.get<CallRecord[]>("/calls?limit=50");
    } catch {
      // History is non-critical.
    }
  }

  startCall(kind: string): void {
    if (this.call.state !== "idle" || !this.activeConversation) {
      return;
    }
    const peerId = this.peerIdOf(this.activeConversation);
    if (peerId <= 0) {
      this.notify("This account no longer exists");
      return;
    }
    this.call = {
      state: "outgoing",
      callId: 0,
      peerId,
      kind,
      muted: false,
      camOff: false,
      startedAt: 0,
      ringSince: Date.now(),
    };
    this.startCallTicker();
    socket.send("call.invite", { callee_id: peerId, kind });
  }

  acceptCall(): void {
    if (this.call.state !== "incoming" || !this.call.callId) {
      return;
    }
    socket.send("call.accept", { call_id: this.call.callId });
  }

  declineCall(): void {
    if (this.call.state !== "incoming" || !this.call.callId) {
      return;
    }
    socket.send("call.decline", { call_id: this.call.callId });
    this.resetCall();
  }

  hangup(): void {
    if (this.call.state === "idle") {
      return;
    }
    if (this.call.callId) {
      socket.send("call.end", { call_id: this.call.callId, reason: "hangup" });
    }
    this.resetCall();
  }

  private resetCall(): void {
    if (this.mgr) {
      this.mgr.stop();
      this.mgr = null;
    }
    const remote = elRef("remoteVideo") as HTMLVideoElement | null;
    if (remote) {
      remote.srcObject = null;
    }
    const local = elRef("localVideo") as HTMLVideoElement | null;
    if (local) {
      local.srcObject = null;
    }
    if (this.callTicker !== null) {
      window.clearInterval(this.callTicker);
      this.callTicker = null;
    }
    this.call = {
      state: "idle",
      callId: 0,
      peerId: 0,
      kind: "audio",
      muted: false,
      camOff: false,
      startedAt: 0,
      ringSince: 0,
    };
    void this.loadCalls();
  }

  private async beginMedia(isCaller: boolean): Promise<void> {
    const kind = this.call.kind;
    if (!this.mgr) {
      this.mgr = new CallManager({
        offer: (sdp: string) => socket.send("call.offer", { call_id: this.call.callId, sdp }),
        answer: (sdp: string) => socket.send("call.answer", { call_id: this.call.callId, sdp }),
        ice: (candidate: RTCIceCandidateInit) =>
          socket.send("call.ice", { call_id: this.call.callId, candidate }),
      });
      this.mgr.onRemoteStream = (stream: MediaStream) => {
        const video = elRef("remoteVideo") as HTMLVideoElement | null;
        if (video && video.srcObject !== stream) {
          video.srcObject = stream;
        }
      };
    }
    try {
      await this.mgr.prepare(kind);
      this.attachLocal();
      if (isCaller) {
        await this.mgr.createOffer();
      }
    } catch (err) {
      this.notify("Microphone/camera unavailable: " + messageOf(err));
      this.hangup();
    }
  }

  private attachLocal(): void {
    if (!this.mgr) {
      return;
    }
    const video = elRef("localVideo") as HTMLVideoElement | null;
    const stream = this.mgr.local;
    if (video && stream && video.srcObject !== stream) {
      video.srcObject = stream;
    }
  }

  toggleMute(): void {
    this.call.muted = !this.call.muted;
    if (this.mgr) {
      this.mgr.setMuted(this.call.muted);
    }
  }

  toggleCam(): void {
    this.call.camOff = !this.call.camOff;
    if (this.mgr) {
      this.mgr.setCameraOff(this.call.camOff);
    }
  }

  private startCallTicker(): void {
    if (this.callTicker !== null) {
      return;
    }
    this.callTicker = window.setInterval(() => {
      this.callNow = Date.now();
    }, 1000);
  }

  redial(record: CallRecord): void {
    void this.startChat(record.peer_id).then(() => this.startCall("audio"));
  }

  callStatusText(): string {
    if (this.call.state === "incoming") {
      return (this.call.kind === "video" ? "Video" : "Voice") + " call incoming...";
    }
    if (this.call.state === "outgoing") {
      return "Ringing...";
    }
    if (this.call.state === "active") {
      const elapsed = this.call.startedAt ? (this.callNow - this.call.startedAt) / 1000 : 0;
      return formatDuration(elapsed);
    }
    return "";
  }

  // ---- misc ----

  async loadMe(): Promise<void> {
    this.me = await api.get<User>("/auth/me");
  }

  logout(): void {
    const refresh = localStorage.getItem("refresh_token");
    socket.close();
    if (refresh) {
      void api.post("/auth/logout", { refresh_token: refresh }).catch(() => undefined);
    }
    api.setTokens(null, null);
    window.location.href = "/";
  }

  notify(message: string): void {
    this.toast = message;
    if (this.toastTimer !== null) {
      window.clearTimeout(this.toastTimer);
    }
    this.toastTimer = window.setTimeout(() => {
      this.toast = "";
      this.toastTimer = null;
    }, 4000);
  }

  // ---- display helpers ----

  nameOf(userId: number): string {
    if (userId <= 0) {
      return "Unknown user";
    }
    if (this.me && this.me.id === userId) {
      return this.me.name;
    }
    const found = this.users.find((u) => u.id === userId);
    if (found) {
      return found.name;
    }
    const conv = this.conversations.find((c) => this.peerIdOf(c) === userId);
    if (conv && conv.peer) {
      return conv.peer.name;
    }
    return "User #" + userId;
  }

  peerIdOf(conv: Conversation | null): number {
    if (!conv) {
      return 0;
    }
    if (conv.peer) {
      return conv.peer.id;
    }
    if (conv.type === "direct") {
      return 0;
    }
    return 0;
  }

  isOnline(userId: number): boolean {
    return this.onlineIds.includes(userId);
  }

  initials(name: string): string {
    return initialsOf(name);
  }

  avatarColor(id: number): string {
    return avatarColor(id);
  }

  time(iso: string): string {
    return clockTime(iso);
  }

  duration(seconds: number): string {
    return formatDuration(seconds);
  }

  preview(conv: Conversation): string {
    if (this.typingMap[String(conv.id)]) {
      return "Typing...";
    }
    if (!conv.last_message) {
      return "No messages yet";
    }
    const mine = this.me !== null && conv.last_message.sender_id === this.meId;
    const body = conv.last_message.body.replace(/\s+/g, " ");
    return (mine ? "You: " : "") + body;
  }

  headerSubtitle(): string {
    const conv = this.activeConversation;
    if (!conv) {
      return "";
    }
    if (this.peerTyping) {
      return "typing...";
    }
    const peerId = this.peerIdOf(conv);
    return this.isOnline(peerId) ? "online" : "offline";
  }

  receiptText(m: Message): string {
    if (m.sender_id !== this.meId || this.activeId === null) {
      return "";
    }
    const read = this.peerReads[String(this.activeId)] ?? 0;
    if (read >= m.id) {
      return "\u2713\u2713";
    }
    return this.isOnline(this.peerIdOf(this.activeConversation)) ? "\u2713\u2713" : "\u2713";
  }

  receiptClass(m: Message): string {
    if (m.sender_id !== this.meId || this.activeId === null) {
      return "";
    }
    const read = this.peerReads[String(this.activeId)] ?? 0;
    return read >= m.id ? "text-wa-accent" : "text-wa-muted";
  }
}

document.addEventListener("alpine:init", () => {
  const alpineRef = (window as any).Alpine;
  alpineRef.data("loginForm", () => new LoginForm());
  alpineRef.data("chatApp", () => new ChatApp());
});
