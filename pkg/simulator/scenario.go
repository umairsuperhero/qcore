package simulator

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// ScenarioDefinition defines a declarative test scenario for the simulator.
type ScenarioDefinition struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description" json:"description"`
	Mode        string            `yaml:"mode" json:"mode"`                               // "4g" or "5g"
	Scenario    string            `yaml:"scenario,omitempty" json:"scenario,omitempty"`   // optional built-in fault hook
	Overrides   ScenarioOverrides `yaml:"overrides,omitempty" json:"overrides,omitempty"` // optional direct option overrides
	Expect      *ScenarioExpect   `yaml:"expect,omitempty" json:"expect,omitempty"`       // optional deterministic PASS/FAIL contract
}

// ScenarioOverrides defines fields to override during the attach.
type ScenarioOverrides struct {
	MMEAddr string `yaml:"mme_address,omitempty" json:"mme_address,omitempty"`
	PLMN    string `yaml:"plmn,omitempty" json:"plmn,omitempty"` // e.g. "00101"
	TAC     uint16 `yaml:"tac,omitempty" json:"tac,omitempty"`
	IMSI    string `yaml:"imsi,omitempty" json:"imsi,omitempty"`
	Ki      string `yaml:"ki,omitempty" json:"ki,omitempty"`
	OPc     string `yaml:"opc,omitempty" json:"opc,omitempty"`
}

type ScenarioExpect struct {
	Result     string `yaml:"result" json:"result"`                               // "success" or "failure"
	Cause      string `yaml:"cause,omitempty" json:"cause,omitempty"`             // optional diagnostic/fault cause tag
	FailedStep string `yaml:"failed_step,omitempty" json:"failed_step,omitempty"` // optional simulator step
}

// LoadScenario parses a YAML scenario definition from an io.Reader.
func LoadScenario(r io.Reader) (*ScenarioDefinition, error) {
	var sd ScenarioDefinition
	decoder := yaml.NewDecoder(r)
	if err := decoder.Decode(&sd); err != nil {
		return nil, fmt.Errorf("failed to parse scenario YAML: %w", err)
	}
	return &sd, nil
}

// LoadScenarioFile reads a scenario definition from disk.
func LoadScenarioFile(path string) (*ScenarioDefinition, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return LoadScenario(f)
}

// Apply updates the simulator Options with the overrides defined in the scenario.
func (sd *ScenarioDefinition) Apply(opts *Options) error {
	if sd.Mode != "" {
		opts.Mode = sd.Mode
	}
	if sd.Scenario != "" {
		opts.Scenario = sd.Scenario
	}
	if sd.Overrides.MMEAddr != "" {
		opts.MMEAddr = sd.Overrides.MMEAddr
	}
	if sd.Overrides.PLMN != "" {
		plmn, err := PackPLMN(sd.Overrides.PLMN)
		if err != nil {
			return fmt.Errorf("invalid PLMN in scenario: %w", err)
		}
		opts.PLMN = plmn
	}
	if sd.Overrides.TAC != 0 {
		opts.TAC = sd.Overrides.TAC
	}
	if sd.Overrides.IMSI != "" {
		opts.IMSI = sd.Overrides.IMSI
	}
	if sd.Overrides.Ki != "" {
		opts.Ki = sd.Overrides.Ki
	}
	if sd.Overrides.OPc != "" {
		opts.OPc = sd.Overrides.OPc
	}
	// Declarative overrides can drive failures on their own, while Scenario lets
	// authored scenarios reuse the small built-in fault hooks.
	return nil
}
