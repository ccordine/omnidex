export interface ChatChannelTransitionGateHost {
  hasChannelSelect(): boolean;
  channelSelect(): HTMLSelectElement;
}

export class ChatChannelTransitionGate {
  private predecessor: Promise<void> = Promise.resolve();
  private pending = 0;
  private priorDisabled: boolean | undefined;

  constructor(private readonly host: ChatChannelTransitionGateHost) {}

  async run<T>(operation: () => Promise<T>): Promise<T> {
    if (this.pending === 0 && this.host.hasChannelSelect()) {
      this.priorDisabled = this.host.channelSelect().disabled;
    }
    this.pending += 1;
    if (this.host.hasChannelSelect()) this.host.channelSelect().disabled = true;
    const prior = this.predecessor;
    let release!: () => void;
    this.predecessor = new Promise<void>((resolve) => { release = resolve; });
    await prior;
    try {
      return await operation();
    } finally {
      release();
      this.pending -= 1;
      if (this.pending < 0) throw new Error("Channel transition accounting underflowed.");
      if (this.pending === 0) {
        const disabled = this.priorDisabled;
        this.priorDisabled = undefined;
        if (disabled !== undefined) {
          if (!this.host.hasChannelSelect()) throw new Error("The canonical channel select disappeared during a transition.");
          this.host.channelSelect().disabled = disabled;
        }
      }
    }
  }
}
