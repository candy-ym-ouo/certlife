package model
import "time"
type RenewalStatus string
const (
	RenewalPending   RenewalStatus = "pending"
	RenewalIssuing   RenewalStatus = "issuing"
	RenewalIssued    RenewalStatus = "issued"
	RenewalDeploying RenewalStatus = "deploying"
	RenewalVerifying RenewalStatus = "verifying"
	RenewalCompleted RenewalStatus = "completed"
	RenewalFailed    RenewalStatus = "failed"
)
type Renewal struct {
	ID            int64         `json:"id"`
	CertificateID int64         `json:"certificate_id"`
	TriggeredBy   string        `json:"triggered_by"`
	Status        RenewalStatus `json:"status"`
	OldSerial     string        `json:"old_serial,omitempty"`
	NewSerial     string        `json:"new_serial,omitempty"`
	Attempt       int           `json:"attempt"`
	Error         string        `json:"error,omitempty"`
	StartedAt     *time.Time    `json:"started_at,omitempty"`
	FinishedAt    *time.Time    `json:"finished_at,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
}
func (r Renewal) Active() bool {
	switch r.Status { case RenewalPending, RenewalIssuing, RenewalIssued, RenewalDeploying, RenewalVerifying:; return true; default:; return false }
}
func CanTransition(from, to RenewalStatus) bool {
	allowed := map[RenewalStatus][]RenewalStatus{
		RenewalPending:   {RenewalIssuing, RenewalFailed},
		RenewalIssuing:   {RenewalIssued, RenewalFailed},
		RenewalIssued:    {RenewalDeploying, RenewalFailed},
		RenewalDeploying: {RenewalVerifying, RenewalFailed},
		RenewalVerifying: {RenewalCompleted, RenewalFailed},
		RenewalFailed:    {RenewalPending},
	}
	for _, candidate := range allowed[from] {
		if candidate == to { return true }
	}
	return false
}
