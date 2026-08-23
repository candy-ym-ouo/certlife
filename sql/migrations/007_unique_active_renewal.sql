-- Keep the newest active workflow for each certificate before enforcing the invariant.
UPDATE renewals
SET status = 'failed',
    error = 'superseded by newer active renewal',
    finished_at = datetime('now')
WHERE status IN ('pending','issuing','issued','deploying','verifying')
  AND id NOT IN (
    SELECT MAX(id)
    FROM renewals
    WHERE status IN ('pending','issuing','issued','deploying','verifying')
    GROUP BY certificate_id
  );

CREATE UNIQUE INDEX IF NOT EXISTS idx_renewals_one_active
ON renewals(certificate_id)
WHERE status IN ('pending','issuing','issued','deploying','verifying');
