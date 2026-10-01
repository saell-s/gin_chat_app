// WebRTC peer connection wrapper for audio/video calls.

interface SignalChannel {
  offer(sdp: string): void;
  answer(sdp: string): void;
  ice(candidate: RTCIceCandidateInit): void;
}

class CallManager {
  private pc: RTCPeerConnection | null = null;
  private localStream: MediaStream | null = null;
  private remoteStream: MediaStream | null = null;
  private signals: SignalChannel;
  private kind = "audio";

  onRemoteStream: (stream: MediaStream) => void = () => undefined;

  constructor(signals: SignalChannel) {
    this.signals = signals;
  }

  get local(): MediaStream | null {
    return this.localStream;
  }

  get remote(): MediaStream | null {
    return this.remoteStream;
  }

  // prepare captures the user's media for the given call kind.
  async prepare(kind: string): Promise<void> {
    if (this.localStream && this.kind === kind) {
      return;
    }
    this.stopTracks();
    this.kind = kind;
    const constraints: MediaStreamConstraints = {
      audio: true,
      video: kind === "video" ? { width: { ideal: 640 }, height: { ideal: 480 } } : false,
    };
    const stream = await navigator.mediaDevices.getUserMedia(constraints);
    this.localStream = stream;
    if (this.pc) {
      for (const track of stream.getTracks()) {
        this.pc.addTrack(track, stream);
      }
    }
  }

  private ensurePeer(): RTCPeerConnection {
    if (this.pc) {
      return this.pc;
    }
    const pc = new RTCPeerConnection({
      iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
    });
    this.remoteStream = new MediaStream();

    pc.onicecandidate = (ev: RTCPeerConnectionIceEvent) => {
      if (ev.candidate && ev.candidate.toJSON) {
        this.signals.ice(ev.candidate.toJSON());
      }
    };
    pc.ontrack = (ev: RTCTrackEvent) => {
      this.remoteStream = ev.streams[0] ?? this.remoteStream;
      if (!this.remoteStream.getTracks().includes(ev.track)) {
        this.remoteStream.addTrack(ev.track);
      }
      this.onRemoteStream(this.remoteStream);
    };
    pc.onconnectionstatechange = () => {
      if (pc.connectionState === "failed") {
        this.stop();
      }
    };

    this.pc = pc;
    if (this.localStream) {
      for (const track of this.localStream.getTracks()) {
        pc.addTrack(track, this.localStream);
      }
    }
    return pc;
  }

  // createOffer is called by the caller once the callee accepts.
  async createOffer(): Promise<void> {
    const pc = this.ensurePeer();
    const offer = await pc.createOffer();
    await pc.setLocalDescription(offer);
    this.signals.offer(JSON.stringify(pc.localDescription));
  }

  // handleOffer answers an incoming offer as the callee.
  async handleOffer(sdp: string): Promise<void> {
    const pc = this.ensurePeer();
    const offer = new RTCSessionDescription(JSON.parse(sdp));
    await pc.setRemoteDescription(offer);
    const answer = await pc.createAnswer();
    await pc.setLocalDescription(answer);
    this.signals.answer(JSON.stringify(pc.localDescription));
  }

  // handleAnswer completes the caller's handshake.
  async handleAnswer(sdp: string): Promise<void> {
    if (!this.pc) {
      return;
    }
    const answer = new RTCSessionDescription(JSON.parse(sdp));
    await this.pc.setRemoteDescription(answer);
  }

  async addIce(raw: unknown): Promise<void> {
    if (!this.pc || !raw) {
      return;
    }
    try {
      const candidate = typeof raw === "string" ? JSON.parse(raw) : raw;
      await this.pc.addIceCandidate(candidate as RTCIceCandidateInit);
    } catch {
      // Candidates may arrive before the remote description; safe to skip.
    }
  }

  setMuted(muted: boolean): void {
    if (!this.localStream) {
      return;
    }
    for (const track of this.localStream.getAudioTracks()) {
      track.enabled = !muted;
    }
  }

  setCameraOff(off: boolean): void {
    if (!this.localStream) {
      return;
    }
    for (const track of this.localStream.getVideoTracks()) {
      track.enabled = !off;
    }
  }

  private stopTracks(): void {
    if (this.localStream) {
      for (const track of this.localStream.getTracks()) {
        track.stop();
      }
      this.localStream = null;
    }
  }

  // stop tears the connection and media down.
  stop(): void {
    if (this.pc) {
      try {
        this.pc.onicecandidate = null;
        this.pc.ontrack = null;
        this.pc.close();
      } catch {
        // Already closed.
      }
      this.pc = null;
    }
    this.stopTracks();
    this.remoteStream = null;
  }
}
