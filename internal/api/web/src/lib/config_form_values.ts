export function collectConfigValues(root: ParentNode, scope: string): Record<string, string> {
  const values: Record<string, string> = {};
  root.querySelectorAll(`[data-project-config='${scope}'][data-config-key]`).forEach((node) => {
    const input = node as HTMLInputElement | HTMLSelectElement;
    const key = input.dataset.configKey?.trim() ?? "";
    if (key && input.value.trim()) values[key] = input.value.trim();
  });
  return values;
}

export function clearConfigValues(root: ParentNode, scope: string): void {
  root.querySelectorAll(`[data-project-config='${scope}']`).forEach((node) => {
    if (node instanceof HTMLInputElement || node instanceof HTMLSelectElement) node.value = "";
  });
}
