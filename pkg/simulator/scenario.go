package simulator

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// ScenarioDefinition defines a declarative test scenario for the simulator.
type ScenarioDefinition struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Mode        string            `yaml:"mode"` // "4g" or "5g"
	Overrides   ScenarioOverrides `yaml:"overrides"`
}

// ScenarioOverrides defines fields to override during the attach.
type ScenarioOverrides struct {
	MMEAddr string `yaml:"mme_address,omitempty"`
	PLMN    string `yaml:"plmn,omitempty"` // e.g. "00101"
	TAC     uint16 `yaml:"tac,omitempty"`
	IMSI    string `yaml:"imsi,omitempty"`
	Ki      string `yaml:"ki,omitempty"`
	OPc     string `yaml:"opc,omitempty"`
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
	// Note: We don't override opts.Scenario here because the declarative overrides
	// themselves drive the failure, so opts.Scenario (the hardcoded hook) isn't needed.
	return nil
}
