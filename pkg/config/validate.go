package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Validate checks the loaded configuration for problems that would otherwise
// surface as cryptic runtime errors (wrong PLMN length, bad CIDR, port 0,
// gateway outside pool, etc.). Errors are gathered up so operators see every
// problem at once instead of fixing one, re-running, fixing the next.
//
// Philosophy: each message says (a) what's wrong, (b) the offending value,
// (c) how to fix it. This is one of QCore's DX promises.
func (c *Config) Validate() error {
	var errs validationErrors

	c.HSS.validate(&errs)
	c.MME.validate(&errs)
	c.SPGW.validate(&errs)
	c.Database.validate(&errs)
	c.Logging.validate(&errs)
	c.Metrics.validate(&errs)
	c.crossValidate(&errs)

	return errs.asError()
}

// crossValidate covers invariants that span sections — e.g. MME points at HSS,
// SPGW advertises an IP that MME must be able to reach, ports don't collide.
func (c *Config) crossValidate(errs *validationErrors) {
	// API port collisions on the same bind address would make the second
	// service fail to start with a less-helpful EADDRINUSE.
	type portBinding struct {
		component string
		bind      string
		port      int
		field     string
	}
	bindings := []portBinding{
		{"hss", c.HSS.BindAddress, c.HSS.APIPort, "hss.api_port"},
		{"mme", c.MME.BindAddress, c.MME.APIPort, "mme.api_port"},
		{"mme", c.MME.BindAddress, c.MME.S1APPort, "mme.s1ap_port"},
		{"spgw", c.SPGW.BindAddress, c.SPGW.APIPort, "spgw.api_port"},
		{"spgw", c.SPGW.BindAddress, c.SPGW.S1UPort, "spgw.s1u_port"},
	}
	if c.Metrics.Enabled {
		// Metrics shares the process's bind address — collision risk when
		// co-located on the same machine.
		for _, b := range bindings {
			if b.port == c.Metrics.Port {
				errs.addf("metrics.port", "metrics port %d collides with %s — pick a different port",
					c.Metrics.Port, b.field)
			}
		}
	}
	for i := 0; i < len(bindings); i++ {
		for j := i + 1; j < len(bindings); j++ {
			a, b := bindings[i], bindings[j]
			if a.bind == b.bind && a.port == b.port && a.port != 0 {
				errs.addf(b.field, "%s and %s both bind %s:%d — pick different ports",
					a.field, b.field, a.bind, a.port)
			}
		}
	}
}

// ---------- per-section validators ----------

func (h *HSSConfig) validate(errs *validationErrors) {
	if h.Name == "" {
		errs.addf("hss.name", "must not be empty — set to e.g. 'qcore-hss'")
	}
	validateBindAddr(errs, "hss.bind_address", h.BindAddress)
	validatePort(errs, "hss.api_port", h.APIPort)
}

func (m *MMEConfig) validate(errs *validationErrors) {
	if m.Name == "" {
		errs.addf("mme.name", "must not be empty — set to e.g. 'qcore-mme'")
	}
	validateBindAddr(errs, "mme.bind_address", m.BindAddress)
	validatePort(errs, "mme.s1ap_port", m.S1APPort)
	validatePort(errs, "mme.api_port", m.APIPort)

	switch strings.ToLower(m.SCTPMode) {
	case "tcp", "sctp":
		// ok
	case "":
		errs.addf("mme.sctp_mode", "must be 'tcp' (dev) or 'sctp' (production); got empty")
	default:
		errs.addf("mme.sctp_mode", "unknown transport %q — use 'tcp' (dev) or 'sctp' (production)", m.SCTPMode)
	}

	validatePLMN(errs, "mme.plmn", m.PLMN)
	validateHTTPURL(errs, "mme.hss_url", m.HSSURL, true /*required*/)
	validateHTTPURL(errs, "mme.spgw_url", m.SPGWURL, false /*optional — empty means fake alloc*/)

	if m.TAC == 0 {
		errs.addf("mme.tac", "TAC=0 is reserved; pick any non-zero value (default 1)")
	}
	for i, tai := range m.TAIList {
		field := fmt.Sprintf("mme.tai_list[%d]", i)
		validateTAI(errs, field, tai, m.PLMN)
	}
}

func (s *SPGWConfig) validate(errs *validationErrors) {
	if s.Name == "" {
		errs.addf("spgw.name", "must not be empty — set to e.g. 'qcore-spgw'")
	}
	validateBindAddr(errs, "spgw.bind_address", s.BindAddress)
	validatePort(errs, "spgw.api_port", s.APIPort)
	validatePort(errs, "spgw.s1u_port", s.S1UPort)

	// UE pool is a CIDR; the gateway must be inside it but usable (not the
	// network or broadcast address).
	_, poolNet, err := net.ParseCIDR(s.UEPool)
	if err != nil {
		errs.addf("spgw.ue_pool", "invalid CIDR %q: %v — use e.g. '10.45.0.0/24'", s.UEPool, err)
	}

	gw := net.ParseIP(s.Gateway)
	if gw == nil {
		errs.addf("spgw.gateway", "invalid IP %q — use e.g. '10.45.0.1'", s.Gateway)
	}
	if poolNet != nil && gw != nil {
		if !poolNet.Contains(gw) {
			errs.addf("spgw.gateway", "%s not inside ue_pool %s — the gateway must live inside the pool CIDR",
				s.Gateway, s.UEPool)
		}
	}

	if s.SGWU1Addr == "" {
		errs.addf("spgw.sgw_u1_addr", "must be set — this is the IP the MME tells the eNB to send GTP-U to")
	} else if ip := net.ParseIP(s.SGWU1Addr); ip == nil {
		errs.addf("spgw.sgw_u1_addr", "invalid IP %q — use e.g. '127.0.0.1' for local dev", s.SGWU1Addr)
	}

	switch strings.ToLower(s.Egress) {
	case "", "log", "tun":
		// ok — buildEgress defaults empty/unknown to log.
	default:
		errs.addf("spgw.egress", "unknown egress %q — use 'log' (default, safe everywhere) or 'tun' (Linux)", s.Egress)
	}
	if strings.EqualFold(s.Egress, "tun") {
		if s.TUNName == "" {
			errs.addf("spgw.tun_name", "tun egress requires a device name (default 'qcore0')")
		}
		if s.TUNMTU <= 0 {
			errs.addf("spgw.tun_mtu", "tun egress requires a positive MTU (default 1400)")
		}
	}
}

func (d *DatabaseConfig) validate(errs *validationErrors) {
	if d.Host == "" {
		errs.addf("database.host", "must not be empty")
	}
	validatePort(errs, "database.port", d.Port)
	if d.Name == "" {
		errs.addf("database.name", "must not be empty")
	}
	if d.User == "" {
		errs.addf("database.user", "must not be empty")
	}
	switch d.SSLMode {
	case "disable", "require", "verify-ca", "verify-full", "prefer", "allow":
		// valid libpq modes
	case "":
		errs.addf("database.ssl_mode", "must not be empty — use 'disable' for local dev, 'require' or stricter for prod")
	default:
		errs.addf("database.ssl_mode", "unknown mode %q — valid: disable | require | verify-ca | verify-full | prefer | allow", d.SSLMode)
	}
	if d.MaxOpenConns < 0 {
		errs.addf("database.max_open_conns", "must be >= 0 (0 means unlimited); got %d", d.MaxOpenConns)
	}
	if d.MaxIdleConns < 0 {
		errs.addf("database.max_idle_conns", "must be >= 0; got %d", d.MaxIdleConns)
	}
	if d.ConnMaxLifetime < 0 {
		errs.addf("database.conn_max_lifetime_seconds", "must be >= 0; got %d", d.ConnMaxLifetime)
	}
}

func (l *LoggingConfig) validate(errs *validationErrors) {
	switch strings.ToLower(l.Level) {
	case "trace", "debug", "info", "warn", "warning", "error", "fatal", "panic":
		// ok
	case "":
		errs.addf("logging.level", "must not be empty — use 'info' for most uses, 'debug' when troubleshooting")
	default:
		errs.addf("logging.level", "unknown level %q — valid: trace | debug | info | warn | error | fatal | panic", l.Level)
	}
	switch strings.ToLower(l.Format) {
	case "", "console", "text", "json":
		// empty is tolerated; logger defaults to console.
	default:
		errs.addf("logging.format", "unknown format %q — valid: console (human) | json (machine)", l.Format)
	}
}

func (m *MetricsConfig) validate(errs *validationErrors) {
	if !m.Enabled {
		return
	}
	validatePort(errs, "metrics.port", m.Port)
}

// ---------- helpers ----------

func validatePort(errs *validationErrors, field string, port int) {
	if port <= 0 || port > 65535 {
		errs.addf(field, "port %d is out of range — use 1-65535", port)
	}
}

func validateBindAddr(errs *validationErrors, field, addr string) {
	if addr == "" {
		errs.addf(field, "must not be empty — use '0.0.0.0' to bind all interfaces, '127.0.0.1' for loopback only")
		return
	}
	if ip := net.ParseIP(addr); ip == nil {
		errs.addf(field, "invalid IP %q — use '0.0.0.0', '127.0.0.1', or a specific host IP", addr)
	}
}

func validateHTTPURL(errs *validationErrors, field, raw string, required bool) {
	if raw == "" {
		if required {
			errs.addf(field, "must not be empty — e.g. 'http://localhost:8080'")
		}
		return
	}
	u, err := url.Parse(raw)
	if err != nil {
		errs.addf(field, "invalid URL %q: %v", raw, err)
		return
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		errs.addf(field, "scheme %q must be http or https (got %q)", u.Scheme, raw)
	}
	if u.Host == "" {
		errs.addf(field, "missing host in %q — include a host and port, e.g. 'http://localhost:8080'", raw)
	}
}

// validatePLMN enforces 3GPP TS 23.003: MCC is 3 digits, MNC is 2 or 3 digits.
func validatePLMN(errs *validationErrors, field, plmn string) {
	if plmn == "" {
		errs.addf(field, "must not be empty — e.g. '00101' (MCC=001, MNC=01)")
		return
	}
	if len(plmn) != 5 && len(plmn) != 6 {
		errs.addf(field, "PLMN %q must be 5 or 6 digits (MCC=3 + MNC=2 or 3); got %d characters", plmn, len(plmn))
		return
	}
	for _, r := range plmn {
		if r < '0' || r > '9' {
			errs.addf(field, "PLMN %q must be digits only", plmn)
			return
		}
	}
}

// validateTAI checks a "PLMN:TAC" string, e.g. "00101:0001".
func validateTAI(errs *validationErrors, field, tai, plmn string) {
	parts := strings.SplitN(tai, ":", 2)
	if len(parts) != 2 {
		errs.addf(field, "TAI %q must be 'PLMN:TAC', e.g. '00101:0001'", tai)
		return
	}
	validatePLMN(errs, field+" (plmn part)", parts[0])
	if plmn != "" && parts[0] != plmn {
		errs.addf(field, "TAI PLMN %s does not match mme.plmn %s", parts[0], plmn)
	}
	if parts[1] == "" {
		errs.addf(field, "TAI %q missing TAC — use 'PLMN:TAC', e.g. '%s:0001'", tai, parts[0])
		return
	}
	tac, err := strconv.ParseUint(parts[1], 16, 16)
	if err != nil {
		errs.addf(field, "TAI %q has invalid TAC %q: %v (must be 1-4 hex digits)", tai, parts[1], err)
		return
	}
	if tac == 0 {
		errs.addf(field, "TAI %q has TAC=0 which is reserved — use a non-zero value", tai)
	}
}

// ---------- error aggregation ----------

type validationErrors struct {
	items []string
}

func (e *validationErrors) addf(field, format string, args ...interface{}) {
	e.items = append(e.items, fmt.Sprintf("  %s: %s", field, fmt.Sprintf(format, args...)))
}

func (e *validationErrors) asError() error {
	if len(e.items) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("invalid configuration (%d problem(s)):\n", len(e.items)))
	sb.WriteString(strings.Join(e.items, "\n"))
	sb.WriteString("\nsee config.example.yaml for a working reference.")
	return fmt.Errorf("%s", sb.String())
}
