package domain

// Reserved-name knowledge lives in domain because every layer that needs it
// (wizard, store, bundle, cli, web) already imports domain, and the import
// rules forbid those packages depending on adapter — where per-agent command
// knowledge would otherwise naturally sit. These are WARNINGS only: a user may
// deliberately shadow a built-in, so nothing here ever blocks an operation.

// reservedCommandNames are slash commands Claude Code ships built in. A store
// artifact rendered to one of these (a command, or a skill — skills are also
// slash-invocable) shadows the agent's own command, which is rarely intended.
// Curated from Claude Code's built-in command set; extend as agents are added.
var reservedCommandNames = map[string]bool{
	"add-dir":         true,
	"agents":          true,
	"bashes":          true,
	"bug":             true,
	"clear":           true,
	"compact":         true,
	"config":          true,
	"context":         true,
	"cost":            true,
	"doctor":          true,
	"export":          true,
	"help":            true,
	"hooks":           true,
	"ide":             true,
	"init":            true,
	"login":           true,
	"logout":          true,
	"mcp":             true,
	"memory":          true,
	"model":           true,
	"output-style":    true,
	"permissions":     true,
	"plan":            true,
	"pr-comments":     true,
	"release-notes":   true,
	"resume":          true,
	"review":          true,
	"rewind":          true,
	"security-review": true,
	"status":          true,
	"statusline":      true,
	"terminal-setup":  true,
	"todos":           true,
	"upgrade":         true,
	"vim":             true,
}

// ReservedNameWarning returns a human-readable warning if name would collide
// with a supported agent's built-in for the given kind, or "" if it is safe.
// Only command-shaped artifacts (commands and skills) can collide; other kinds
// return "".
func ReservedNameWarning(kind ArtifactKind, name string) string {
	switch kind {
	case KindCommand, KindSkill:
		if reservedCommandNames[name] {
			return name + " collides with Claude Code's built-in /" + name +
				" — consider a namespace prefix (e.g. \"my-" + name + "\")"
		}
	}
	return ""
}
