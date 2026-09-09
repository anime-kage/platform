-- Let someone dismiss a series from "Continuă vizionarea".
--
-- Deliberately not a watchlist status. Hiding a row from the home page and
-- saying "I dropped this" are different statements: a viewer may want the
-- series out of the way while still counting it as watching, and Netflix draws
-- the same distinction. Folding the two together would make the shelf edit
-- someone's list behind their back.
--
-- hidden_at is a timestamp rather than a boolean so the dismissal expires by
-- itself: the row is hidden only while nothing newer has happened. Watch
-- another episode and last_activity moves past hidden_at, and the series comes
-- back without anyone having to un-hide it.
CREATE TABLE continue_hidden (
  user_id   integer     NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
  anime_id  integer     NOT NULL REFERENCES anime(id)  ON DELETE CASCADE,
  hidden_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, anime_id)
);
