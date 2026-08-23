package model
import (
	"errors"
	"fmt"
	"math"
	"net/mail"
	"strings"
	"time"
)
type CertificateStatus string
const (
	CertificateValid        CertificateStatus = "valid"
	CertificateExpiringSoon CertificateStatus = "expiring_soon"
	CertificateExpired      CertificateStatus = "expired"
)
// Certificate is the registered metadata and policy for one certificate.
type Certificate struct {
	ID                  int64             `json:"id"`
	Name                string            `json:"name"`
	Domain              string            `json:"domain"`
	SANs                []string          `json:"sans"`
	Issuer              string            `json:"issuer"`
	SerialNumber        string            `json:"serial_number"`
	Algorithm           string            `json:"algorithm"`
	KeyBits             int               `json:"key_bits"`
	ValidFrom           time.Time         `json:"valid_from"`
	ValidUntil          time.Time         `json:"valid_until"`
	Owner               string            `json:"owner"`
	Environment         string            `json:"environment"`
	Tags                []string          `json:"tags"`
	Contacts            []string          `json:"contacts"`
	NotifyThresholdDays int               `json:"notify_threshold_days"`
	AutoRenew           bool              `json:"auto_renew"`
	RenewThresholdDays  int               `json:"renew_threshold_days"`
	DeployTargetIDs     []int64           `json:"deploy_target_ids"`
	Status              CertificateStatus `json:"status"`
	DaysToExpire        int               `json:"days_to_expire"`
	LastCheckedAt       *time.Time        `json:"last_checked_at,omitempty"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
	DeletedAt           *time.Time        `json:"deleted_at,omitempty"`
}
func (c *Certificate) ApplyDefaults() {
	c.Name = strings.TrimSpace(c.Name)
	c.Domain = strings.ToLower(strings.TrimSpace(c.Domain))
	c.Issuer = strings.TrimSpace(c.Issuer)
	c.SerialNumber = strings.ToUpper(strings.TrimSpace(c.SerialNumber))
	c.Algorithm = strings.ToUpper(strings.TrimSpace(c.Algorithm))
	c.Environment = strings.ToLower(strings.TrimSpace(c.Environment))
	if c.Environment == "" { c.Environment = "prod" }
	if c.NotifyThresholdDays == 0 { c.NotifyThresholdDays = 30 }
	if c.RenewThresholdDays == 0 { c.RenewThresholdDays = 14 }
	c.SANs = uniqueStrings(c.SANs)
	c.Tags = uniqueStrings(c.Tags)
	c.Contacts = uniqueStrings(c.Contacts)
}
func (c Certificate) Validate() error {
	if c.Name == "" || c.Domain == "" || c.Issuer == "" || c.SerialNumber == "" { return errors.New("name, domain, issuer and serial_number are required") }
	if !c.ValidFrom.Before(c.ValidUntil) { return errors.New("valid_from must be before valid_until") }
	if c.Algorithm != "RSA" && c.Algorithm != "ECDSA" { return errors.New("algorithm must be RSA or ECDSA") }
	if c.Environment != "prod" && c.Environment != "staging" && c.Environment != "dev" { return errors.New("environment must be prod, staging or dev") }
	if c.NotifyThresholdDays < 0 || c.RenewThresholdDays < 0 { return errors.New("threshold days cannot be negative") }
	for _, address := range c.Contacts {
		if _, err := mail.ParseAddress(address); err != nil { return fmt.Errorf("invalid contact %q", address) }
	}
	return nil
}
func (c *Certificate) Refresh(now time.Time) CertificateStatus {
	c.DaysToExpire = DaysUntil(c.ValidUntil, now)
	switch {
	case !c.ValidUntil.After(now):
		c.Status = CertificateExpired
	case c.DaysToExpire <= c.NotifyThresholdDays:
		c.Status = CertificateExpiringSoon
	default:
		c.Status = CertificateValid
	}
	checked := now.UTC()
	c.LastCheckedAt = &checked
	return c.Status
}
func DaysUntil(deadline, now time.Time) int { return int(math.Ceil(deadline.Sub(now).Hours() / 24)) }
func (c Certificate) AllDomains() []string {
	return uniqueStrings(append([]string{c.Domain}, c.SANs...))
}
func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] { seen[value] = true; out = append(out, value) }
	}
	return out
}
