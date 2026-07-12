// Form → MCPSpec parsing for the MCP page. The textarea formats mirror what
// people already paste around: shell-ish args, KEY=${VAR} env lines and
// "Header: value" lines — one entry per line, blanks ignored.

/** Split an args line on whitespace, honoring double/single quotes for values with spaces. */
export function parseArgs(raw: string): string[] {
  const out: string[] = [];
  const re = /"([^"]*)"|'([^']*)'|(\S+)/g;
  for (const m of raw.matchAll(re)) {
    out.push(m[1] ?? m[2] ?? m[3]);
  }
  return out;
}

/** Parse KEY=value lines (first "=" splits; later "=" stay in the value). */
export function parseEnvLines(raw: string): Record<string, string> {
  return parseLines(raw, "=");
}

/** Parse "Header: value" lines (first ":" splits). */
export function parseHeaderLines(raw: string): Record<string, string> {
  return parseLines(raw, ":");
}

function parseLines(raw: string, sep: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of raw.split("\n")) {
    const trimmed = line.trim();
    if (trimmed === "") continue;
    const i = trimmed.indexOf(sep);
    if (i <= 0) continue; // no separator or empty key — skip, backend drops empties anyway
    const key = trimmed.slice(0, i).trim();
    const value = trimmed.slice(i + 1).trim();
    if (key !== "" && value !== "") out[key] = value;
  }
  return out;
}
