const sleep = (ms: number) => new Promise<void>((resolve) => window.setTimeout(resolve, ms));

export async function revealTagsProgressively(
  render: (tags: string[]) => string,
  recycle: (html: string) => void,
  finalTags: string[],
  previousTags: string[],
  delayMs = 140,
): Promise<void> {
  const seen = new Set(previousTags);
  const ordered = [...previousTags];
  for (const tag of finalTags) {
    if (seen.has(tag)) continue;
    seen.add(tag);
    ordered.push(tag);
    recycle(render(ordered));
    await sleep(delayMs);
  }
  recycle(render(finalTags));
}

export function appendStreamDelta(field: HTMLTextAreaElement | null, text: string) {
  if (!field) return;
  field.value += text;
  field.scrollTop = field.scrollHeight;
}

export function resetStreamField(field: HTMLTextAreaElement | null) {
  if (field) field.value = "";
}
