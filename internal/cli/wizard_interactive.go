package cli

import (
	"fmt"

	"github.com/AxeForging/loadout/internal/wizard"
	"github.com/charmbracelet/huh"
)

// collectAnswers runs the questionnaire as an interactive huh form, one
// group per schema section. The schema stays the single source of truth —
// this function only maps question types onto form fields.
func collectAnswers() (wizard.Answers, error) {
	sections := wizard.Sections()

	strVals := map[string]*string{}
	listVals := map[string]*[]string{}
	boolVals := map[string]*bool{}

	var groups []*huh.Group
	for _, section := range sections {
		var fields []huh.Field
		for _, q := range section.Questions {
			switch q.Type {
			case wizard.TypeSelect:
				v := new(string)
				if d, ok := q.Default.(string); ok {
					*v = d
				}
				strVals[q.ID] = v
				fields = append(fields, huh.NewSelect[string]().
					Title(q.Prompt).
					Options(toOptions(q)...).
					Value(v))
			case wizard.TypeMulti:
				v := new([]string)
				if d, ok := q.Default.([]string); ok {
					*v = append([]string{}, d...)
				}
				listVals[q.ID] = v
				fields = append(fields, huh.NewMultiSelect[string]().
					Title(q.Prompt).
					Options(toOptions(q)...).
					Value(v))
			case wizard.TypeText:
				v := new(string)
				if d, ok := q.Default.(string); ok {
					*v = d
				}
				strVals[q.ID] = v
				fields = append(fields, huh.NewInput().
					Title(q.Prompt).
					Value(v))
			case wizard.TypeBool:
				v := new(bool)
				if d, ok := q.Default.(bool); ok {
					*v = d
				}
				boolVals[q.ID] = v
				fields = append(fields, huh.NewConfirm().
					Title(q.Prompt).
					Value(v))
			}
		}
		groups = append(groups, huh.NewGroup(fields...).Title(section.Title))
	}

	if err := huh.NewForm(groups...).Run(); err != nil {
		return nil, fmt.Errorf("wizard cancelled: %w", err)
	}

	answers := wizard.Answers{}
	for id, v := range strVals {
		answers[id] = *v
	}
	for id, v := range listVals {
		answers[id] = *v
	}
	for id, v := range boolVals {
		answers[id] = *v
	}
	return answers, nil
}

func toOptions(q wizard.Question) []huh.Option[string] {
	out := make([]huh.Option[string], 0, len(q.Options))
	defaults := map[string]bool{}
	if d, ok := q.Default.([]string); ok {
		for _, v := range d {
			defaults[v] = true
		}
	}
	for _, o := range q.Options {
		opt := huh.NewOption(o.Label, o.Value)
		if defaults[o.Value] {
			opt = opt.Selected(true)
		}
		out = append(out, opt)
	}
	return out
}
