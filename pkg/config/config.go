package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	HSS      HSSConfig      `mapstructure:"hss"`
	MME      MMEConfig      `mapstructure:"mme"`
	SPGW     SPGWConfig     `mapstructure:"spgw"`
	Database DatabaseConfig `mapstructure:"database"`
	Logging  LoggingConfig  `mapstructure:"logging"`
	Metrics  MetricsConfig  `mapstructure:"metrics"`
}

type HSSConfig struct {
	Name        string `mapstructure:"name"`
	BindAddress string `mapstructure:"bind_address"`
	APIPort     int    `mapstructure:"api_port"`
}

type MMEConfig struct {
	Name        string   `mapstructure:"name"`
	BindAddress string   `mapstructure:"bind_address"`
	S1APPort    int      `mapstructure:"s1ap_port"`
	APIPort     int      `mapstructure:"api_port"`
	SCTPMode    string   `mapstructure:"sctp_mode"` // "tcp" (dev) or "sctp" (production)
	PLMN        string   `mapstructure:"plmn"`      // e.g. "00101"
	HSSURL      string   `mapstructure:"hss_url"`   // HSS REST API base URL
	TAC         uint16   `mapstructure:"tac"`        // Tracking Area Code
	MMEGroupID  uint16   `mapstructure:"mme_group_id"`
	MMECode     uint8    `mapstructure:"mme_code"`
	RelCapacity uint8    `mapstructure:"relative_capacity"` // 0-255, for load balancing
	TAIList     []string `mapstructure:"tai_list"`           // e.g. ["00101:0001"]
	SPGWURL     string   `mapstructure:"spgw_url"`           // HTTP S11 endpoint of SPGW
}

type SPGWConfig struct {
	Name        string `mapstructure:"name"`
	BindAddress string `mapstructure:"bind_address"`
	APIPort     int    `mapstructure:"api_port"` // HTTP API (our S11-over-HTTP)
	S1UPort     int    `mapstructure:"s1u_port"` // GTP-U (2152)
	UEPool      string `mapstructure:"ue_pool"`  // e.g. "10.45.0.0/24"
	Gateway     string `mapstructure:"gateway"`  // e.g. "10.45.0.1"
	SGWU1Addr   string `mapstructure:"sgw_u1_addr"` // what we advertise to the MME as our S1-U IP
	Egress      string `mapstructure:"egress"`      // "log" (default) or "tun" (Linux only)
	TUNName     string `mapstructure:"tun_name"`    // Linux TUN device name (default "qcore0")
	TUNMTU      int    `mapstructure:"tun_mtu"`     // Linux TUN MTU (default 1400 to fit under typical L2 after GTP overhead)
}

type DatabaseConfig struct {
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	Name            string `mapstructure:"name"`
	User            string `mapstructure:"user"`
	Password        string `mapstructure:"password"`
	SSLMode         string `mapstructure:"ssl_mode"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime_seconds"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

type MetricsConfig struct {
	Enabled bool `mapstructure:"enabled"`
	Port    int  `mapstructure:"port"`
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("hss.name", "qcore-hss")
	v.SetDefault("hss.bind_address", "0.0.0.0")
	v.SetDefault("hss.api_port", 8080)

	v.SetDefault("mme.name", "qcore-mme")
	v.SetDefault("mme.bind_address", "0.0.0.0")
	v.SetDefault("mme.s1ap_port", 36412)
	v.SetDefault("mme.api_port", 8081)
	v.SetDefault("mme.sctp_mode", "tcp")
	v.SetDefault("mme.plmn", "00101")
	v.SetDefault("mme.hss_url", "http://localhost:8080")
	v.SetDefault("mme.tac", 1)
	v.SetDefault("mme.mme_group_id", 1)
	v.SetDefault("mme.mme_code", 1)
	v.SetDefault("mme.relative_capacity", 127)
	v.SetDefault("mme.spgw_url", "http://localhost:8082")

	v.SetDefault("spgw.name", "qcore-spgw")
	v.SetDefault("spgw.bind_address", "0.0.0.0")
	v.SetDefault("spgw.api_port", 8082)
	v.SetDefault("spgw.s1u_port", 2152)
	v.SetDefault("spgw.ue_pool", "10.45.0.0/24")
	v.SetDefault("spgw.gateway", "10.45.0.1")
	v.SetDefault("spgw.sgw_u1_addr", "127.0.0.1")
	v.SetDefault("spgw.egress", "log")
	v.SetDefault("spgw.tun_name", "qcore0")
	v.SetDefault("spgw.tun_mtu", 1400)

	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.name", "qcore")
	v.SetDefault("database.user", "qcore")
	v.SetDefault("database.password", "qcore")
	v.SetDefault("database.ssl_mode", "disable")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime_seconds", 300)

	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "console")

	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.port", 9090)
}

func Load(path string) (*Config, error) {
	v := viper.New()
	setDefaults(v)

	v.SetEnvPrefix("QCORE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("reading config file %s: %w", path, err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	return &cfg, nil
}

func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}
