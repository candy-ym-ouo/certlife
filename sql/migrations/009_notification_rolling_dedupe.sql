-- Enforce rolling 24-hour deduplication at database level; daily summaries always pass.
DROP INDEX IF EXISTS idx_notify_dedupe;
CREATE INDEX IF NOT EXISTS idx_notify_lookup
ON notifications(IFNULL(certificate_id,0), event_type, channel, recipient, created_at);
CREATE TRIGGER IF NOT EXISTS trg_notify_dedupe
BEFORE INSERT ON notifications
WHEN NEW.event_type <> 'daily_summary' AND EXISTS (
  SELECT 1 FROM notifications
  WHERE IFNULL(certificate_id,0) = IFNULL(NEW.certificate_id,0)
    AND event_type = NEW.event_type AND channel = NEW.channel AND recipient = NEW.recipient
    AND datetime(created_at) > datetime(NEW.created_at, '-24 hours')
)
BEGIN
  SELECT RAISE(IGNORE);
END;
