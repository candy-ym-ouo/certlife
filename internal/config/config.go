// Package config loads CertLife settings from YAML, environment variables,
// and command-line overrides supplied by the executable.
package config
import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"gopkg.in/yaml.v3"
)
// Duration gives YAML configuration a human-readable duration format.
type Duration struct{ time.Duration }
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	parsed, err := time.ParseDuration(value.Value)
	if err != nil { return fmt.Errorf("invalid duration %q: %w", value.Value, err) }
	d.Duration = parsed
	return nil
}
func (d Duration) MarshalYAML() (any, error) { return d.String(), nil }
// Config is the complete application configuration.
type Config struct {
	Addr     string          `yaml:"addr"`
	APIKey   string          `yaml:"api_key"`
	DBPath   string          `yaml:"db_path"`
	LogLevel string          `yaml:"log_level"`
	Deploy   DeployConfig    `yaml:"deploy"`
	Notify   NotifyConfig    `yaml:"notify"`
	Schedule SchedulerConfig `yaml:"scheduler"`
}
type DeployConfig struct { Directory      string `yaml:"directory"`; RollbackOnFail bool   `yaml:"rollback_on_fail"` }
type NotifyConfig struct {
	SMTPHost      string `yaml:"smtp_host"`
	SMTPPort      int    `yaml:"smtp_port"`
	SMTPUsername  string `yaml:"smtp_username"`
	SMTPPassword  string `yaml:"smtp_password"`
	SMTPFrom      string `yaml:"smtp_from"`
	WebhookSecret string `yaml:"webhook_secret"`
}
type SchedulerConfig struct {
	ExpiryChecker    Duration `yaml:"expiry_checker_interval"`
	RenewalRunner    Duration `yaml:"renewal_runner_interval"`
	DeployVerifier   Duration `yaml:"deploy_verifier_interval"`
	NotifyDispatcher Duration `yaml:"notify_dispatcher_interval"`
	DailySummary     Duration `yaml:"daily_summary_interval"`
}
// Default returns a configuration suitable for local development.
func Default() Config {
	return Config{
		Addr: "127.0.0.1:8080", APIKey: "dev-key", DBPath: "certlife.db", LogLevel: "info",
		Deploy: DeployConfig{Directory: "deployments", RollbackOnFail: true},
		Notify: NotifyConfig{SMTPPort: 25, SMTPFrom: "certlife@localhost", WebhookSecret: "dev-secret"},
		Schedule: SchedulerConfig{
			ExpiryChecker: Duration{6 * time.Hour}, RenewalRunner: Duration{time.Hour},
			DeployVerifier: Duration{30 * time.Minute}, NotifyDispatcher: Duration{5 * time.Minute},
			DailySummary: Duration{24 * time.Hour},
		},
	}
}
// Load reads a YAML file. A missing empty path is allowed and uses defaults.
func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) { return cfg, err }
		} else if err := yaml.Unmarshal(data, &cfg); err != nil { return cfg, fmt.Errorf("parse config: %w", err) }
	}
	applyEnvironment(&cfg)
	if cfg.Addr == "" || cfg.APIKey == "" || cfg.DBPath == "" { return cfg, errors.New("addr, api_key and db_path are required") }
	return cfg, nil
}
func applyEnvironment(cfg *Config) {
	setString("CERTLIFE_ADDR", &cfg.Addr)
	setString("CERTLIFE_API_KEY", &cfg.APIKey)
	setString("CERTLIFE_DB", &cfg.DBPath)
	setString("CERTLIFE_LOG_LEVEL", &cfg.LogLevel)
	setString("CERTLIFE_DEPLOY_DIR", &cfg.Deploy.Directory)
	setString("CERTLIFE_SMTP_HOST", &cfg.Notify.SMTPHost)
	setString("CERTLIFE_SMTP_USERNAME", &cfg.Notify.SMTPUsername)
	setString("CERTLIFE_SMTP_PASSWORD", &cfg.Notify.SMTPPassword)
	setString("CERTLIFE_SMTP_FROM", &cfg.Notify.SMTPFrom)
	setString("CERTLIFE_WEBHOOK_SECRET", &cfg.Notify.WebhookSecret)
	if value := strings.TrimSpace(os.Getenv("CERTLIFE_SMTP_PORT")); value != "" {
		if port, err := strconv.Atoi(value); err == nil { cfg.Notify.SMTPPort = port }
	}
}
func setString(name string, target *string) {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" { *target = value }
}
