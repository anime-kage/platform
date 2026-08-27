--: per-series translation glossary. Injected into the cached prompt
-- prefix of every auto-translate window for the series — name spellings,
-- honorific choices, recurring-term decisions. Edited by coordinators (5.x).
ALTER TABLE public.anime ADD COLUMN translation_glossary text;
