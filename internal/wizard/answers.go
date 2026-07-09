package wizard

import (
	"fmt"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
)

// Answers maps question IDs to values: string for select/text, bool for
// bool, []string for multi.
type Answers map[string]any

// Str returns a string answer (or the schema default, or "").
func (a Answers) Str(id string) string {
	if v, ok := a[id]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	if d, ok := defaultFor(id).(string); ok {
		return d
	}
	return ""
}

// Bool returns a bool answer (or the schema default, or false).
func (a Answers) Bool(id string) bool {
	if v, ok := a[id]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	if d, ok := defaultFor(id).(bool); ok {
		return d
	}
	return false
}

// List returns a multi answer (or the schema default, or nil).
func (a Answers) List(id string) []string {
	if v, ok := a[id]; ok {
		switch vv := v.(type) {
		case []string:
			return vv
		case []any:
			out := make([]string, 0, len(vv))
			for _, item := range vv {
				if s, ok := item.(string); ok {
					out = append(out, s)
				}
			}
			return out
		}
	}
	if d, ok := defaultFor(id).([]string); ok {
		return d
	}
	return nil
}

// EffectiveList returns a multi answer's checked options plus any custom
// values from its "<id>.other" free-text companion (comma-separated). When
// free-text values are given, the generic "other" placeholder option is
// dropped in favor of the specific values it stands in for.
func (a Answers) EffectiveList(id string) []string {
	var extras []string
	for _, extra := range strings.Split(a.Str(id+".other"), ",") {
		extra = strings.TrimSpace(extra)
		if extra != "" {
			extras = append(extras, extra)
		}
	}
	out := make([]string, 0, len(a.List(id))+len(extras))
	for _, v := range a.List(id) {
		if v == "other" && len(extras) > 0 {
			continue
		}
		out = append(out, v)
	}
	return append(out, extras...)
}

// Has reports whether a multi answer contains value.
func (a Answers) Has(id, value string) bool {
	for _, v := range a.List(id) {
		if v == value {
			return true
		}
	}
	return false
}

func defaultFor(id string) any {
	for _, q := range Questions() {
		if q.ID == id {
			return q.Default
		}
	}
	return nil
}

// Summary renders the effective answers (including schema defaults) as a
// compact context block, e.g. for AI-enrichment prompts.
func (a Answers) Summary() string {
	var b strings.Builder
	for _, s := range Sections() {
		for _, q := range s.Questions {
			switch q.Type {
			case TypeMulti, TypeList:
				if v := a.List(q.ID); len(v) > 0 {
					fmt.Fprintf(&b, "- %s: %s\n", q.ID, strings.Join(v, ", "))
				}
			case TypeBool:
				fmt.Fprintf(&b, "- %s: %t\n", q.ID, a.Bool(q.ID))
			default:
				if v := a.Str(q.ID); v != "" {
					fmt.Fprintf(&b, "- %s: %s\n", q.ID, v)
				}
			}
		}
	}
	return b.String()
}

// LoadAnswers reads an answers YAML file (question ID → value).
func LoadAnswers(path string) (Answers, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var a Answers
	if err := yaml.Unmarshal(raw, &a); err != nil {
		return nil, fmt.Errorf("parse answers file: %w", err)
	}
	return a.validate()
}

// Merge overlays other on top of a (other wins).
func (a Answers) Merge(other Answers) Answers {
	out := Answers{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range other {
		out[k] = v
	}
	return out
}

// Validate rejects unknown question IDs and values outside the schema's
// options, so typos in answer files and API payloads fail loudly.
func (a Answers) Validate() (Answers, error) { return a.validate() }

func (a Answers) validate() (Answers, error) {
	byID := map[string]Question{}
	for _, q := range Questions() {
		byID[q.ID] = q
	}
	for id, v := range a {
		q, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("unknown question %q in answers", id)
		}
		switch q.Type {
		case TypeSelect:
			s, ok := v.(string)
			if !ok || !hasOption(q, s) {
				return nil, fmt.Errorf("%s: %v is not one of %s", id, v, optionValues(q))
			}
		case TypeMulti:
			for _, item := range (Answers{id: v}).List(id) {
				if !hasOption(q, item) {
					return nil, fmt.Errorf("%s: %q is not one of %s", id, item, optionValues(q))
				}
			}
		case TypeBool:
			if _, ok := v.(bool); !ok {
				return nil, fmt.Errorf("%s: expected true/false, got %v", id, v)
			}
		case TypeList:
			switch vv := v.(type) {
			case []string:
				// already the expected shape
			case []any:
				for _, item := range vv {
					if _, ok := item.(string); !ok {
						return nil, fmt.Errorf("%s: expected a list of strings, got %v", id, item)
					}
				}
			case string:
				// hand-edited YAML: newline-separated text coerces to a list
				a[id] = SplitLines(vv)
			default:
				return nil, fmt.Errorf("%s: expected a list of strings, got %T", id, v)
			}
		}
	}
	return a, nil
}

// SplitLines splits s on newlines, trimming and dropping blank lines. Used to
// turn a multiline text field (CLI list-question input, or hand-edited YAML)
// into an ordered []string answer.
func SplitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func hasOption(q Question, value string) bool {
	for _, o := range q.Options {
		if o.Value == value {
			return true
		}
	}
	return false
}

func optionValues(q Question) []string {
	out := make([]string, 0, len(q.Options))
	for _, o := range q.Options {
		out = append(out, o.Value)
	}
	return out
}
