import type { PageServerLoad } from './$types';
import {
  listAllAnime,
  listAllManga,
  listPublicUserLists,
  leaderboardAnime,
  curatedSlots
} from '$lib/server/api';
import { PLATFORM_LISTS, platformItems } from '$lib/data/platformLists';

export const load: PageServerLoad = async ({ fetch }) => {
  const [byScoreAll, byScoreMangaAll, watched, publicLists, curated] = await Promise.all([
    listAllAnime(fetch, 'score'),
    listAllManga(fetch, 'score'),
    leaderboardAnime(fetch, 'all', 50),
    listPublicUserLists(fetch),
    curatedSlots(fetch)
  ]);
  const byScore = byScoreAll.filter((a) => a.imageUrl);
  const byScoreManga = byScoreMangaAll.filter((m) => m.imageUrl);
  const byWatchers = watched.filter((a) => a.imageUrl);

  // Platform-curated lists (Top 50 MAL, cele mai urmărite, top pe gen) — live.
  const platform = PLATFORM_LISTS.map((def) => {
    const items = platformItems(def, byScore, byScoreManga, byWatchers);
    return {
      slug: `top/${def.slug}`,
      title: def.title,
      desc: def.desc,
      count: items.length,
      covers: items.slice(0, 5).map((a) => a.imageUrl as string)
    };
  }).filter((l) => l.covers.length >= 3);

  // The featured slot ("Listă remarcată"). It used to be whichever seeded
  // editorial list had the highest invented like count -- a placeholder with a
  // fabricated author and fabricated engagement. It now points at a real,
  // computed platform list; the Vitrină will make it choosable.
  // An editor's choice from the Vitrină wins. Falling back to a computed
  // platform list means the block is never empty and never invents an author.
  const pickedList = curated['liste_featured']?.[0]?.list ?? null;
  const featuredDef = platform.find((l) => l.slug === 'top/top-anime') ?? platform[0] ?? null;
  const featured = pickedList
    ? {
        slug: String(pickedList.id),
        title: pickedList.title,
        desc: pickedList.description ?? '',
        count: pickedList.itemCount,
        covers: pickedList.covers,
        likes: pickedList.likeCount,
        comments: 0,
        authorName: pickedList.ownerName,
        authorAvatarUrl: pickedList.ownerAvatarUrl ?? '',
        authorHue: (pickedList.userId * 47) % 360
      }
    : featuredDef
    ? {
        slug: featuredDef.slug,
        title: featuredDef.title,
        desc: featuredDef.desc,
        count: featuredDef.count,
        covers: featuredDef.covers,
        likes: 0,
        comments: 0,
        // Platform lists have no member author -- the template omits the
        // byline rather than inventing one.
        authorName: '',
        authorAvatarUrl: '',
        authorHue: 0
      }
    : null;

  // real members' public lists — already sorted most-liked-first by the API
  const community = publicLists.map((l) => ({
    slug: String(l.id),
    title: l.title,
    desc: l.description ?? '',
    likes: l.likeCount,
    comments: 0,
    count: l.itemCount,
    authorName: l.ownerName,
    authorAvatarUrl: l.ownerAvatarUrl ?? '',
    authorHue: (l.userId * 47) % 360,
    covers: l.covers
  }));

  return { platform, lists: community, featured };
};
