package simulator

import (
	"strings"
	"testing"
)

func TestLoadScenarioWithScenarioAndExpect(t *testing.T) {
	def, err := LoadScenario(strings.NewReader(`
name: wrong-ki-check
description: Exercise the auth mismatch rule.
mode: 5g
scenario: wrong_ki
expect:
  result: failure
  cause: wrong_ki
  failed_step: security_mode_command
`))
	if err != nil {
		t.Fatalf("LoadScenario: %v", err)
	}
	if def.Name != "wrong-ki-check" || def.Mode != "5g" || def.Scenario != "wrong_ki" {
		t.Fatalf("unexpected scenario definition: %+v", def)
	}
	if def.Expect == nil || def.Expect.Result != "failure" || def.Expect.Cause != "wrong_ki" {
		t.Fatalf("unexpected expect block: %+v", def.Expect)
	}

	opts := Options{}
	if err := def.Apply(&opts); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if opts.Mode != "5g" || opts.Scenario != "wrong_ki" {
		t.Fatalf("Apply did not set mode/scenario: %+v", opts)
	}
}
