package dashboard

import (
	"os"
	"strings"
	"testing"

	"github.com/qcore-project/qcore/pkg/simulator"
)

func TestScenarioStoreCRUDAndNameValidation(t *testing.T) {
	store, err := NewScenarioStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewScenarioStore: %v", err)
	}

	def := &simulator.ScenarioDefinition{
		Name:        "wrong_ki_check",
		Description: "Expect the wrong Ki diagnostic.",
		Mode:        "5g",
		Scenario:    "wrong_ki",
		Expect:      &simulator.ScenarioExpect{Result: "failure", Cause: "wrong_ki"},
	}
	if _, err := store.Save(def); err != nil {
		t.Fatalf("Save: %v", err)
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Name != def.Name || list[0].Expect.Cause != "wrong_ki" {
		t.Fatalf("unexpected list: %+v", list)
	}

	got, err := store.Get(def.Name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Definition.Scenario != "wrong_ki" || !strings.Contains(got.YAML, "expect:") {
		t.Fatalf("unexpected record: %+v", got)
	}

	if err := store.Delete(def.Name); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(def.Name); !os.IsNotExist(err) {
		t.Fatalf("Get after delete error = %v, want not exist", err)
	}

	def.Name = "../bad"
	if _, err := store.Save(def); err == nil {
		t.Fatal("expected invalid name to fail")
	}
}

func TestScenarioStoreRejectsMalformedDefinition(t *testing.T) {
	store, err := NewScenarioStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewScenarioStore: %v", err)
	}

	_, err = store.Save(&simulator.ScenarioDefinition{
		Name:     "bad_mode",
		Mode:     "6g",
		Scenario: "wrong_ki",
	})
	if err == nil || !strings.Contains(err.Error(), "mode") {
		t.Fatalf("expected mode validation error, got %v", err)
	}

	_, err = store.Save(&simulator.ScenarioDefinition{
		Name:     "bad_fault",
		Mode:     "5g",
		Scenario: "packet_storm",
	})
	if err == nil || !strings.Contains(err.Error(), "built-in scenario") {
		t.Fatalf("expected scenario validation error, got %v", err)
	}
}
