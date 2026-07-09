package wizard

import (
	"reflect"
	"strings"
	"testing"
)

func TestEffectiveList_MergesFreeTextOtherIntoCheckedOptions(t *testing.T) {
	a := Answers{
		"toolchain.packageManagers":       []string{"go-modules", "other"},
		"toolchain.packageManagers.other": "Maven, Cargo",
	}
	got := a.EffectiveList("toolchain.packageManagers")
	want := []string{"go-modules", "Maven", "Cargo"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("EffectiveList() = %v, want %v", got, want)
	}
}

func TestEffectiveList_KeepsOtherPlaceholderWhenNoFreeTextGiven(t *testing.T) {
	a := Answers{"toolchain.packageManagers": []string{"go-modules", "other"}}
	got := a.EffectiveList("toolchain.packageManagers")
	want := []string{"go-modules", "other"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("EffectiveList() = %v, want %v", got, want)
	}
}

func TestEffectiveList_BehavesLikeListWhenNoOtherCompanionExists(t *testing.T) {
	a := Answers{"profile.appTypes": []string{"cli", "api"}}
	got := a.EffectiveList("profile.appTypes")
	want := []string{"cli", "api"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("EffectiveList() = %v, want %v", got, want)
	}
}

func TestValidate_ListQuestion(t *testing.T) {
	t.Run("accepts []string as-is", func(t *testing.T) {
		a := Answers{"standards.custom": []string{"always test", "no coverage theater"}}
		got, err := a.validate()
		if err != nil {
			t.Fatalf("validate() error = %v", err)
		}
		want := []string{"always test", "no coverage theater"}
		if !reflect.DeepEqual(got.List("standards.custom"), want) {
			t.Errorf("standards.custom = %v, want %v", got.List("standards.custom"), want)
		}
	})

	t.Run("accepts []any of strings", func(t *testing.T) {
		a := Answers{"standards.custom": []any{"always test", "no coverage theater"}}
		got, err := a.validate()
		if err != nil {
			t.Fatalf("validate() error = %v", err)
		}
		want := []string{"always test", "no coverage theater"}
		if !reflect.DeepEqual(got.List("standards.custom"), want) {
			t.Errorf("standards.custom = %v, want %v", got.List("standards.custom"), want)
		}
	})

	t.Run("coerces a newline-separated string", func(t *testing.T) {
		a := Answers{"standards.custom": "always test\n\nno coverage theater\n  edge cases  \n"}
		got, err := a.validate()
		if err != nil {
			t.Fatalf("validate() error = %v", err)
		}
		want := []string{"always test", "no coverage theater", "edge cases"}
		if !reflect.DeepEqual(got.List("standards.custom"), want) {
			t.Errorf("standards.custom = %v, want %v", got.List("standards.custom"), want)
		}
	})

	t.Run("rejects a non-string list member", func(t *testing.T) {
		a := Answers{"standards.custom": []any{"fine", 42}}
		if _, err := a.validate(); err == nil {
			t.Error("expected error for non-string list member")
		}
	})

	t.Run("rejects a non-string non-list value", func(t *testing.T) {
		a := Answers{"standards.custom": 42}
		if _, err := a.validate(); err == nil {
			t.Error("expected error for non-list, non-string value")
		}
	})

	t.Run("absent list answer does not break validation", func(t *testing.T) {
		a := Answers{"toolchain.docker": true}
		got, err := a.validate()
		if err != nil {
			t.Fatalf("validate() error = %v", err)
		}
		if v := got.List("standards.custom"); len(v) != 0 {
			t.Errorf("standards.custom = %v, want empty", v)
		}
	})
}

func TestSummary_IncludesListAnswers(t *testing.T) {
	a := Answers{"standards.custom": []string{"always test", "no coverage theater"}}
	summary := a.Summary()
	if !strings.Contains(summary, "standards.custom: always test, no coverage theater") {
		t.Errorf("Summary() = %q, want it to include the custom instructions list", summary)
	}
}

func TestSplitLines_TrimsAndDropsBlankLines(t *testing.T) {
	got := SplitLines("  first  \n\nsecond\n   \nthird")
	want := []string{"first", "second", "third"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SplitLines() = %v, want %v", got, want)
	}
}
