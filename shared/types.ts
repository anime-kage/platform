// Shared TypeScript types between frontend and backend

export interface FavoriteRef {
  type: 'anime' | 'manga';
  id: number;
}

export interface User {
  id: number;
  username: string;
  email: string;
  avatarUrl?: string;
  bio?: string;
  favoriteGenres: string[];
  /** Letterboxd-style showcase: up to 5 hand-picked titles */
  favorites?: FavoriteRef[];
  role: 'user' | 'translator' | 'verifier' | 'coordinator' | 'moderator' | 'admin';
  createdAt: Date;
  updatedAt: Date;
}

export interface Anime {
  id: number;
  malId?: number;
  title: string;
  titleEnglish?: string;
  titleRomanian?: string;
  synopsis?: string;
  /** auto-translated at import (or hand-written) — shown instead of synopsis when present */
  synopsisRomanian?: string;
  genres: string[];
  studios: string[];
  status: 'airing' | 'completed' | 'upcoming';
  type: 'tv' | 'movie' | 'ova' | 'special';
  episodes?: number;
  score?: number;
  year?: number;
  season?: string;
  imageUrl?: string;
  trailerUrl?: string;
  broadcastDay?: string;
  broadcastTime?: string;
  /** URL slug ("91-days"); absent until the backfill has run, so links must
      fall back to the numeric id. */
  slug?: string;
  createdAt: Date;
  updatedAt: Date;
}

export interface Manga {
  id: number;
  malId?: number;
  title: string;
  titleEnglish?: string;
  titleRomanian?: string;
  synopsis?: string;
  /** auto-translated at import (or hand-written) — shown instead of synopsis when present */
  synopsisRomanian?: string;
  genres: string[];
  authors: string[];
  status: 'publishing' | 'completed' | 'upcoming' | 'hiatus' | 'discontinued';
  type: 'manga' | 'manhwa' | 'manhua' | 'novel';
  chapters?: number;
  volumes?: number;
  score?: number;
  year?: number;
  imageUrl?: string;
  /** URL slug, same contract as Anime.slug */
  slug?: string;
  createdAt: Date;
  updatedAt: Date;
}

export interface Episode {
  id: number;
  animeId: number;
  episodeNumber: number;
  title?: string;
  airDate?: string;
  duration?: number;
  /** per-episode description, written by the team (not from MAL) */
  synopsis?: string;
  /** MAL's filler mark — an episode outside the source material */
  isFiller: boolean;
  /** MAL's recap mark — an episode that only replays earlier ones */
  isRecap: boolean;
  links: ContentLink[];
  createdAt: Date;
}

export interface Chapter {
  id: number;
  mangaId: number;
  chapterNumber: string;
  title?: string;
  releaseDate?: string;
  pages?: number;
  links: ContentLink[];
  createdAt: Date;
}

/**
 * A playback/reading source (PLAN 3.1).
 * kind 'embed' = iframe fallback; 'extract' = resolved to an HLS manifest by
 * the resolver service via provider+providerRef (played in our own player).
 * lastCheckedAt/lastOk are written by the source health checker.
 */
export interface ContentLink {
  id: number;
  episodeId?: number;
  chapterId?: number;
  hostingUrl: string;
  quality?: string;
  language: string;
  isActive: boolean;
  kind: 'embed' | 'extract';
  provider?: string;
  providerRef?: string;
  priority: number;
  lastCheckedAt?: string;
  lastOk?: boolean;
}

/**
 * The stream resolve response (PLAN 3.3): GET /api/episodes/:id/stream.
 * Mirrors backend-go/internal/resolver.Result — the player feeds manifestUrl
 * to HLS.js (kind 'hls') or straight into <video src> (kind 'mp4').
 */
export interface ResolvedStream {
  manifestUrl: string;
  kind: 'hls' | 'mp4';
  headers?: Record<string, string>;
  subtitles?: { url: string; language: string; label?: string }[];
}

export interface StreamSourceInfo {
  id: number;
  provider: string;
  quality?: string;
  language: string;
}

/** Skip intro/outro ranges for an episode (PLAN 3.6), seconds from start. */
export interface SkipRange {
  start: number;
  end: number;
}

export interface EpisodeSkipMarks {
  intro: SkipRange | null;
  outro: SkipRange | null;
}

/**
 * Our own subtitle track (PLAN 3.5). Published tracks are merged into the
 * stream response and rendered as <track> in our player — RO first/default.
 */
export interface Subtitle {
  id: number;
  episodeId: number;
  language: string;
  label?: string;
  format: 'vtt' | 'srt' | 'ass';
  url: string;
  status: 'machine' | 'edited' | 'published';
  translatorId?: number;
  sourceSub?: string;
  createdAt: string;
}

/**
 * A release in the translator pipeline (PLAN 4.1): video + EN sub in staging,
 * RO draft as SubtitleEvent rows. State machine:
 * draft → in_review → approved → published, with changes_requested looping
 * back to the translator.
 */
export interface Release {
  id: number;
  /** anime episode (editor flow) or manga chapter (bring-your-own-pages, 4.6) */
  medium: 'anime' | 'manga';
  /** absent until the series exists in the catalog (see proposedTitle) */
  animeId?: number;
  mangaId?: number;
  /** the translator's free-text series name — the coordinator links the real one at publish */
  proposedTitle?: string;
  /** anime releases only */
  episodeNumber?: number;
  /** manga releases only */
  chapterNumber?: number;
  uploaderId: number;
  reviewerId?: number;
  /** who verification is routed to — filters queues; coordinators/admins can reassign */
  assignedVerifierId?: number;
  state: 'draft' | 'in_review' | 'changes_requested' | 'approved' | 'published';
  reviewNotes?: string;
  createdAt: string;
  updatedAt: string;
  animeTitle?: string;
  animeImage?: string;
  mangaTitle?: string;
  mangaImage?: string;
  uploaderName?: string;
  reviewerName?: string;
  assignedVerifierName?: string;
  /** per-release translation progress, subqueried alongside the row */
  totalEvents: number;
  doneEvents: number;
  hasVideo: boolean;
  /** manga releases: staged page counts (RO edition / EN originals) */
  pageCount?: number;
  enPageCount?: number;
  /** an auto-translate run is currently filling this release's rows */
  translating: boolean;
}

/**
 * One line of a release's subtitle draft. `edited` marks human-touched rows —
 * auto-translate (4.3) only fills rows whose roText is still empty.
 */
export interface SubtitleEvent {
  id: number;
  releaseId: number;
  idx: number;
  startMs: number;
  endMs: number;
  enText: string;
  roText: string;
  edited: boolean;
}

/** Result of the admin "test this source before saving" button (PLAN 5.2). */
export interface TestSourceResult {
  ok: boolean;
  message?: string;
  manifestUrl?: string;
  streamKind?: 'hls' | 'mp4';
}

/** Admin health dashboard payload (PLAN 5.6). */
export interface AdminDeadSource {
  id: number;
  kind: 'embed' | 'extract';
  provider?: string;
  hostingUrl: string;
  quality?: string;
  lastCheckedAt?: string;
  episodeId: number;
  episodeNumber: number;
  animeId: number;
  animeTitle: string;
}

export interface AdminEpisodeGap {
  episodeId: number;
  episodeNumber: number;
  animeId: number;
  animeTitle: string;
}

export interface AdminHealthReport {
  deadSources: AdminDeadSource[];
  missingSource: { total: number; episodes: AdminEpisodeGap[] };
  missingRoSub: { total: number; episodes: AdminEpisodeGap[] };
}

/**
 * A chapter's page images for the native reader (PLAN 6.1). `language` is the
 * edition actually returned (RO default, falls back to what exists);
 * `languages` lists every available edition.
 */
export interface ChapterPages {
  language: string;
  languages: string[];
  pages: string[];
}

/** One row of the moderation queue (PLAN 5.5). */
export interface AdminReportedComment {
  id: number;
  content: string;
  createdAt: string;
  userId: number;
  username: string;
  userRole: string;
  userBanned: boolean;
  animeId?: number;
  mangaId?: number;
  contextTitle?: string;
}

/** A row of the admin user manager. */
export interface AdminUser {
  id: number;
  username: string;
  role: string;
  isBanned: boolean;
}

/** A row of the admin team tab: everyone holding a team role. */
export interface TeamMember extends AdminUser {
  createdAt: string;
  /** Per-user in-flight release cap; null = the server-wide default. */
  releaseCap: number | null;
  /** Unpublished anime releases they hold right now. */
  inFlight: number;
}

/** A MyAnimeList search hit (publish page) — nothing imported yet. */
/** One person a release's verification can be routed to. */
export interface VerifierOption {
  id: number;
  username: string;
  role: string;
}

/** A curated custom list (the /liste feature) — distinct from watchlist/readlist. */
export interface UserList {
  id: number;
  userId: number;
  title: string;
  description?: string;
  isPublic: boolean;
  createdAt: string;
  updatedAt: string;
  ownerName: string;
  /** owner's avatar, joined so a list card can draw a real byline */
  ownerAvatarUrl?: string;
  itemCount: number;
  /** first item posters, for the card fans */
  covers: string[];
  likeCount: number;
  /** whether the requesting viewer has liked this list */
  liked: boolean;
}

/** One title on a custom list — exactly one of animeId/mangaId is set. */
export interface UserListItem {
  id: number;
  listId: number;
  animeId?: number;
  mangaId?: number;
  note?: string;
  position: number;
  addedAt: string;
  title: string;
  titleRomanian?: string;
  imageUrl?: string;
  year?: number;
  score?: number;
  genres: string[];
}

export interface MalSearchHit {
  malId: number;
  title: string;
  type: string;
  year?: number;
  episodes?: number;
  /** manga hits carry chapters instead of episodes */
  chapters?: number;
  imageUrl?: string;
}

export type RequestStatusKey = 'pending' | 'in_progress' | 'approved' | 'rejected';

/** A member's "Cereri" ask for a series to be subtitled, with its vote tally. */
export interface TranslationRequest {
  id: number;
  userId: number;
  medium: 'anime' | 'manga';
  malId?: number;
  title: string;
  imageUrl?: string;
  note?: string;
  status: RequestStatusKey;
  createdAt: string;
  updatedAt: string;
  requesterName: string;
  voteCount: number;
  /** whether the requesting viewer has voted */
  voted: boolean;
}

/** The admin panel's stat strip: catalog and team headcounts. */
export interface AdminOverview {
  animeCount: number;
  mangaCount: number;
  teamCount: number;
}

export interface WatchlistEntry {
  id: number;
  userId: number;
  animeId: number;
  status: 'watching' | 'completed' | 'on-hold' | 'dropped' | 'plan-to-watch';
  score?: number;
  episodesWatched: number;
  notes?: string;
  startedAt?: Date;
  completedAt?: Date;
  updatedAt: Date;
  anime: Anime;
  /**
   * What the platform actually has, as opposed to `anime.episodes` — that is
   * the series total from MAL and counts episodes nobody has uploaded here
   * yet. `availableEpisodes` only counts episodes with a playable source, and
   * `nextEpisode` is the lowest one still ahead of the viewer (absent when
   * they're caught up on everything we have).
   */
  availableEpisodes: number;
  nextEpisode?: number;
}

/**
 * One card in the "Continuă vizionarea" row. Driven by playback, not by the
 * watchlist: it answers "what do I press play on", so a series you never
 * explicitly added still shows up once you've watched any of it.
 *
 * `positionS` is where to resume inside `episodeNumber` — 0 means the episode
 * is fresh and should start from the beginning.
 */
export interface ContinueEntry {
  anime: Anime;
  episodeId: number;
  episodeNumber: number;
  positionS: number;
  durationS?: number;
  availableEpisodes: number;
  watchedEpisodes: number;
  lastActivity: string;
}

export interface ReadlistEntry {
  id: number;
  userId: number;
  mangaId: number;
  status: 'reading' | 'completed' | 'on-hold' | 'dropped' | 'plan-to-read';
  score?: number;
  chaptersRead: number;
  volumesRead: number;
  notes?: string;
  startedAt?: Date;
  completedAt?: Date;
  updatedAt: Date;
  manga: Manga;
}

export interface Comment {
  id: number;
  userId: number;
  animeId?: number;
  mangaId?: number;
  /** set when the comment belongs to one episode/chapter, not the whole series */
  episodeId?: number;
  chapterId?: number;
  parentId?: number;
  /** top-level comment of the thread this reply belongs to */
  rootId?: number;
  /** who this reply answers, when it isn't the thread root */
  replyToUsername?: string;
  /** short quote of the answered message, for "in reply to" UI */
  replyToExcerpt?: string;
  content: string;
  likesCount: number;
  dislikesCount: number;
  repliesCount: number;
  userVote?: 'like' | 'dislike' | null;
  createdAt: Date;
  updatedAt: Date;
  user: Pick<User, 'id' | 'username' | 'avatarUrl'>;
  replies?: Comment[];
}

/** A review = a watchlist/readlist entry where the user wrote something */
export interface Review {
  /** the watchlist/readlist entry id — used as reviewId for reply threads */
  entryId: number;
  userId: number;
  score?: number;
  notes: string;
  updatedAt: Date;
  replyCount: number;
  user: Pick<User, 'id' | 'username' | 'avatarUrl'>;
}

/** Follower/following counts on a public profile (+ viewer's relation to it) */
export interface FollowNetwork {
  followers: number;
  following: number;
  isFollowing: boolean;
}

/** One row in a followers/following list */
export interface FollowUser {
  id: number;
  username: string;
  bio?: string;
  avatarUrl?: string;
  role: string;
  followersCount: number;
  /** whether the VIEWER follows this user (false for guests) */
  isFollowing: boolean;
}

/** A review as it appears on the author's profile (anime or manga) */
export interface UserReview {
  kind: 'anime' | 'manga';
  entryId: number;
  score?: number;
  notes: string;
  updatedAt: Date;
  replyCount: number;
  title: {
    id: number;
    title: string;
    titleRomanian?: string;
    imageUrl?: string;
    year?: number;
  };
}

// ── Community (/comunitate) ──────────────────────────────────────────────────

/** A real account on the Members tab, with the counts that matter socially. */
export interface CommunityMember {
  id: number;
  username: string;
  avatarUrl?: string;
  bio?: string;
  role: string;
  followers: number;
  reviewCount: number;
  listCount: number;
  isFollowing: boolean;
}

/** A member's review (list entry with notes) as it shows on the Reviews tab. */
export interface CommunityReview {
  kind: 'anime' | 'manga';
  entryId: number;
  score?: number;
  notes: string;
  updatedAt: string;
  replyCount: number;
  user: Pick<User, 'id' | 'username' | 'avatarUrl'>;
  title: { id: number; title: string; titleRomanian?: string; imageUrl?: string; year?: number };
}

/** One line of the friends-only activity feed. */
export interface ActivityEvent {
  type: 'review' | 'list' | 'thread';
  user: Pick<User, 'id' | 'username' | 'avatarUrl'>;
  verb: string;
  target: string;
  link?: string;
  meta?: string;
  createdAt: string;
}

export interface CommunityStats {
  members: number;
  subtitles: number;
  titles: number;
  /** Members seen in the last 5 minutes (users.last_seen_at, touched by the
   *  auth middleware). Shown instead of `subtitles` on the community page. */
  online: number;
}

/** One person's standing on the translator/verifier hall of fame. */
export interface HallEntry {
  userId: number;
  username: string;
  avatarUrl?: string;
  role: string;
  count: number;
}

/** The two hall-of-fame leaderboards for a time window. */
export interface HallOfFame {
  translators: HallEntry[];
  verifiers: HallEntry[];
  window: 'month' | 'all';
}

/** The three people behind a published episode/chapter: who translated it,
    who verified it, and the coordinator who published it. Any may be null —
    nothing published there yet, no recorded reviewer, or a release published
    before `published_by` existed (PLAN 8.20; deliberately not backfilled). */
export interface ReleaseCredits {
  translator: Pick<User, 'id' | 'username' | 'avatarUrl'> | null;
  verifier: Pick<User, 'id' | 'username' | 'avatarUrl'> | null;
  coordinator: Pick<User, 'id' | 'username' | 'avatarUrl'> | null;
}

/** A staff account on the community Echipă tab (distinct from the admin
    TeamMember above — this one carries the public profile bits). */
export interface CommunityTeamMember {
  id: number;
  username: string;
  avatarUrl?: string;
  bio?: string;
  role: string;
  createdAt: string;
}

export interface ForumThread {
  id: number;
  category: string;
  title: string;
  body?: string;
  isPinned: boolean;
  isLocked: boolean;
  replyCount: number;
  lastActivityAt: string;
  createdAt: string;
  author: Pick<User, 'id' | 'username' | 'avatarUrl'>;
}

export interface ForumReply {
  id: number;
  body: string;
  createdAt: string;
  author: Pick<User, 'id' | 'username' | 'avatarUrl'>;
}

/**
 * One entry of the team-decided weekly programme ("Programul săptămânii"):
 * this episode of this series goes live at this instant. `scheduledAt` is a
 * real instant (RFC3339 with offset), so it renders in each viewer's own
 * timezone — unlike the MAL broadcast day/time it replaced, which could only
 * be shown as "23:30 JST".
 */
/** A custom chat emote uploaded by the team. Rendered at a fixed height in
    chat, so `width`/`height` are the source image's own size, not the display
    size — they exist so the admin list can flag awkward proportions. */
export interface Emote {
  id: number;
  code: string;
  imageUrl: string;
  width: number;
  height: number;
  isActive: boolean;
  createdAt: string;
}

export interface ScheduleSlot {
  id: number;
  animeId: number;
  episodeNumber: number;
  scheduledAt: string;
  note?: string;
  createdByName?: string;
  title: string;
  titleEnglish?: string;
  titleRomanian?: string;
  imageUrl?: string;
  /** the episode already exists on the site, so a card can link to the player */
  published: boolean;
}

/**
 * A neighbouring series: the next season, the previous one, or an alternative
 * retelling. Only titles that exist in the catalog are ever returned, so every
 * card links somewhere real. `relation` is '' for the anime at the centre of
 * its own chain.
 */
export interface RelatedAnime {
  relation: 'SEQUEL' | 'PREQUEL' | 'ALTERNATIVE' | 'SIDE_STORY' | 'PARENT' | 'SPIN_OFF' | 'SUMMARY' | '';
  id: number;
  title: string;
  titleEnglish?: string;
  titleRomanian?: string;
  imageUrl?: string;
  year?: number;
  type: string;
  status: string;
  episodes?: number;
  /** so links go straight to the canonical URL, not through the id redirect */
  slug?: string;
  /** episodes we actually host — 0 means a link there has nothing to play */
  episodeCount: number;
}

/** `chain` is the prequel→sequel run in watch order (empty when the series
    stands alone); `related` is everything that doesn't linearise. */
export interface AnimeRelations {
  chain: RelatedAnime[];
  related: RelatedAnime[];
}

/** One entry of the "Știri & anunțuri" strip on /home, written by the team. */
export interface Announcement {
  id: number;
  /** short accent label, and the filter on /anunturi */
  tag: string;
  title: string;
  /** the post body — a Markdown subset, rendered by lib/components/Markdown.svelte */
  body?: string;
  /** wide image at the top of the post; the home card stays text-only */
  coverUrl?: string;
  /** shareable URL segment; falls back to the id when absent */
  slug?: string;
  commentCount: number;
  /** internal path or https URL the card links to */
  url?: string;
  isPublished: boolean;
  authorName?: string;
  createdAt: string;
  updatedAt: string;
}

/** Per-day totals from watch_history */
export interface HistoryDay {
  date: string; // YYYY-MM-DD
  episodes: number;
  chapters: number;
}

export interface UserStats {
  totalAnimeWatched: number;
  totalEpisodesWatched: number;
  totalHoursWatched: number;
  totalMangaRead: number;
  totalChaptersRead: number;
  averageAnimeScore: number;
  averageMangaScore: number;
}

export interface ApiError {
  error: string;
  message: string;
  statusCode: number;
  timestamp: string;
}

// API Response types
export interface PaginatedResponse<T> {
  data: T[];
  pagination: {
    page: number;
    limit: number;
    total: number;
    totalPages: number;
  };
}

export interface AuthResponse {
  user: User;
  token: string;
  message?: string;
}

// Form types
export interface LoginForm {
  email: string;
  password: string;
}

export interface RegisterForm {
  username: string;
  email: string;
  password: string;
  confirmPassword: string;
  /** Single-use code from the Discord bot; required when the server runs
   *  with INVITE_ONLY (PLAN 9.7), ignored otherwise. */
  inviteCode?: string;
}

/** Server switches the client must know before rendering (GET /api/config). */
export interface PublicConfig {
  inviteOnly: boolean;
}

/* ── Curated placements (PLAN 5.8) ──────────────────────────────────────── */

/** One editorially chosen title. Exactly one of anime/manga is set. */
export interface CuratedPick {
  position: number;
  anime?: Anime;
  manga?: Manga;
  /** Set for slots that feature a member's list rather than a title. */
  list?: UserList;
  /** Artwork for this placement only — the series poster is never changed. */
  imageUrl?: string;
}

/** What an editor submits — a reference, before the server resolves it. */
export interface CuratedRef {
  /** 'list' is for slots that feature a member's collection instead of a title. */
  mediaType: 'anime' | 'manga' | 'list';
  id: number;
  /** Sent back on every save: a slot write is a full replace, so omitting
   *  this would drop the artwork when reordering. */
  imageUrl?: string;
}

/** A placement's rules, served by the API so the admin UI can't drift. */
export interface CuratedSlotDef {
  key: string;
  label: string;
  hint: string;
  /** how many titles the placement shows */
  max: number;
  /** 'anime' | 'manga', or '' when either kind is allowed */
  media: string;
}

export type CuratedSlots = Record<string, CuratedPick[]>;

/* ── List import + profile banner (PLAN 8.16 / 8.17) ────────────────────── */

/** What one import did. `skipped` = titles we have no catalog entry for. */
export interface ImportResult {
  imported: number;
  updated: number;
  skipped: number;
  /** a short sample of titles we don't carry, to explain a thin import */
  unmatched: string[];
}

/** Keyed by media kind; a MAL export only ever fills one of the two. */
export interface ImportReport {
  anime?: ImportResult;
  manga?: ImportResult;
}

/** The series a member chose as their profile backdrop. */
export interface ProfileBanner {
  mediaType: 'anime' | 'manga';
  id: number;
  title: string;
  bannerUrl: string;
}

/** One option in the backdrop picker — drawn from the member's own lists. */
export interface BannerChoice {
  mediaType: 'anime' | 'manga';
  id: number;
  title: string;
  bannerUrl: string;
}

export interface SearchFilters {
  query?: string;
  genres?: string[];
  year?: number;
  status?: string;
  type?: string;
  letter?: string;
  page?: number;
  limit?: number;
  sort?: 'score' | 'year' | 'title' | 'createdAt';
}

export interface WatchlistUpdateForm {
  status: WatchlistEntry['status'];
  score?: number;
  episodesWatched?: number;
  notes?: string;
}

export interface ReadlistUpdateForm {
  status: ReadlistEntry['status'];
  score?: number;
  chaptersRead?: number;
  volumesRead?: number;
  notes?: string;
}

export interface ProfileUpdateForm {
  username?: string;
  bio?: string;
  favoriteGenres?: string[];
  favorites?: FavoriteRef[];
}

// In-app notification (header bell + /notificari inbox). `text` is
// pre-rendered Romanian; `actor` is the triggering user's name for the avatar
// (absent for system events); `unread` mirrors the server's read_at.
export type NotificationType = 'reply' | 'follow' | 'release' | 'system';

export interface Notification {
  id: number;
  type: NotificationType;
  text: string;
  link?: string;
  actor?: string;
  unread: boolean;
  createdAt: string;
}

/** One live-chat line (PLAN 8.6). The author's role rides along so the panel
    can badge them without a second lookup. */
export interface ChatMessage {
  id: number;
  body: string;
  /** Twitch-style reply: a snapshot of what was answered, so the quote still
   *  reads correctly even if the original is later deleted. */
  replyToUser?: string;
  replyToExcerpt?: string;
  /** The message being answered, so the client can scroll to it. Absent on
   *  replies sent before this existed, and on replies whose target has been
   *  deleted — the quote stays, it just stops being clickable. */
  replyToId?: number;
  createdAt: string;
  userId: number;
  username: string;
  role: string;
  avatarUrl?: string;
}

/**
 * A live-chat timeout or ban. Scoped to the chat room only — it never
 * suspends the account, and an account suspension is a separate thing.
 * `expiresAt` absent means a permanent ban; a date means a lapsing timeout.
 */
export interface ChatRestriction {
  userId: number;
  username: string;
  expiresAt?: string;
  reason?: string;
  /** the staff member who issued it */
  byName?: string;
  createdAt: string;
}

/** A member's report that something is wrong with an episode (PLAN 5.5): a
 *  dead source, wrong video, bad skip markers. Carries series context so the
 *  moderation queue can link straight to the episode. */
export interface EpisodeReport {
  id: number;
  episodeId: number;
  episodeNumber: number;
  animeId: number;
  animeSlug?: string;
  animeTitle: string;
  body: string;
  status: 'open' | 'resolved';
  reporter?: string;
  createdAt: string;
  resolvedAt?: string;
}
