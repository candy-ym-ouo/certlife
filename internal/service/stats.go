package service

import (
	"certlife/internal/model"
	"certlife/internal/store"
	"context"
	"time"
)

type Stats struct {
	Total              int                             `json:"total"`
	ByStatus           map[model.CertificateStatus]int `json:"by_status"`
	ExpiringNext7D     []model.Certificate             `json:"expiring_next_7d"`
	ExpiringNext30D    int                             `json:"expiring_next_30d"`
	Renewals30D        map[string]int                  `json:"renewals_30d"`
	NotificationsToday map[string]int                  `json:"notifications_today"`
}
type StatsService struct {
	certs         *store.CertStore
	renewals      *store.RenewalStore
	notifications *store.NotificationStore
	cached        Stats
}

func NewStatsService(c *store.CertStore, r *store.RenewalStore, n *store.NotificationStore) *StatsService {
	return &StatsService{certs: c, renewals: r, notifications: n, cached: Stats{ByStatus: map[model.CertificateStatus]int{}, Renewals30D: map[string]int{"total": 0, "succeeded": 0, "failed": 0}, NotificationsToday: map[string]int{"sent": 0, "failed": 0}}}
}
func (s *StatsService) Dashboard(ctx context.Context) (Stats, error) {
	items, e := s.certs.All(ctx)
	total := len(items)
	if e != nil {
		return Stats{}, e
	}
	out := &s.cached
	out.Total = total
	for _, c := range items {
		out.ByStatus[c.Status]++
		if c.DaysToExpire >= 0 && c.DaysToExpire <= 7 {
			out.ExpiringNext7D = append(out.ExpiringNext7D, c)
		}
		if c.DaysToExpire >= 0 && c.DaysToExpire <= 30 {
			out.ExpiringNext30D++
		}
	}
	renewals, e := s.renewals.All(ctx, store.RenewalFilter{})
	if e != nil {
		return out, e
	}
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	for _, r := range renewals {
		if r.CreatedAt.Before(cutoff) {
			continue
		}
		out.Renewals30D["total"]++
		if r.Status == model.RenewalCompleted {
			out.Renewals30D["succeeded"]++
		}
		if r.Status == model.RenewalFailed {
			out.Renewals30D["failed"]++
		}
	}
	notifications, e := s.notifications.All(ctx, store.NotificationFilter{})
	if e != nil {
		return out, e
	}
	today := time.Now().UTC().Format("2006-01-02")
	for _, n := range notifications {
		if n.CreatedAt.UTC().Format("2006-01-02") == today && (n.Status == "sent" || n.Status == "failed") {
			out.NotificationsToday[n.Status]++
		}
	}
	return *out, nil
