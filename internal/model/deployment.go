package model
import "time"
type DeploymentTarget struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Scheme    string    `json:"scheme"`
	Method    string    `json:"method"`
	VerifyTLS bool      `json:"verify_tls"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
func (t *DeploymentTarget) ApplyDefaults() {
	if t.Port == 0 { t.Port = 443 }
	if t.Scheme == "" { t.Scheme = "https" }
	if t.Method == "" { t.Method = "tls" }
}
func (t DeploymentTarget) Validate() error {
	if t.Name == "" || t.Host == "" { return ErrInvalidTarget }
	if t.Port < 1 || t.Port > 65535 { return ErrInvalidTarget }
	if t.Method != "tls" && t.Method != "static" { return ErrInvalidTarget }
	return nil
}
var ErrInvalidTarget = &ValidationError{Message: "invalid deployment target"}
type ValidationError struct{ Message string }
func (e *ValidationError) Error() string { return e.Message }
type VerificationResult struct {
	HandshakeOK bool      `json:"handshake_ok"`
	SerialOK    bool      `json:"serial_ok"`
	SANsOK      bool      `json:"sans_ok"`
	ValidityOK  bool      `json:"validity_ok"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	CheckedAt   time.Time `json:"checked_at"`
	Error       string    `json:"error,omitempty"`
}
func (v VerificationResult) OK() bool { return v.HandshakeOK && v.SerialOK && v.SANsOK && v.ValidityOK && v.Error == "" }
type Deployment struct {
	ID            int64               `json:"id"`
	RenewalID     int64               `json:"renewal_id"`
	CertificateID int64               `json:"certificate_id"`
	TargetID      int64               `json:"target_id"`
	Status        string              `json:"status"`
	Detail        string              `json:"detail,omitempty"`
	Verification  *VerificationResult `json:"verification,omitempty"`
	Rollback      bool                `json:"rollback"`
	StartedAt     *time.Time          `json:"started_at,omitempty"`
	FinishedAt    *time.Time          `json:"finished_at,omitempty"`
}
