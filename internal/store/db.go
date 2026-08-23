// Package store owns SQLite persistence and schema migrations.
package store
import (
	"context"
	"database/sql"
	"fmt"
	"time"
	_ "modernc.org/sqlite"
)
type DB struct{ *sql.DB }
type migration struct { version   int; name, sql string }
var migrations = []migration{
	{1, "init_schema", `
CREATE TABLE IF NOT EXISTS certificates(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL,domain TEXT NOT NULL,sans TEXT NOT NULL DEFAULT '[]',issuer TEXT NOT NULL,serial_number TEXT NOT NULL,algorithm TEXT NOT NULL,key_bits INTEGER,valid_from TEXT NOT NULL,valid_until TEXT NOT NULL,owner TEXT,environment TEXT NOT NULL DEFAULT 'prod',tags TEXT NOT NULL DEFAULT '[]',contacts TEXT NOT NULL DEFAULT '[]',notify_threshold_days INTEGER NOT NULL DEFAULT 30,auto_renew INTEGER NOT NULL DEFAULT 0,renew_threshold_days INTEGER NOT NULL DEFAULT 14,deploy_target_ids TEXT NOT NULL DEFAULT '[]',status TEXT NOT NULL DEFAULT 'valid',days_to_expire INTEGER NOT NULL DEFAULT 0,last_checked_at TEXT,created_at TEXT NOT NULL,updated_at TEXT NOT NULL,deleted_at TEXT);
CREATE INDEX IF NOT EXISTS idx_certs_status ON certificates(status); CREATE INDEX IF NOT EXISTS idx_certs_domain ON certificates(domain); CREATE INDEX IF NOT EXISTS idx_certs_environment ON certificates(environment);
CREATE TABLE IF NOT EXISTS deployment_targets(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL,host TEXT NOT NULL,port INTEGER NOT NULL DEFAULT 443,scheme TEXT NOT NULL DEFAULT 'https',method TEXT NOT NULL DEFAULT 'tls',verify_tls INTEGER NOT NULL DEFAULT 1,enabled INTEGER NOT NULL DEFAULT 1,created_at TEXT NOT NULL,updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS renewals(id INTEGER PRIMARY KEY AUTOINCREMENT,certificate_id INTEGER NOT NULL,triggered_by TEXT NOT NULL,status TEXT NOT NULL,old_serial TEXT,new_serial TEXT,attempt INTEGER NOT NULL DEFAULT 1,error TEXT,started_at TEXT,finished_at TEXT,created_at TEXT NOT NULL,FOREIGN KEY(certificate_id) REFERENCES certificates(id));
CREATE INDEX IF NOT EXISTS idx_renewals_cert ON renewals(certificate_id); CREATE INDEX IF NOT EXISTS idx_renewals_status ON renewals(status);
CREATE TABLE IF NOT EXISTS deployments(id INTEGER PRIMARY KEY AUTOINCREMENT,renewal_id INTEGER NOT NULL,certificate_id INTEGER NOT NULL,target_id INTEGER NOT NULL,status TEXT NOT NULL,detail TEXT,verification TEXT,rollback INTEGER NOT NULL DEFAULT 0,started_at TEXT,finished_at TEXT,FOREIGN KEY(renewal_id) REFERENCES renewals(id),FOREIGN KEY(target_id) REFERENCES deployment_targets(id));
CREATE INDEX IF NOT EXISTS idx_deploy_renewal ON deployments(renewal_id); CREATE INDEX IF NOT EXISTS idx_deploy_cert ON deployments(certificate_id);`},
	{2, "notifications", `CREATE TABLE IF NOT EXISTS notifications(id INTEGER PRIMARY KEY AUTOINCREMENT,certificate_id INTEGER,event_type TEXT NOT NULL,channel TEXT NOT NULL,recipient TEXT NOT NULL,subject TEXT NOT NULL,body TEXT NOT NULL,status TEXT NOT NULL,retry_count INTEGER NOT NULL DEFAULT 0,last_error TEXT,sent_at TEXT,created_at TEXT NOT NULL); CREATE UNIQUE INDEX IF NOT EXISTS idx_notify_dedupe ON notifications(IFNULL(certificate_id,0),event_type,channel,recipient,date(created_at));`},
	{3, "notification_rules", `CREATE TABLE IF NOT EXISTS notification_rules(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL,event_types TEXT NOT NULL DEFAULT '[]',environments TEXT NOT NULL DEFAULT '[]',channels TEXT NOT NULL DEFAULT '["email"]',recipients TEXT NOT NULL DEFAULT '[]',webhook_url TEXT,enabled INTEGER NOT NULL DEFAULT 1,created_at TEXT NOT NULL);`},
	{4, "audit_logs", `CREATE TABLE IF NOT EXISTS audit_logs(id INTEGER PRIMARY KEY AUTOINCREMENT,actor TEXT NOT NULL,action TEXT NOT NULL,resource_type TEXT NOT NULL,resource_id TEXT,detail TEXT,created_at TEXT NOT NULL); CREATE INDEX IF NOT EXISTS idx_audit_resource ON audit_logs(resource_type,resource_id);`},
	{5, "schema_migrations_meta", `SELECT 1;`},
	{6, "seed_demo", `INSERT INTO deployment_targets(name,host,port,scheme,method,verify_tls,enabled,created_at,updated_at) SELECT 'local-static','localhost',8443,'https','static',0,1,datetime('now'),datetime('now') WHERE NOT EXISTS(SELECT 1 FROM deployment_targets); INSERT INTO notification_rules(name,event_types,environments,channels,recipients,webhook_url,enabled,created_at) SELECT 'default-log','[]','[]','["email"]','["ops@example.com"]','',1,datetime('now') WHERE NOT EXISTS(SELECT 1 FROM notification_rules);`},
	{7, "unique_active_renewal", `UPDATE renewals SET status='failed',error='superseded by newer active renewal',finished_at=datetime('now') WHERE status IN ('pending','issuing','issued','deploying','verifying') AND id NOT IN (SELECT MAX(id) FROM renewals WHERE status IN ('pending','issuing','issued','deploying','verifying') GROUP BY certificate_id); CREATE UNIQUE INDEX IF NOT EXISTS idx_renewals_one_active ON renewals(certificate_id) WHERE status IN ('pending','issuing','issued','deploying','verifying');`},
	{8, "notification_retry_backoff", `ALTER TABLE notifications ADD COLUMN last_attempt_at TEXT;`},
	{9, "notification_rolling_dedupe", `DROP INDEX IF EXISTS idx_notify_dedupe; CREATE INDEX IF NOT EXISTS idx_notify_lookup ON notifications(IFNULL(certificate_id,0),event_type,channel,recipient,created_at); CREATE TRIGGER IF NOT EXISTS trg_notify_dedupe BEFORE INSERT ON notifications WHEN NEW.event_type<>'daily_summary' AND EXISTS(SELECT 1 FROM notifications WHERE IFNULL(certificate_id,0)=IFNULL(NEW.certificate_id,0) AND event_type=NEW.event_type AND channel=NEW.channel AND recipient=NEW.recipient AND datetime(created_at)>datetime(NEW.created_at,'-24 hours')) BEGIN SELECT RAISE(IGNORE); END;`},
}
func Open(path string) (*DB, error) {
	dsn := path
	if path == ":memory:" { dsn = fmt.Sprintf("file:certlife-%d?mode=memory&cache=shared", time.Now().UnixNano()) }
	db, err := sql.Open("sqlite", dsn)
	if err != nil { return nil, err }
	db.SetMaxOpenConns(1)
	store := &DB{db}
	if _, err = db.Exec(`PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`); err != nil { db.Close(); return nil, err }
	if err = store.Migrate(context.Background()); err != nil { db.Close(); return nil, err }
	return store, nil
}
func (db *DB) Migrate(ctx context.Context) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY,name TEXT NOT NULL,applied_at TEXT NOT NULL)`); err != nil { return err }
	for _, item := range migrations {
		var exists int
		err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM schema_migrations WHERE version=?`, item.version).Scan(&exists)
		if err != nil { return err }
		if exists > 0 { continue }
		tx, err := db.BeginTx(ctx, nil)
		if err != nil { return err }
		if _, err = tx.ExecContext(ctx, item.sql); err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version,name,applied_at) VALUES(?,?,?)`, item.version, item.name, time.Now().UTC().Format(time.RFC3339Nano))
		}
		if err == nil { _, err = tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", item.version)) }
		if err != nil { tx.Rollback(); return fmt.Errorf("migration %d %s: %w", item.version, item.name, err) }
		if err = tx.Commit(); err != nil { return err }
	}
	return nil
}
func (db *DB) PingContext(ctx context.Context) error { return db.DB.PingContext(ctx) }
