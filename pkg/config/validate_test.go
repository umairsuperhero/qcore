package config

import (
	"strings"
	"testing"
)

// validConfig returns a Config that passes Validate() cleanly. Each test then
// mutates exactly one field so the failure signal is clean.
func validConfig() *Config {
	return &Config{
		HSS: HSSConfig{
			Name:        "qcore-hss",
			BindAddress: "0.0.0.0",
			APIPort:     8080,
		},
		MME: MMEConfig{
			Name:        "qcore-mme",
			BindAddress: "0.0.0.0",
			S1APPort:    36412,
			APIPort:     8081,
			SCTPMode:    "tcp",
			PLMN:        "00101",
			HSSURL:      "http://localhost:8080",
			TAC:         1,
			MMEGroupID:  1,
			MMECode:     1,
			RelCapacity: 127,
			SPGWURL:     "http://localhost:8082",
		},
		SPGW: SPGWConfig{
			Name:        "qcore-spgw",
			BindAddress: "0.0.0.0",
			APIPort:     8082,
			S1UPort:     2152,
			UEPool:      "10.45.0.0/24",
			Gateway:     "10.45.0.1",
			SGWU1Addr:   "127.0.0.1",
			Egress:      "log",
		},
		Database: DatabaseConfig{
			Host:            "localhost",
			Port:            5432,
			Name:            "qcore",
			User:            "qcore",
			Password:        "qcore",
			SSLMode:         "disable",
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 300,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "console",
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Port:    9090,
		},
	}
}

func TestValidate_DefaultsPass(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestValidate_FieldErrors(t *testing.T) {
	type tc struct {
		name   string
		mutate func(c *Config)
		// wantInError: each listed substring must appear in the error — this
		// also verifies we produce *actionable* messages, not just "bad".
		wantInError []string
	}
	cases := []tc{
		{
			name:        "missing hss name",
			mutate:      func(c *Config) { c.HSS.Name = "" },
			wantInError: []string{"hss.name", "empty"},
		},
		{
			name:        "port out of range",
			mutate:      func(c *Config) { c.HSS.APIPort = 70000 },
			wantInError: []string{"hss.api_port", "70000", "1-65535"},
		},
		{
			name:        "port zero",
			mutate:      func(c *Config) { c.HSS.APIPort = 0 },
			wantInError: []string{"hss.api_port"},
		},
		{
			name:        "bad bind address",
			mutate:      func(c *Config) { c.HSS.BindAddress = "not-an-ip" },
			wantInError: []string{"hss.bind_address", "not-an-ip"},
		},
		{
			name:        "unknown sctp mode",
			mutate:      func(c *Config) { c.MME.SCTPMode = "udp" },
			wantInError: []string{"mme.sctp_mode", "udp", "tcp", "sctp"},
		},
		{
			name:        "plmn too short",
			mutate:      func(c *Config) { c.MME.PLMN = "1234" },
			wantInError: []string{"mme.plmn", "1234", "5 or 6 digits"},
		},
		{
			name:        "plmn non-digit",
			mutate:      func(c *Config) { c.MME.PLMN = "0010A" },
			wantInError: []string{"mme.plmn", "digits only"},
		},
		{
			name:        "tac zero",
			mutate:      func(c *Config) { c.MME.TAC = 0 },
			wantInError: []string{"mme.tac", "reserved"},
		},
		{
			name:        "hss_url missing scheme",
			mutate:      func(c *Config) { c.MME.HSSURL = "localhost:8080" },
			wantInError: []string{"mme.hss_url"},
		},
		{
			name:        "spgw gateway outside pool",
			mutate:      func(c *Config) { c.SPGW.Gateway = "10.99.0.1" },
			wantInError: []string{"spgw.gateway", "ue_pool", "10.45.0.0/24"},
		},
		{
			name:        "spgw bad cidr",
			mutate:      func(c *Config) { c.SPGW.UEPool = "10.45.0.0" /* no mask */ },
			wantInError: []string{"spgw.ue_pool", "10.45.0.0"},
		},
		{
			name: "tun egress without device name",
			mutate: func(c *Config) {
				c.SPGW.Egress = "tun"
				c.SPGW.TUNName = ""
				c.SPGW.TUNMTU = 1400
			},
			wantInError: []string{"spgw.tun_name"},
		},
		{
			name:        "unknown logging level",
			mutate:      func(c *Config) { c.Logging.Level = "super-verbose" },
			wantInError: []string{"logging.level", "super-verbose"},
		},
		{
			name:        "db ssl mode bad",
			mutate:      func(c *Config) { c.Database.SSLMode = "maybe" },
			wantInError: []string{"database.ssl_mode", "maybe"},
		},
		{
			name:        "tai list mismatched plmn",
			mutate:      func(c *Config) { c.MME.TAIList = []string{"00102:0001"} },
			wantInError: []string{"tai_list", "00102", "00101"},
		},
		{
			name:        "tai malformed",
			mutate:      func(c *Config) { c.MME.TAIList = []string{"no-colon"} },
			wantInError: []string{"tai_list", "no-colon", "PLMN:TAC"},
		},
		{
			name: "metrics port collides with api port",
			mutate: func(c *Config) {
				c.Metrics.Enabled = true
				c.Metrics.Port = 8080
			},
			wantInError: []string{"metrics.port", "8080"},
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig()
			tt.mutate(c)
			err := c.Validate()
			if err == nil {
				t.Fatalf("expected validation error for %q, got nil", tt.name)
			}
			msg := err.Error()
			for _, want := range tt.wantInError {
				if !strings.Contains(msg, want) {
					t.Errorf("error missing %q.\nFull error:\n%s", want, msg)
				}
			}
		})
	}
}

func TestValidate_AggregatesMultipleErrors(t *testing.T) {
	c := validConfig()
	c.HSS.Name = ""
	c.MME.PLMN = "xyz"
	c.SPGW.Egress = "ethernet"

	err := c.Validate()
	if err == nil {
		t.Fatal("expected multiple errors, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"hss.name", "mme.plmn", "spgw.egress"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected combined error to mention %q.\nFull error:\n%s", want, msg)
		}
	}
	// Header advertises the count.
	if !strings.Contains(msg, "3 problem") {
		t.Errorf("expected error to report 3 problems.\nFull error:\n%s", msg)
	}
	// Footer should point at the reference config.
	if !strings.Contains(msg, "config.example.yaml") {
		t.Errorf("expected footer to mention config.example.yaml.\nFull error:\n%s", msg)
	}
}

func TestValidate_MetricsDisabledSkipsPortCheck(t *testing.T) {
	c := validConfig()
	c.Metrics.Enabled = false
	c.Metrics.Port = 0 // would normally fail port range check
	if err := c.Validate(); err != nil {
		t.Fatalf("metrics disabled + port=0 should be fine, got: %v", err)
	}
}
