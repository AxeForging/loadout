// Package web serves the embedded loadout SPA and its JSON API. The API
// mirrors the CLI's promises: reads are free, every write is two-phase —
// POST /api/sync/plan returns a token, POST /api/sync/apply consumes it. The
// server binds to localhost only; it is a local companion, not a service.
package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AxeForging/loadout/internal/bundle"
	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/engine"
	"github.com/AxeForging/loadout/internal/logging"
	"github.com/AxeForging/loadout/internal/source"
	"github.com/AxeForging/loadout/internal/wizard"
)

type Options struct {
	Addr    string
	Engine  *engine.Engine
	Sources *source.Manager
	// Dev enables permissive CORS for the Vite dev server.
	Dev bool
}

type Server struct {
	opts Options
	mux  *http.ServeMux

	mu          sync.Mutex
	plans       map[string]pendingPlan
	bundlePlans map[string]pendingBundlePlan
}

type pendingPlan struct {
	plan    *engine.Plan
	created time.Time
}

type pendingBundlePlan struct {
	plan     *bundle.ImportPlan
	tempFile string
	created  time.Time
}

func (p pendingBundlePlan) discard() {
	p.plan.Close()
	_ = os.Remove(p.tempFile)
}

func New(opts Options) *Server {
	if opts.Addr == "" {
		opts.Addr = "127.0.0.1:4600"
	}
	s := &Server{opts: opts, mux: http.NewServeMux(), plans: map[string]pendingPlan{}, bundlePlans: map[string]pendingBundlePlan{}}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/overview", s.handleOverview)
	s.mux.HandleFunc("GET /api/status", s.handleStatus)
	s.mux.HandleFunc("GET /api/artifacts", s.handleArtifacts)
	s.mux.HandleFunc("GET /api/artifacts/{id}", s.handleArtifact)
	s.mux.HandleFunc("GET /api/diff", s.handleDiff)
	s.mux.HandleFunc("POST /api/sync/plan", s.handleSyncPlan)
	s.mux.HandleFunc("POST /api/sync/apply", s.handleSyncApply)
	s.mux.HandleFunc("GET /api/sources", s.handleSources)
	s.mux.HandleFunc("POST /api/sources/add", s.handleSourcesAdd)
	s.mux.HandleFunc("POST /api/sources/check", s.handleSourcesCheck)
	s.mux.HandleFunc("GET /api/wizard/schema", s.handleWizardSchema)
	s.mux.HandleFunc("GET /api/wizard/presets", s.handleWizardPresets)
	s.mux.HandleFunc("GET /api/profiles", s.handleProfiles)
	s.mux.HandleFunc("GET /api/profiles/{name}", s.handleProfileGet)
	s.mux.HandleFunc("POST /api/profiles", s.handleProfileSave)
	s.mux.HandleFunc("DELETE /api/profiles/{name}", s.handleProfileDelete)
	s.mux.HandleFunc("GET /api/library/instructions", s.handleLibraryList)
	s.mux.HandleFunc("POST /api/library/instructions", s.handleLibraryAdd)
	s.mux.HandleFunc("DELETE /api/library/instructions/{id}", s.handleLibraryDelete)
	s.mux.HandleFunc("GET /api/library/groups", s.handleLibraryGroupsList)
	s.mux.HandleFunc("POST /api/library/groups", s.handleLibraryGroupSave)
	s.mux.HandleFunc("DELETE /api/library/groups/{name}", s.handleLibraryGroupDelete)
	s.mux.HandleFunc("GET /api/mcp", s.handleMCPList)
	s.mux.HandleFunc("POST /api/mcp", s.handleMCPCreate)
	s.mux.HandleFunc("POST /api/wizard/plan", s.handleWizardPlan)
	s.mux.HandleFunc("POST /api/wizard/generate", s.handleWizardGenerate)
	s.mux.HandleFunc("GET /api/bundle/export", s.handleBundleExport)
	s.mux.HandleFunc("POST /api/bundle/export", s.handleBundleExportSelected)
	s.mux.HandleFunc("POST /api/bundle/export/flat", s.handleBundleExportFlat)
	s.mux.HandleFunc("POST /api/bundle/import/plan", s.handleBundleImportPlan)
	s.mux.HandleFunc("POST /api/bundle/import/apply", s.handleBundleImportApply)
	s.mux.Handle("/", spaHandler())
}

// ListenAndServe blocks until ctx is done.
func (s *Server) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{Addr: s.opts.Addr, Handler: s.withCORS(s.mux), ReadHeaderTimeout: 5 * time.Second}
	ln, err := net.Listen("tcp", s.opts.Addr)
	if err != nil {
		return err
	}
	logging.Log.Info().Msgf("loadout web UI on http://%s", s.opts.Addr)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Handler exposes the routed handler for tests.
func (s *Server) Handler() http.Handler { return s.withCORS(s.mux) }

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.opts.Dev {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// ---- read endpoints ----

type agentJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Home      string `json:"home"`
}

func (s *Server) agents(ctx context.Context) []agentJSON {
	var out []agentJSON
	for _, ad := range s.opts.Engine.Registry.All() {
		det, err := ad.Detect(ctx)
		out = append(out, agentJSON{
			ID: string(ad.ID()), Name: ad.DisplayName(),
			Installed: err == nil && det.Installed, Home: det.Home,
		})
	}
	return out
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	rows, plan, err := s.opts.Engine.Status(r.Context(), engine.Filter{})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	states := map[string]int{}
	for _, row := range rows {
		states[string(row.State)]++
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"store": map[string]any{
			"name":      s.opts.Engine.Store.Manifest.Name,
			"root":      s.opts.Engine.Store.Root,
			"artifacts": len(s.opts.Engine.Store.Manifest.Artifacts),
		},
		"agents":    s.agents(r.Context()),
		"pending":   len(plan.Changes()),
		"conflicts": len(plan.Conflicts()),
		"states":    states,
	})
}

type statusRowJSON struct {
	Artifact string `json:"artifact"`
	Kind     string `json:"kind"`
	Agent    string `json:"agent"`
	State    string `json:"state"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	rows, plan, err := s.opts.Engine.Status(r.Context(), engine.Filter{})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]statusRowJSON, 0, len(rows))
	for _, row := range rows {
		out = append(out, statusRowJSON{Artifact: row.Artifact, Kind: string(row.Kind), Agent: string(row.Agent), State: string(row.State)})
	}
	skips := make([]map[string]string, 0, len(plan.Skips))
	for _, sk := range plan.Skips {
		skips = append(skips, map[string]string{"agent": string(sk.Agent), "reason": sk.Reason})
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": out, "skips": skips})
}

func (s *Server) handleArtifacts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.opts.Engine.Store.Manifest.Artifacts)
}

func (s *Server) handleArtifact(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	art, ok := s.opts.Engine.Store.Artifact(id)
	if !ok {
		writeErr(w, http.StatusNotFound, fmt.Errorf("unknown artifact %q", id))
		return
	}
	content, err := s.opts.Engine.Store.Content(art)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	files := map[string]string{}
	for p, raw := range content.Files {
		files[p] = string(raw)
	}
	writeJSON(w, http.StatusOK, map[string]any{"artifact": art, "files": files})
}

func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	f := engine.Filter{}
	if a := r.URL.Query().Get("artifact"); a != "" {
		f.Artifacts = []string{a}
	}
	if a := r.URL.Query().Get("agent"); a != "" {
		f.Agents = []domain.AgentID{domain.AgentID(a)}
	}
	plan, err := s.opts.Engine.Plan(r.Context(), f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	type diffJSON struct {
		Agent    string `json:"agent"`
		Artifact string `json:"artifact"`
		RelPath  string `json:"relPath"`
		Diff     string `json:"diff"`
	}
	out := []diffJSON{}
	for _, it := range plan.Changes() {
		text, err := engine.UnifiedDiff(it)
		if err != nil {
			continue
		}
		out = append(out, diffJSON{Agent: string(it.Agent), Artifact: it.Artifact.ID, RelPath: it.RelPath, Diff: text})
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- two-phase sync ----

type changeJSON struct {
	Op       string `json:"op"`
	Agent    string `json:"agent"`
	Artifact string `json:"artifact"`
	RelPath  string `json:"relPath"`
}

func (s *Server) handleSyncPlan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Agents    []string `json:"agents"`
		Artifacts []string `json:"artifacts"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	f := engine.Filter{Artifacts: req.Artifacts}
	for _, a := range req.Agents {
		f.Agents = append(f.Agents, domain.AgentID(a))
	}
	plan, err := s.opts.Engine.Plan(r.Context(), f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	token := newToken()
	s.mu.Lock()
	for t, p := range s.plans { // drop stale plans
		if time.Since(p.created) > 10*time.Minute {
			delete(s.plans, t)
		}
	}
	s.plans[token] = pendingPlan{plan: plan, created: time.Now()}
	s.mu.Unlock()

	changes := make([]changeJSON, 0)
	for _, it := range plan.Changes() {
		changes = append(changes, changeJSON{Op: string(it.Op), Agent: string(it.Agent), Artifact: it.Artifact.ID, RelPath: it.RelPath})
	}
	conflicts := make([]changeJSON, 0)
	for _, it := range plan.Conflicts() {
		conflicts = append(conflicts, changeJSON{Op: "conflict", Agent: string(it.Agent), Artifact: it.Artifact.ID, RelPath: it.RelPath})
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "changes": changes, "conflicts": conflicts})
}

func (s *Server) handleSyncApply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token              string `json:"token"`
		OverwriteConflicts bool   `json:"overwriteConflicts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("missing plan token — call /api/sync/plan first"))
		return
	}
	s.mu.Lock()
	pending, ok := s.plans[req.Token]
	delete(s.plans, req.Token)
	s.mu.Unlock()
	if !ok {
		writeErr(w, http.StatusConflict, fmt.Errorf("unknown or expired plan token — re-plan and review again"))
		return
	}
	// The token round-trip IS the confirmation: the client showed the plan
	// and the user explicitly applied it. Overwriting files loadout never
	// installed is a separate, explicit opt-in the client sets after seeing
	// the conflicts.
	pending.plan.OverwriteConflicts = req.OverwriteConflicts
	n, err := s.opts.Engine.Apply(r.Context(), pending.plan, func(*engine.Plan) bool { return true })
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"applied": n})
}

func newToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---- sources ----

func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	if s.opts.Sources == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	refs, err := s.opts.Sources.Lock()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if refs == nil {
		refs = []domain.SourceRef{}
	}
	writeJSON(w, http.StatusOK, refs)
}

func (s *Server) handleSourcesAdd(w http.ResponseWriter, r *http.Request) {
	if s.opts.Sources == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("no store configured"))
		return
	}
	var req struct {
		Name   string `json:"name"`
		URL    string `json:"url"`
		Ref    string `json:"ref"`
		Subdir string `json:"subdir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.URL == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("url is required"))
		return
	}
	name := req.Name
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(req.URL), ".git")
	}
	// Cloning a dead or slow remote must not hang the request forever.
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	result, err := s.opts.Sources.Add(ctx, name, req.URL, req.Ref, req.Subdir)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if result.Added == nil {
		result.Added = []string{}
	}
	if result.Skipped == nil {
		result.Skipped = []string{}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleSourcesCheck(w http.ResponseWriter, r *http.Request) {
	if s.opts.Sources == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	results, err := s.opts.Sources.Check(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	type checkJSON struct {
		Name      string `json:"name"`
		Pinned    string `json:"pinned"`
		Remote    string `json:"remote"`
		HasUpdate bool   `json:"hasUpdate"`
		Error     string `json:"error,omitempty"`
	}
	out := make([]checkJSON, 0, len(results))
	for _, res := range results {
		c := checkJSON{Name: res.Name, Pinned: res.Pinned, Remote: res.Remote, HasUpdate: res.HasUpdate}
		if res.Err != nil {
			c.Error = res.Err.Error()
		}
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- wizard ----

func (s *Server) handleWizardSchema(w http.ResponseWriter, r *http.Request) {
	// Alongside the questionnaire, ship what the builder needs to preview
	// resolved command/skill names live: which question holds the namespace
	// prefix, and which names would shadow an agent built-in.
	writeJSON(w, http.StatusOK, map[string]any{
		"sections":                  wizard.Sections(),
		"namespacePrefixQuestionId": wizard.NamespacePrefixQuestionID,
		"reservedCommandNames":      domain.ReservedCommandNames(),
	})
}

func (s *Server) handleWizardPresets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, wizard.Presets())
}

func (s *Server) decodeAnswers(r *http.Request) (wizard.Answers, error) {
	var req struct {
		Answers wizard.Answers `json:"answers"`
		Preset  string         `json:"preset"`
		Profile string         `json:"profile"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}
	answers := wizard.Answers{}
	if req.Preset != "" {
		preset, err := wizard.LoadPreset(req.Preset)
		if err != nil {
			return nil, err
		}
		answers = answers.Merge(preset)
	}
	if req.Profile != "" {
		profile, err := wizard.LoadProfile(s.opts.Engine.Store, req.Profile)
		if err != nil {
			return nil, err
		}
		answers = answers.Merge(profile)
	}
	return answers.Merge(req.Answers).Validate()
}

func (s *Server) handleProfiles(w http.ResponseWriter, r *http.Request) {
	names, err := wizard.ListProfiles(s.opts.Engine.Store)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if names == nil {
		names = []string{}
	}
	writeJSON(w, http.StatusOK, names)
}

func (s *Server) handleProfileGet(w http.ResponseWriter, r *http.Request) {
	a, err := wizard.LoadProfile(s.opts.Engine.Store, r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) handleProfileSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string         `json:"name"`
		Answers wizard.Answers `json:"answers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}
	answers, err := req.Answers.Validate()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := wizard.SaveProfile(s.opts.Engine.Store, req.Name, answers); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": req.Name})
}

func (s *Server) handleProfileDelete(w http.ResponseWriter, r *http.Request) {
	if err := wizard.RemoveProfile(s.opts.Engine.Store, r.PathValue("name")); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"removed": true})
}

func (s *Server) handleLibraryList(w http.ResponseWriter, r *http.Request) {
	entries, err := wizard.ListLibrary(s.opts.Engine.Store)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleLibraryAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}
	entry, err := wizard.AddToLibrary(s.opts.Engine.Store, req.Text)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (s *Server) handleLibraryDelete(w http.ResponseWriter, r *http.Request) {
	if err := wizard.RemoveFromLibrary(s.opts.Engine.Store, r.PathValue("id")); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"removed": true})
}

func (s *Server) handleLibraryGroupsList(w http.ResponseWriter, r *http.Request) {
	groups, err := wizard.ListGroups(s.opts.Engine.Store)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

func (s *Server) handleLibraryGroupSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string   `json:"name"`
		EntryIDs []string `json:"entryIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}
	group, err := wizard.SaveGroup(s.opts.Engine.Store, req.Name, req.EntryIDs)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, group)
}

func (s *Server) handleLibraryGroupDelete(w http.ResponseWriter, r *http.Request) {
	if err := wizard.RemoveGroup(s.opts.Engine.Store, r.PathValue("name")); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"removed": true})
}

func (s *Server) handleWizardPlan(w http.ResponseWriter, r *http.Request) {
	answers, err := s.decodeAnswers(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	create, skip := wizard.Plan(s.opts.Engine.Store, answers)
	if create == nil {
		create = []string{}
	}
	if skip == nil {
		skip = []string{}
	}
	warnings := wizard.NameWarnings(answers)
	if warnings == nil {
		warnings = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"create": create, "skip": skip, "warnings": warnings})
}

func (s *Server) handleWizardGenerate(w http.ResponseWriter, r *http.Request) {
	answers, err := s.decodeAnswers(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	g, err := wizard.Generate(s.opts.Engine.Store, answers)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	created := make([]string, 0, len(g.Created))
	for _, a := range g.Created {
		created = append(created, a.ID)
	}
	if g.Skipped == nil {
		g.Skipped = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": created, "skipped": g.Skipped})
}

// ---- bundle ----

func (s *Server) handleBundleExport(w http.ResponseWriter, r *http.Request) {
	name := s.opts.Engine.Store.Manifest.Name
	file := strings.ReplaceAll(name, " ", "-") + ".loadout.tar.gz"
	tmp, err := bundle.ExportArchiveTemp(s.opts.Engine.Store, "loadout web")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	defer tmp.Cleanup()
	w.Header().Set("Content-Disposition", `attachment; filename="`+file+`"`)
	w.Header().Set("Content-Type", "application/gzip")
	http.ServeFile(w, r, tmp.Path)
}

func (s *Server) handleBundleExportSelected(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ArtifactIDs     []string `json:"artifactIds"`
		ProfileNames    []string `json:"profileNames"`
		LibraryEntryIDs []string `json:"libraryEntryIds"`
		Sources         bool     `json:"sources"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}
	sel := bundle.Selection{
		ArtifactIDs: req.ArtifactIDs, ProfileNames: req.ProfileNames,
		LibraryEntryIDs: req.LibraryEntryIDs, Sources: req.Sources,
	}
	name := s.opts.Engine.Store.Manifest.Name
	file := strings.ReplaceAll(name, " ", "-") + ".loadout.tar.gz"
	tmp, err := bundle.ExportSelectedArchiveTemp(s.opts.Engine.Store, "loadout web", sel)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	defer tmp.Cleanup()
	w.Header().Set("Content-Disposition", `attachment; filename="`+file+`"`)
	w.Header().Set("Content-Type", "application/gzip")
	http.ServeFile(w, r, tmp.Path)
}

// handleBundleExportFlat renders a Selection as a single self-contained
// YAML document and returns it as JSON text (not a file download) — the
// primary use case is pasting it into a gist or a doc, not always saving a
// file.
func (s *Server) handleBundleExportFlat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ArtifactIDs     []string `json:"artifactIds"`
		ProfileNames    []string `json:"profileNames"`
		LibraryEntryIDs []string `json:"libraryEntryIds"`
		Sources         bool     `json:"sources"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}
	sel := bundle.Selection{
		ArtifactIDs: req.ArtifactIDs, ProfileNames: req.ProfileNames,
		LibraryEntryIDs: req.LibraryEntryIDs, Sources: req.Sources,
	}
	raw, err := bundle.ExportFlat(s.opts.Engine.Store, "loadout web", sel)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"yaml": string(raw)})
}

// maxBundleUploadSize caps a bundle upload at 50MB, rejected before it is
// buffered in full.
const maxBundleUploadSize = 50 << 20

func (s *Server) handleBundleImportPlan(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var req struct {
			YAML string `json:"yaml"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.YAML) == "" {
			writeErr(w, http.StatusBadRequest, fmt.Errorf(`invalid request body: expected {"yaml": "..."}`))
			return
		}
		tmp, err := os.CreateTemp("", "loadout-upload-*.yaml")
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		tempFile := tmp.Name()
		if _, err := tmp.WriteString(req.YAML); err != nil {
			tmp.Close()
			_ = os.Remove(tempFile)
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		tmp.Close()
		s.planBundleImport(w, tempFile)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBundleUploadSize)
	if err := r.ParseMultipartForm(maxBundleUploadSize); err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, fmt.Errorf("upload too large or malformed (max %dMB): %w", maxBundleUploadSize>>20, err))
		return
	}
	file, _, err := r.FormFile("bundle")
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("missing bundle file: %w", err))
		return
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "loadout-upload-*.tar.gz")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	tempFile := tmp.Name()
	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		_ = os.Remove(tempFile)
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	tmp.Close()
	s.planBundleImport(w, tempFile)
}

// planBundleImport calls PlanImport on an already-materialized bundle file
// (a multipart upload or a paste-text flat YAML written to a temp file),
// stashes the plan under a token, and writes the plan JSON. Shared by both
// intake paths in handleBundleImportPlan.
func (s *Server) planBundleImport(w http.ResponseWriter, tempFile string) {
	plan, err := bundle.PlanImport(s.opts.Engine.Store, tempFile)
	if err != nil {
		_ = os.Remove(tempFile)
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	token := newToken()
	s.mu.Lock()
	for t, p := range s.bundlePlans { // drop stale plans
		if time.Since(p.created) > 10*time.Minute {
			p.discard()
			delete(s.bundlePlans, t)
		}
	}
	s.bundlePlans[token] = pendingBundlePlan{plan: plan, tempFile: tempFile, created: time.Now()}
	s.mu.Unlock()

	if plan.New == nil {
		plan.New = []domain.Artifact{}
	}
	if plan.Conflicts == nil {
		plan.Conflicts = []domain.Artifact{}
	}
	if plan.Identical == nil {
		plan.Identical = []string{}
	}
	if plan.Sources == nil {
		plan.Sources = []domain.SourceRef{}
	}
	if plan.NewProfiles == nil {
		plan.NewProfiles = []string{}
	}
	if plan.ConflictProfiles == nil {
		plan.ConflictProfiles = []string{}
	}
	if plan.NewLibraryEntries == nil {
		plan.NewLibraryEntries = []wizard.LibraryEntry{}
	}
	if plan.NewGroups == nil {
		plan.NewGroups = []string{}
	}
	if plan.ConflictGroups == nil {
		plan.ConflictGroups = []string{}
	}
	if plan.Warnings == nil {
		plan.Warnings = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":             token,
		"meta":              plan.Meta,
		"new":               plan.New,
		"conflicts":         plan.Conflicts,
		"identical":         plan.Identical,
		"sources":           plan.Sources,
		"newProfiles":       plan.NewProfiles,
		"conflictProfiles":  plan.ConflictProfiles,
		"newLibraryEntries": plan.NewLibraryEntries,
		"newGroups":         plan.NewGroups,
		"conflictGroups":    plan.ConflictGroups,
		"warnings":          plan.Warnings,
	})
}

func (s *Server) handleBundleImportApply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token     string `json:"token"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("missing plan token — call /api/bundle/import/plan first"))
		return
	}
	s.mu.Lock()
	pending, ok := s.bundlePlans[req.Token]
	delete(s.bundlePlans, req.Token)
	s.mu.Unlock()
	if !ok {
		writeErr(w, http.StatusConflict, fmt.Errorf("unknown or expired plan token — re-upload and review again"))
		return
	}
	defer pending.discard()

	sourcesAdded := len(pending.plan.Sources)
	profilesAdded := len(pending.plan.NewProfiles)
	if req.Overwrite {
		profilesAdded += len(pending.plan.ConflictProfiles)
	}
	libraryEntriesAdded := len(pending.plan.NewLibraryEntries)
	groupsAdded := len(pending.plan.NewGroups)
	if req.Overwrite {
		groupsAdded += len(pending.plan.ConflictGroups)
	}
	applied, err := bundle.Apply(s.opts.Engine.Store, pending.plan, req.Overwrite)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if applied == nil {
		applied = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"applied":             applied,
		"sourcesAdded":        sourcesAdded,
		"profilesAdded":       profilesAdded,
		"libraryEntriesAdded": libraryEntriesAdded,
		"groupsAdded":         groupsAdded,
	})
}

// spaHandler serves the embedded SPA with an index.html fallback for client
// routes.
func spaHandler() http.Handler {
	dist, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if _, err := fs.Stat(dist, path); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
