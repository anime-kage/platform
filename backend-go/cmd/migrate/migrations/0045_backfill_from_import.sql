-- Backfill from_import for lists imported before 0044 added the column.
--
-- There is no record of which rows an import wrote, so this uses the one
-- signature a bulk import leaves and a human cannot: many rows for the same
-- member landing in the same minute. Measured on the live data the split is
-- not close — the import is 159 rows in one minute, while genuine activity is
-- one row per minute per member — so a threshold of 10 cannot catch a person
-- updating their list by hand, however fast they click.
--
-- No-op on a fresh database, and idempotent: rows already true stay true.
WITH bulk AS (
  SELECT user_id, date_trunc('minute', updated_at) AS bucket
  FROM watchlist GROUP BY 1, 2 HAVING count(*) >= 10
)
UPDATE watchlist w SET from_import = true
FROM bulk b
WHERE w.user_id = b.user_id
  AND date_trunc('minute', w.updated_at) = b.bucket
  AND w.from_import = false;

WITH bulk AS (
  SELECT user_id, date_trunc('minute', updated_at) AS bucket
  FROM readlist GROUP BY 1, 2 HAVING count(*) >= 10
)
UPDATE readlist r SET from_import = true
FROM bulk b
WHERE r.user_id = b.user_id
  AND date_trunc('minute', r.updated_at) = b.bucket
  AND r.from_import = false;
