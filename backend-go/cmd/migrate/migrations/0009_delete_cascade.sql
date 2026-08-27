--: deleting a title from the admin panel must take its episodes/
-- chapters and their links with it. These four FKs were the only content FKs
-- without ON DELETE CASCADE (a latent bug: even the existing episode delete
-- failed once the episode had links).

ALTER TABLE public.episodes
  DROP CONSTRAINT episodes_anime_id_anime_id_fk,
  ADD CONSTRAINT episodes_anime_id_anime_id_fk
    FOREIGN KEY (anime_id) REFERENCES public.anime(id) ON DELETE CASCADE;

ALTER TABLE public.chapters
  DROP CONSTRAINT chapters_manga_id_manga_id_fk,
  ADD CONSTRAINT chapters_manga_id_manga_id_fk
    FOREIGN KEY (manga_id) REFERENCES public.manga(id) ON DELETE CASCADE;

ALTER TABLE public.content_links
  DROP CONSTRAINT content_links_episode_id_episodes_id_fk,
  ADD CONSTRAINT content_links_episode_id_episodes_id_fk
    FOREIGN KEY (episode_id) REFERENCES public.episodes(id) ON DELETE CASCADE,
  DROP CONSTRAINT content_links_chapter_id_chapters_id_fk,
  ADD CONSTRAINT content_links_chapter_id_chapters_id_fk
    FOREIGN KEY (chapter_id) REFERENCES public.chapters(id) ON DELETE CASCADE;
