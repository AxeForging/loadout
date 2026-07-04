package wizard

import (
	"reflect"
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
