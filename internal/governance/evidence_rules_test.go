package governance

import (
	"reflect"
	"testing"

	"github.com/gunesis-game/pmcp/internal/state"
)

func TestRequiredWorkflowsFollowTransitiveDependants(t *testing.T) {
	policy := &state.Policy{Testing: state.Testing{
		Tiers: map[string][]string{"fast": {"Lint"}, "integration": {"CI"}, "full": {"CI", "E2E"}},
		Components: map[string]state.Component{
			"store":  {Tests: []string{"Store tests"}, Dependants: []string{"core"}},
			"core":   {Tests: []string{"Core tests"}, Dependants: []string{"server"}},
			"server": {Tests: []string{"Server tests"}},
			"guide":  {Tests: []string{"Guide tests"}},
		},
	}}
	request, err := ParseEvidenceRequest(ArgsOf(map[string]any{"tier": "fast", "components": "store"}), policy, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Core tests", "Lint", "Server tests", "Store tests"}
	if !reflect.DeepEqual(request.Required, want) {
		t.Fatalf("got %v, want %v", request.Required, want)
	}
	if _, err := ParseEvidenceRequest(ArgsOf(map[string]any{"tier": "fast", "components": "nope"}), policy, 0); err == nil {
		t.Fatal("an unknown component must be rejected")
	}
	full, err := ParseEvidenceRequest(ArgsOf(nil), policy, 0)
	if err != nil || !reflect.DeepEqual(full.Required, []string{"CI", "E2E"}) || full.WaitMinutes != DefaultEvidenceWaitMinutes {
		t.Fatalf("default tier is full with the default wait: %v %v", full, err)
	}
	if _, err := ParseEvidenceRequest(ArgsOf(map[string]any{"wait_minutes": 61}), policy, 0); err == nil {
		t.Fatal("wait_minutes above 60 must be rejected")
	}
}

func TestArgsListAcceptsStringOrList(t *testing.T) {
	args := ArgsOf(map[string]any{"one": "a", "many": []string{"a", "b"}, "blank": "  ", "bad": []any{"a", ""}})
	if values, _, _ := args.Strings("one"); !reflect.DeepEqual(values, []string{"a"}) {
		t.Fatalf("single string: %v", values)
	}
	if values, _, _ := args.Strings("many"); len(values) != 2 {
		t.Fatalf("list: %v", values)
	}
	if _, _, err := args.Strings("blank"); err == nil {
		t.Fatal("blank string must be rejected")
	}
	if _, _, err := args.Strings("bad"); err == nil {
		t.Fatal("list with an empty string must be rejected")
	}
	if err := args.Allow("one"); err == nil {
		t.Fatal("unknown arguments must be rejected")
	}
}
