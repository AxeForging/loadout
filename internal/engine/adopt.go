package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AxeForging/loadout/internal/adapter"
	"github.com/AxeForging/loadout/internal/domain"
)

// ErrNotReversible is returned by PlanAdopt when an agent's rendering of an
// artifact cannot be mapped back to store content byte-for-byte (e.g. a
// command rendered as TOML or as a generated skill).
var ErrNotReversible = errors.New("agent format is not reversible")

// AdoptPlan is the reverse of a sync for one artifact on one agent: the
// store content rebuilt from what is on disk in that agent's home.
type AdoptPlan struct {
	Artifact domain.Artifact
	Agent    domain.AgentID
	// Store is the artifact's current store content; Adopted is the
	// candidate rebuilt from disk. Keys are store content file keys.
	Store   domain.Content
	Adopted domain.Content
	// Changed lists the content keys whose adopted bytes differ, sorted.
	Changed []string
	// Ignored lists files found on disk inside the artifact's directory that
	// no store file renders to; adopt never adds them (spec 010 R9).
	Ignored []string
}

// PlanAdopt rebuilds an artifact's store content from its installed copy in
// one agent's home. It reads only; nothing is written. The mapping is
// verified by re-rendering the candidate and requiring a byte-exact match
// with disk, so adopt can never store something that would sync back
// differently.
func (e *Engine) PlanAdopt(ctx context.Context, artifactID string, agent domain.AgentID) (*AdoptPlan, error) {
	art, ok := e.Store.Artifact(artifactID)
	if !ok {
		return nil, fmt.Errorf("unknown artifact %q", artifactID)
	}
	ad, ok := e.Registry.Get(agent)
	if !ok {
		return nil, fmt.Errorf("unknown agent %q", agent)
	}
	if !ad.Supports(art.Kind) {
		return nil, fmt.Errorf("%s does not support %s artifacts", agent, art.Kind)
	}
	det, err := ad.Detect(ctx)
	if err != nil {
		return nil, fmt.Errorf("detect %s: %w", agent, err)
	}
	if !det.Installed {
		return nil, fmt.Errorf("%s is not installed (home directory missing)", agent)
	}
	content, err := e.Store.Content(art)
	if err != nil {
		return nil, err
	}

	notReversible := func(format string, a ...any) error {
		return fmt.Errorf("%w: %s on %s: %s — copy the edits into the store by hand ('loadout diff %s --agent %s' shows them)",
			ErrNotReversible, art.ID, agent, fmt.Sprintf(format, a...), art.ID, agent)
	}

	ap := &AdoptPlan{Artifact: art, Agent: agent, Store: content, Adopted: domain.Content{Files: map[string][]byte{}}}
	mapped := map[string]bool{}
	for key, raw := range content.Files {
		// Render each store file alone to learn which target it becomes.
		targets, err := ad.Render(art, domain.Content{Files: map[string][]byte{key: raw}})
		if err != nil || len(targets) != 1 {
			return nil, notReversible("store file %q does not render to exactly one target", key)
		}
		tf := targets[0]
		// Only verbatim renderings are reversible: the target must carry the
		// store bytes unchanged (whole file, or wrapped as a managed block).
		// An adapter that rewrites the file (a command rendered as a skill,
		// TOML, …) would make adopt store the agent's format, not ours.
		if !rendersVerbatim(tf, raw) {
			return nil, notReversible("%s renders %q in its own format", agent, key)
		}
		abs, err := e.targetPath(tf, det.Home)
		if err != nil {
			return nil, err
		}
		mapped[abs] = true
		disk, err := os.ReadFile(abs)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("%s is not installed on %s (%s missing) — nothing to adopt", art.ID, agent, tf.RelPath)
			}
			return nil, err
		}
		switch tf.Merge {
		case adapter.MergeReplace:
			ap.Adopted.Files[key] = disk
		case adapter.MergeBlock:
			body, err := blockBody(tf.Content, disk)
			if err != nil {
				return nil, fmt.Errorf("%s on %s: %w", art.ID, agent, err)
			}
			if bytes.HasSuffix(raw, []byte("\n")) {
				body = append(body, '\n')
			}
			ap.Adopted.Files[key] = body
		default:
			return nil, notReversible("%s targets merge into shared settings and cannot be adopted", art.Kind)
		}
	}

	// Verify: the candidate must render back to exactly what is on disk.
	rendered, err := ad.Render(art, ap.Adopted)
	if err != nil {
		return nil, notReversible("adopted content does not render: %v", err)
	}
	for _, tf := range rendered {
		abs, err := e.targetPath(tf, det.Home)
		if err != nil {
			return nil, err
		}
		disk, _ := os.ReadFile(abs)
		if !renderMatchesDisk(tf, disk) {
			return nil, notReversible("re-rendering the on-disk version does not reproduce %s", tf.RelPath)
		}
	}

	for key, b := range ap.Adopted.Files {
		if !bytes.Equal(b, content.Files[key]) {
			ap.Changed = append(ap.Changed, key)
		}
	}
	sort.Strings(ap.Changed)
	ap.Ignored = unmappedFiles(art.ID, mapped)
	return ap, nil
}

// ApplyAdopt writes an AdoptPlan's changed files into the store, after a
// confirmation and a store snapshot (trigger "adopt", restorable with
// `loadout restore`), then records the artifact as installed on the agent
// so a later store edit reads as drifted-store, not conflict. It never
// writes into agent homes.
func (e *Engine) ApplyAdopt(ctx context.Context, ap *AdoptPlan, confirm func(*AdoptPlan) bool) (int, error) {
	if len(ap.Changed) == 0 {
		return 0, e.recordInstalled(ctx, ap.Artifact.ID, ap.Agent)
	}
	if confirm == nil || !confirm(ap) {
		return 0, ErrNotConfirmed
	}
	if e.BackupDir != "" {
		var rels []string
		for _, key := range ap.Changed {
			rels = append(rels, e.Store.ContentRelPath(ap.Artifact, key))
		}
		if _, err := SnapshotStorePaths(e.BackupDir, "adopt", e.Store.Root, rels, e.KeepBackups, e.now()); err != nil {
			return 0, fmt.Errorf("snapshot before adopt: %w", err)
		}
	}
	files := map[string][]byte{}
	for _, key := range ap.Changed {
		files[key] = ap.Adopted.Files[key]
	}
	if err := e.Store.WriteContent(ap.Artifact, files); err != nil {
		return 0, err
	}
	return len(ap.Changed), e.recordInstalled(ctx, ap.Artifact.ID, ap.Agent)
}

// recordInstalled refreshes the install record of one artifact on one agent
// from a fresh plan, provided every target is now in sync.
func (e *Engine) recordInstalled(ctx context.Context, artifactID string, agent domain.AgentID) error {
	p, err := e.Plan(ctx, Filter{Agents: []domain.AgentID{agent}, Artifacts: []string{artifactID}})
	if err != nil {
		return err
	}
	for _, it := range p.Items {
		if it.Op != OpNoop {
			return fmt.Errorf("%s on %s changed during adopt (%s) — re-run adopt", artifactID, agent, it.RelPath)
		}
	}
	st, err := loadState(e.StatePath)
	if err != nil {
		return err
	}
	for key, files := range groupFragments(p.Items) {
		st.setGroup(key.artifact, key.agent, files)
	}
	return st.save(e.StatePath)
}

// targetPath resolves a rendered target to its absolute path.
func (e *Engine) targetPath(tf adapter.TargetFile, home string) (string, error) {
	base, err := e.resolveBase(tf.Root, home)
	if err != nil {
		return "", err
	}
	return joinUnder(base, tf.RelPath)
}

// blockBody returns the inner body of the managed block a fragment
// addresses, without markers or trailing newlines.
func blockBody(fragment, disk []byte) ([]byte, error) {
	id, err := adapter.BlockID(fragment)
	if err != nil {
		return nil, err
	}
	block := adapter.ExtractBlock(disk, id)
	if block == nil {
		return nil, fmt.Errorf("managed block %q not found on disk — nothing to adopt", id)
	}
	inner := block[bytes.IndexByte(block, '\n')+1:]
	inner = inner[:bytes.LastIndex(inner, []byte("<!-- loadout:end"))]
	return append([]byte{}, bytes.TrimRight(inner, "\n")...), nil
}

func rendersVerbatim(tf adapter.TargetFile, raw []byte) bool {
	switch tf.Merge {
	case adapter.MergeReplace:
		return bytes.Equal(tf.Content, raw)
	case adapter.MergeBlock:
		body, err := blockBody(tf.Content, tf.Content)
		return err == nil && bytes.Equal(body, bytes.TrimRight(raw, "\n"))
	}
	return false
}

func renderMatchesDisk(tf adapter.TargetFile, disk []byte) bool {
	switch tf.Merge {
	case adapter.MergeReplace:
		return bytes.Equal(tf.Content, disk)
	case adapter.MergeBlock:
		id, err := adapter.BlockID(tf.Content)
		if err != nil {
			return false
		}
		return bytes.Equal(bytes.TrimRight(tf.Content, "\n"), bytes.TrimRight(adapter.ExtractBlock(disk, id), "\n"))
	}
	return false
}

// unmappedFiles lists files on disk inside a directory artifact's install
// folder (the deepest directory shared by all mapped targets, named after the
// artifact) that no store file maps to.
func unmappedFiles(artifactID string, mapped map[string]bool) []string {
	if len(mapped) < 1 {
		return nil
	}
	var dir string
	for abs := range mapped {
		d := filepath.Dir(abs)
		if dir == "" {
			dir = d
			continue
		}
		for !strings.HasPrefix(d+string(filepath.Separator), dir+string(filepath.Separator)) {
			dir = filepath.Dir(dir)
		}
	}
	if filepath.Base(dir) != artifactID {
		return nil // a shared folder (commands/, the agent home) — not ours to scan
	}
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || mapped[p] {
			return nil
		}
		if rel, err := filepath.Rel(dir, p); err == nil {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out
}
