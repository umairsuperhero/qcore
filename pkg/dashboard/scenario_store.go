package dashboard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/qcore-project/qcore/pkg/simulator"
	"gopkg.in/yaml.v3"
)

var scenarioNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

type ScenarioSummary struct {
	Name        string                    `json:"name"`
	Description string                    `json:"description,omitempty"`
	Mode        string                    `json:"mode,omitempty"`
	Scenario    string                    `json:"scenario,omitempty"`
	Expect      *simulator.ScenarioExpect `json:"expect,omitempty"`
}

type ScenarioRecord struct {
	ScenarioSummary
	YAML       string                        `json:"yaml"`
	Definition *simulator.ScenarioDefinition `json:"-"`
}

type ScenarioStore struct {
	dir string
}

func NewScenarioStore(dir string) (*ScenarioStore, error) {
	if strings.TrimSpace(dir) == "" {
		dir = "scenarios"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create scenario dir %s: %w", dir, err)
	}
	return &ScenarioStore{dir: dir}, nil
}

func ValidateScenarioName(name string) error {
	if !scenarioNamePattern.MatchString(name) {
		return fmt.Errorf("invalid scenario name %q: use 1-64 letters, numbers, hyphen, or underscore; start with a letter or number", name)
	}
	return nil
}

func (s *ScenarioStore) Save(def *simulator.ScenarioDefinition) (*ScenarioRecord, error) {
	if def == nil {
		return nil, errors.New("scenario definition is required")
	}
	if err := validateScenarioDefinition(def); err != nil {
		return nil, err
	}
	data, err := yaml.Marshal(def)
	if err != nil {
		return nil, fmt.Errorf("marshal scenario YAML: %w", err)
	}
	path, err := s.path(def.Name)
	if err != nil {
		return nil, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return nil, fmt.Errorf("write scenario %s: %w", def.Name, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return nil, fmt.Errorf("save scenario %s: %w", def.Name, err)
	}
	return scenarioRecord(def, string(data)), nil
}

func (s *ScenarioStore) Get(name string) (*ScenarioRecord, error) {
	path, err := s.path(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	def, err := simulator.LoadScenario(strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	if err := validateScenarioDefinition(def); err != nil {
		return nil, err
	}
	return scenarioRecord(def, string(data)), nil
}

func (s *ScenarioStore) List() ([]ScenarioSummary, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	out := make([]ScenarioSummary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".yaml")
		rec, err := s.Get(name)
		if err != nil {
			return nil, err
		}
		out = append(out, rec.ScenarioSummary)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *ScenarioStore) Delete(name string) error {
	path, err := s.path(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return nil
}

func (s *ScenarioStore) path(name string) (string, error) {
	if err := ValidateScenarioName(name); err != nil {
		return "", err
	}
	return filepath.Join(s.dir, name+".yaml"), nil
}

func validateScenarioDefinition(def *simulator.ScenarioDefinition) error {
	def.Name = strings.TrimSpace(def.Name)
	def.Mode = strings.ToLower(strings.TrimSpace(def.Mode))
	def.Scenario = strings.TrimSpace(def.Scenario)
	if err := ValidateScenarioName(def.Name); err != nil {
		return err
	}
	switch def.Mode {
	case "", "4g", "5g":
	default:
		return fmt.Errorf("invalid scenario mode %q: use 4g or 5g", def.Mode)
	}
	if def.Scenario != "" && !validScenario(def.Scenario) {
		return fmt.Errorf("invalid built-in scenario %q", def.Scenario)
	}
	if def.Expect != nil {
		def.Expect.Result = strings.ToLower(strings.TrimSpace(def.Expect.Result))
		def.Expect.Cause = strings.TrimSpace(def.Expect.Cause)
		def.Expect.FailedStep = strings.TrimSpace(def.Expect.FailedStep)
		switch def.Expect.Result {
		case "", "success", "failure":
		default:
			return fmt.Errorf("invalid expect.result %q: use success or failure", def.Expect.Result)
		}
	}
	return nil
}

func scenarioRecord(def *simulator.ScenarioDefinition, raw string) *ScenarioRecord {
	return &ScenarioRecord{
		ScenarioSummary: ScenarioSummary{
			Name:        def.Name,
			Description: def.Description,
			Mode:        def.Mode,
			Scenario:    def.Scenario,
			Expect:      def.Expect,
		},
		YAML:       raw,
		Definition: def,
	}
}
