export class ChatRoleplayMutationGate {
  private tail: Promise<void> = Promise.resolve();

  async run<T>(
    isCurrentChannel: () => boolean,
    operation: () => Promise<T>,
  ): Promise<T> {
    const predecessor = this.tail;
    let release!: () => void;
    this.tail = new Promise<void>((resolve) => { release = resolve; });
    await predecessor;
    try {
      if (!isCurrentChannel()) {
        throw new Error("The selected world changed before the roleplay update could start.");
      }
      return await operation();
    } finally {
      release();
    }
  }
}
