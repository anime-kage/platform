-- chapters.chapter_number was numeric(5,2), i.e. a hard ceiling of 999.99.
--
-- That is not a theoretical limit: One Piece is past chapter 1180, Detective
-- Conan past 1120, and a bulk manga import silently loses every chapter at or
-- above 1000 — the rows simply cannot be inserted. numeric(7,2) takes it to
-- 99,999.99, which no serialised manga is going to reach.
--
-- The two decimal places stay: half-chapters (10.5) are real and the reader
-- already renders them.
ALTER TABLE public.chapters
  ALTER COLUMN chapter_number TYPE numeric(7,2);
