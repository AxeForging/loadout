// Mirrors internal/wizard's name resolution (customName + namespacePrefix in
// content.go) so the builder can preview the exact install name live:
// a trimmed, valid rename wins verbatim; anything else falls back to the
// default, and the namespace prefix applies only to names left at default.

// Same shape store.ValidateName accepts: lowercase letters, digits, dashes;
// no leading/trailing dash.
const VALID_NAME = /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/;

export function isValidName(v: string): boolean {
  return VALID_NAME.test(v);
}

function normalized(raw: unknown): string {
  const v = typeof raw === "string" ? raw.trim() : "";
  return v !== "" && isValidName(v) ? v : "";
}

/** The artifact/slash-command name the wizard will actually generate. */
export function resolveGeneratedName(raw: unknown, def: string, prefixRaw: unknown): string {
  const v = normalized(raw) || def;
  const prefix = normalized(prefixRaw);
  if (prefix !== "" && v === def) return `${prefix}-${def}`;
  return v;
}

/** True when the typed value is non-empty but would be rejected (and thus silently fall back to the default). */
export function fallsBackToDefault(raw: unknown): boolean {
  const v = typeof raw === "string" ? raw.trim() : "";
  return v !== "" && !isValidName(v);
}
