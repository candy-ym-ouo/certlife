package main
import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"certlife/internal/api"
	"certlife/internal/config"
	"certlife/internal/service"
	"certlife/internal/store"
	"certlife/internal/task"
)
func main() {
	configPath := flag.String("config", "config.yaml", "configuration file")
	addr := flag.String("addr", "", "HTTP listen address override")
	apiKey := flag.String("api-key", "", "API key override")
	dbPath := flag.String("db", "", "SQLite database path override")
	flag.Parse()
	cfg, err := config.Load(*configPath)
	if err != nil { slog.Error("load configuration", "error", err); os.Exit(1) }
	if *addr != "" { cfg.Addr = *addr }
	if *apiKey != "" { cfg.APIKey = *apiKey }
	if *dbPath != "" { cfg.DBPath = *dbPath }
	database, err := store.Open(cfg.DBPath)
	if err != nil { slog.Error("open database", "error", err); os.Exit(1) }
	defer database.Close()
	certStore := store.NewCertStore(database)
	renewalStore := store.NewRenewalStore(database)
	deploymentStore := store.NewDeploymentStore(database)
	notificationStore := store.NewNotificationStore(database)
	certService := service.NewCertService(certStore, notificationStore)
	notifyService := service.NewNotifyService(notificationStore, cfg.Notify)
	deployService := service.NewDeployService(deploymentStore, cfg.Deploy.Directory, cfg.Deploy.RollbackOnFail)
	renewalService := service.NewRenewalService(renewalStore, certService, deployService, notifyService, service.MockIssuer{})
	statsService := service.NewStatsService(certStore, renewalStore, notificationStore)
	jobs := task.NewJobs(certService, renewalService, notifyService, statsService)
	scheduler := task.NewScheduler()
	must(scheduler.Register(task.Job{Name: "expiry_checker", Interval: cfg.Schedule.ExpiryChecker.Duration, Run: jobs.ExpiryChecker}))
	must(scheduler.Register(task.Job{Name: "renewal_runner", Interval: cfg.Schedule.RenewalRunner.Duration, Run: jobs.RenewalRunner}))
	must(scheduler.Register(task.Job{Name: "deploy_verifier", Interval: cfg.Schedule.DeployVerifier.Duration, Run: jobs.DeployVerifier}))
	must(scheduler.Register(task.Job{Name: "notify_dispatcher", Interval: cfg.Schedule.NotifyDispatcher.Duration, Run: jobs.NotifyDispatcher}))
	must(scheduler.Register(task.Job{Name: "daily_summary", Interval: cfg.Schedule.DailySummary.Duration, Run: jobs.DailySummary}))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	scheduler.Start(ctx)
	server := &http.Server{Addr: cfg.Addr, Handler: api.New(cfg.APIKey, database, certService, renewalService, deployService, notifyService, statsService, scheduler), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		slog.Info("CertLife listening", "addr", cfg.Addr, "db", cfg.DBPath)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed { slog.Error("HTTP server stopped", "error", err); cancel() }
	}()
	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	_ = scheduler.Stop(shutdownCtx)
	slog.Info("CertLife stopped")
}
func must(err error) {
	if err != nil { panic(err) }
}
