export type ModalShellOptions = {
  wide?: boolean;
};

export function getModalElements(): { modal: HTMLElement | null; panel: HTMLElement | null } {
  return {
    modal: document.querySelector('[data-chat-target="modal"]') as HTMLElement | null,
    panel: document.querySelector('[data-chat-target="modalPanel"]') as HTMLElement | null,
  };
}

export function openModalShell(options: ModalShellOptions = {}): void {
  const { modal, panel } = getModalElements();
  if (!modal || !panel) return;

  modal.classList.remove("hidden", "is-closing");
  modal.classList.add("grid");

  if (options.wide) {
    panel.classList.remove("max-w-5xl");
    panel.classList.add("max-w-6xl");
  } else {
    panel.classList.remove("max-w-6xl");
    panel.classList.add("max-w-5xl");
  }

  modal.classList.remove("is-open");
  void modal.offsetHeight;
  requestAnimationFrame(() => {
    modal.classList.add("is-open");
  });
}

export function closeModalShell(onClosed?: () => void): void {
  const { modal } = getModalElements();
  if (!modal || modal.classList.contains("hidden")) {
    onClosed?.();
    return;
  }

  let finished = false;
  const finish = () => {
    if (finished) return;
    finished = true;
    modal.classList.add("hidden");
    modal.classList.remove("grid", "is-open", "is-closing");
    document.dispatchEvent(new CustomEvent("omni:modal-closed"));
    onClosed?.();
  };

  if (!modal.classList.contains("is-open")) {
    finish();
    return;
  }

  modal.classList.add("is-closing");
  modal.classList.remove("is-open");

  const onTransitionEnd = (event: TransitionEvent) => {
    if (event.target !== modal) return;
    modal.removeEventListener("transitionend", onTransitionEnd);
    finish();
  };
  modal.addEventListener("transitionend", onTransitionEnd);
  window.setTimeout(finish, 420);
}

export function resetModalPanelWidth(): void {
  const { panel } = getModalElements();
  panel?.classList.remove("max-w-6xl");
  panel?.classList.add("max-w-5xl");
}
