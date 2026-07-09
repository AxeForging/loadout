package wizard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/store"
)

// Generated summarizes a wizard run.
type Generated struct {
	Created []domain.Artifact
	Skipped []string // IDs that already existed in the store
}

// Plan lists the artifact IDs a Generate call would create for these
// answers, without writing anything.
func Plan(s *store.Store, a Answers) (create, skip []string) {
	for _, spec := range artifactSpecs(a) {
		if _, exists := s.Artifact(spec.id); exists {
			skip = append(skip, spec.id)
		} else {
			create = append(create, spec.id)
		}
	}
	return create, skip
}

// Generate writes the artifacts derived from the answers into the store.
// Existing artifact IDs are skipped, never overwritten.
func Generate(s *store.Store, a Answers) (Generated, error) {
	var g Generated
	targets := targetsFrom(a)
	for _, spec := range artifactSpecs(a) {
		if _, exists := s.Artifact(spec.id); exists {
			g.Skipped = append(g.Skipped, spec.id)
			continue
		}
		art, err := writeArtifact(s, spec, targets)
		if err != nil {
			return g, err
		}
		g.Created = append(g.Created, art)
	}
	return g, nil
}

func targetsFrom(a Answers) []string {
	agents := a.List("targets.agents")
	for _, t := range agents {
		if t == domain.TargetAll {
			return nil // manifest default: all
		}
	}
	return agents
}

// artifactSpec is one artifact the wizard emits: an ID, a kind, and its
// files (path within the artifact → content; empty path = single file).
type artifactSpec struct {
	id    string
	kind  domain.ArtifactKind
	files map[string]string
}

func artifactSpecs(a Answers) []artifactSpec {
	// Command/skill names default to today's fixed IDs, but a team can
	// rename any of them (e.g. /sdd-workflow -> /fe-sdd-workflow) — the
	// custom name becomes both the store artifact ID and, for commands,
	// the synced slash-command name.
	handoffID := customName(a, "continuity.handoffCommandName", "handoff")
	resumeID := customName(a, "continuity.resumeCommandName", "resume")
	specID := customName(a, "sdd.specCommandName", "spec")
	planID := customName(a, "sdd.planCommandName", "plan")
	workflowID := customName(a, "sdd.workflowSkillName", "sdd-workflow")

	specs := []artifactSpec{
		{id: "core", kind: domain.KindInstruction, files: map[string]string{"": coreInstruction(a)}},
		{id: "toolchain", kind: domain.KindInstruction, files: map[string]string{"": toolchainInstruction(a)}},
		{id: "conventions", kind: domain.KindInstruction, files: map[string]string{"": conventionsInstruction(a)}},
	}
	if a.Str("conv.commits") != "freeform" || a.Str("conv.prTitles") != "freeform" {
		files := map[string]string{"SKILL.md": commitAndPRSkill(a)}
		if a.Bool("conv.lefthook") {
			files["references/lefthook.yml"] = lefthookSnippet(a)
		}
		specs = append(specs, artifactSpec{id: "commit-and-pr", kind: domain.KindSkill, files: files})
	}
	if len(a.List("conv.linters")) > 0 {
		specs = append(specs, artifactSpec{id: "linting", kind: domain.KindSkill, files: map[string]string{"SKILL.md": lintingSkill(a)}})
	}
	if a.Bool("toolchain.docker") {
		specs = append(specs, artifactSpec{id: "docker-workflows", kind: domain.KindSkill, files: map[string]string{"SKILL.md": dockerSkill(a)}})
	}
	if len(a.List("quality.levels")) > 0 {
		specs = append(specs, artifactSpec{id: "testing", kind: domain.KindInstruction, files: map[string]string{"": testingInstruction(a)}})
	}
	if custom := customInstruction(a); custom != "" {
		specs = append(specs, artifactSpec{id: "custom-instructions", kind: domain.KindInstruction, files: map[string]string{"": custom}})
	}
	if a.Bool("continuity.handoff") {
		specs = append(specs,
			artifactSpec{id: handoffID, kind: domain.KindCommand, files: map[string]string{"": handoffCommand()}},
			artifactSpec{id: resumeID, kind: domain.KindCommand, files: map[string]string{"": resumeCommand(handoffID)}},
			artifactSpec{id: "context-handoff", kind: domain.KindSkill, files: map[string]string{"SKILL.md": contextHandoffSkill(handoffID, resumeID)}},
		)
	}
	if a.Bool("sdd.enable") {
		specs = append(specs,
			artifactSpec{id: "sdd", kind: domain.KindInstruction, files: map[string]string{"": sddInstruction(a, specID, planID)}},
			artifactSpec{id: planID, kind: domain.KindCommand, files: map[string]string{"": planCommand(a)}},
			artifactSpec{id: specID, kind: domain.KindCommand, files: map[string]string{"": specCommand(a)}},
			artifactSpec{id: workflowID, kind: domain.KindSkill, files: map[string]string{"SKILL.md": sddWorkflowSkill(a, workflowID, specID, planID, handoffID, resumeID)}},
		)
	}
	return specs
}

var kindDirs = map[domain.ArtifactKind]string{
	domain.KindInstruction: "instructions",
	domain.KindCommand:     "commands",
	domain.KindSkill:       "skills",
}

func writeArtifact(s *store.Store, spec artifactSpec, targets []string) (domain.Artifact, error) {
	dir := kindDirs[spec.kind]
	var relPath string
	switch spec.kind {
	case domain.KindInstruction, domain.KindCommand:
		relPath = filepath.Join(dir, spec.id+".md")
		abs := filepath.Join(s.Root, relPath)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return domain.Artifact{}, err
		}
		if err := os.WriteFile(abs, []byte(spec.files[""]), 0o644); err != nil {
			return domain.Artifact{}, err
		}
	case domain.KindSkill:
		relPath = filepath.Join(dir, spec.id)
		for rel, content := range spec.files {
			abs := filepath.Join(s.Root, relPath, rel)
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				return domain.Artifact{}, err
			}
			mode := os.FileMode(0o644)
			if strings.HasSuffix(rel, ".sh") {
				mode = 0o755
			}
			if err := os.WriteFile(abs, []byte(content), mode); err != nil {
				return domain.Artifact{}, err
			}
		}
	default:
		return domain.Artifact{}, fmt.Errorf("wizard cannot emit kind %q", spec.kind)
	}

	art := domain.Artifact{
		ID:      spec.id,
		Kind:    spec.kind,
		Path:    filepath.ToSlash(relPath),
		Origin:  "local",
		Targets: targets,
		Tags:    []string{"wizard"},
	}
	if err := s.AddArtifact(art); err != nil {
		return domain.Artifact{}, err
	}
	return art, nil
}
