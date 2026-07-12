// One rendering for unified diffs everywhere (drift matrix, sync dock):
// additions in sync green, removals in danger red, context faint.
export function DiffView({ diff }: { diff: string }) {
  return (
    <pre className="overflow-x-auto rounded-lg border border-line bg-bg-deep/60 p-4 font-mono text-xs leading-relaxed">
      {diff.split("\n").map((line, i) => (
        <span
          key={i}
          className={
            line.startsWith("+")
              ? "block text-sync"
              : line.startsWith("-")
                ? "block text-danger"
                : "block text-ink-faint"
          }
        >
          {line || " "}
        </span>
      ))}
    </pre>
  );
}
