import { setGlobalLoading } from "./loading";

interface WorkingSurface {
  overlay: HTMLElement;
  message: HTMLElement;
}

interface WorkingOperation {
  id: symbol;
  message: string;
}

type WorkingSurfaceProvider = () => WorkingSurface;
type GlobalLoadingSetter = (loading: boolean) => void;

export class ScrumWorkingState {
  private readonly operations: WorkingOperation[] = [];
  private active = false;

  constructor(
    private readonly surface: WorkingSurfaceProvider,
    private readonly setGlobal: GlobalLoadingSetter = setGlobalLoading,
  ) {}

  start(message: string): () => void {
    const normalizedMessage = message.trim();
    if (!normalizedMessage) throw new Error("Scrum working state requires a visible status message.");

    const operation: WorkingOperation = { id: Symbol(normalizedMessage), message: normalizedMessage };
    this.operations.push(operation);
    this.render();

    let completed = false;
    return () => {
      if (completed) throw new Error(`Scrum operation ${JSON.stringify(normalizedMessage)} already completed.`);
      completed = true;
      const index = this.operations.findIndex(({ id }) => id === operation.id);
      if (index < 0) throw new Error(`Scrum operation ${JSON.stringify(normalizedMessage)} is not active.`);
      this.operations.splice(index, 1);
      this.render();
    };
  }

  private render(): void {
    const { overlay, message } = this.surface();
    const active = this.operations.length > 0;
    if (active !== this.active) {
      this.active = active;
      this.setGlobal(active);
    }
    overlay.classList.toggle("hidden", !active);
    overlay.classList.toggle("flex", active);
    overlay.setAttribute("aria-hidden", active ? "false" : "true");
    const current = this.operations.at(-1);
    if (current) message.textContent = current.message;
  }
}
