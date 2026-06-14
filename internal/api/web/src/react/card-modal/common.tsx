import type { FormEvent, InputHTMLAttributes, ReactNode, SelectHTMLAttributes, TextareaHTMLAttributes } from "react";

type ButtonTone = "default" | "primary" | "danger" | "ok";

const tones: Record<ButtonTone, string> = {
  default: "border-white/10 text-zinc-200 hover:border-cyan-300/40 hover:text-cyan-100",
  primary: "border-cyan-300/40 bg-cyan-300/10 text-cyan-100 hover:bg-cyan-300/20",
  danger: "border-rose-400/30 bg-rose-400/10 text-rose-100 hover:bg-rose-400/20",
  ok: "border-emerald-400/30 bg-emerald-400/10 text-emerald-100 hover:bg-emerald-400/20",
};

export function ActionButton({
  children,
  disabled,
  onClick,
  tone = "default",
  type = "button",
}: {
  children: ReactNode;
  disabled?: boolean;
  onClick?: () => void;
  tone?: ButtonTone;
  type?: "button" | "submit";
}) {
  return (
    <button
      type={type}
      disabled={disabled}
      onClick={onClick}
      className={`inline-flex items-center justify-center rounded-md border px-3 py-1.5 text-xs font-medium transition disabled:cursor-not-allowed disabled:opacity-50 ${tones[tone]}`}
    >
      {children}
    </button>
  );
}

export function TextInput(props: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      {...props}
      className={`rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40 ${props.className ?? ""}`}
    />
  );
}

export function TextArea(props: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea
      {...props}
      className={`scrollbar rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm leading-6 text-zinc-100 outline-none focus:border-cyan-300/40 ${props.className ?? ""}`}
    />
  );
}

export function Select(props: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select
      {...props}
      className={`rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-cyan-300/40 ${props.className ?? ""}`}
    />
  );
}

export function Panel({ children, title, aside }: { children: ReactNode; title: string; aside?: ReactNode }) {
  return (
    <section className="rounded-md border border-white/10 bg-zinc-950/45 p-4">
      <div className="flex items-start justify-between gap-3">
        <h3 className="text-sm font-semibold text-zinc-100">{title}</h3>
        {aside}
      </div>
      <div className="mt-3">{children}</div>
    </section>
  );
}

export function EmptyState({ children }: { children: ReactNode }) {
  return <p className="rounded-md border border-dashed border-white/10 px-3 py-5 text-center text-xs text-zinc-500">{children}</p>;
}

export function SpinnerLabel({ label }: { label: string }) {
  return (
    <span className="inline-flex items-center gap-2 text-sm text-zinc-400" role="status" aria-live="polite">
      <span className="inline-block h-4 w-4 shrink-0 animate-spin rounded-full border-2 border-cyan-300/25 border-t-cyan-200" aria-hidden="true" />
      <span>{label}</span>
    </span>
  );
}

export function submitForm(handler: () => void) {
  return (event: FormEvent) => {
    event.preventDefault();
    handler();
  };
}

export function shortDate(value?: string) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}
