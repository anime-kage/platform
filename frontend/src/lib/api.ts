import type {
  User,
  Anime,
  Manga,
  Episode,
  Chapter,
  ContentLink,
  AuthResponse,
  LoginForm,
  RegisterForm,
  PaginatedResponse,
  PublicConfig,
  CuratedPick,
  CuratedRef,
  CuratedSlotDef,
  CuratedSlots,
  ImportReport,
  ProfileBanner,
  BannerChoice,
  SearchFilters,
  ApiError,
  WatchlistEntry,
  ReadlistEntry,
  UserList,
  UserListItem,
  UserStats,
  WatchlistUpdateForm,
  ReadlistUpdateForm,
  ProfileUpdateForm,
  Comment as CommentType,
  Review,
  HistoryDay,
  FollowNetwork,
  FollowUser,
  UserReview,
  ResolvedStream,
  StreamSourceInfo,
  Subtitle,
  EpisodeSkipMarks,
  SkipRange,
  Release,
  SubtitleEvent,
  TestSourceResult,
  AdminEpisodeGap,
  AdminHealthReport,
  AdminOverview,
  AdminReportedComment,
  MalSearchHit,
  TranslationRequest,
  RequestStatusKey,
  VerifierOption,
  AdminUser,
  TeamMember,
  ChapterPages,
  Notification,
  CommunityMember,
  CommunityReview,
  ActivityEvent,
  CommunityStats,
  CommunityTeamMember,
  HallOfFame,
  ReleaseCredits,
  ForumThread,
  ForumReply,
  ChatMessage,
  ChatRestriction,
  ContinueEntry,
  Announcement,
  ScheduleSlot,
  Emote,
  EpisodeReport
} from '$shared/types';
import { mediaUrl } from './media';

/** One GIF from the picker. `url` is what gets embedded, `preview` is the
    smaller copy the grid shows so 24 full-size animations don't load at once. */
export type GiphyGif = {
  id: string;
  title: string;
  url: string;
  preview: string;
  width: number;
  height: number;
};

/** The editable half of an announcement — the same body for create and update. */
export type AnnouncementInput = {
  tag: string;
  title: string;
  /** Markdown subset — see lib/markdown.ts */
  body?: string;
  url?: string;
  /** must be a path we minted via uploadAnnouncementImage; the API rejects
      anything that isn't under /uploads/ */
  coverUrl?: string;
  isPublished?: boolean;
};

/** A programme slot as the editor sends it. `scheduledAt` must be RFC3339 with
    an offset (`new Date(...).toISOString()`), never a bare local string. */
export type ScheduleSlotInput = {
  animeId: number;
  episodeNumber: number;
  scheduledAt: string;
  note?: string;
};

const API_BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:3000';

/**
 * The rejection value for a cancelled chunked upload.
 *
 * Shaped like the API's own error objects so existing `errMsg` helpers render it,
 * and flagged so the retry loop can tell "the user cancelled" from "the network
 * blipped" — the first must never be retried.
 */
export const UPLOAD_ABORTED = 'upload-aborted';
const uploadAborted = () => ({
  aborted: true,
  code: UPLOAD_ABORTED,
  error: 'Încărcare anulată.',
  message: 'Încărcare anulată.'
});

class ApiClient {
  private baseUrl: string;
  private token: string | null = null;

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl;
    if (typeof window !== 'undefined') {
      const stored = localStorage.getItem('auth_token');
      // Route it through setToken rather than assigning: a session that predates
      // the cookie (or one whose cookie expired first) would otherwise have a
      // working browser token and no server-side credential, which reads as
      // "logged in, but every page 401s". Re-mirroring on load repairs that.
      if (stored) this.setToken(stored);
    }
  }

  private async request<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
    const url = `${this.baseUrl}${endpoint}`;

    // FormData must set its own Content-Type: the header carries the multipart
    // boundary, and forcing application/json here leaves the server unable to
    // parse the body at all. That surfaced as "image too big" on a 300 KB file,
    // because a parse failure and an oversize body are the same error branch.
    const isForm = options.body instanceof FormData;
    const headers: Record<string, string> = {
      ...(isForm ? {} : { 'Content-Type': 'application/json' }),
      ...(options.headers as Record<string, string>),
    };

    if (this.token) {
      headers.Authorization = `Bearer ${this.token}`;
    }

    const response = await fetch(url, { ...options, headers });

    if (!response.ok) {
      const error: ApiError = await response.json().catch(() => ({
        error: 'Network Error',
        message: 'Failed to connect to server',
        statusCode: response.status,
        timestamp: new Date().toISOString()
      }));
      throw error;
    }

    return response.json();
  }

  // ── Auth ──────────────────────────────────────────────────────────────────

  async login(credentials: LoginForm): Promise<AuthResponse> {
    const response = await this.request<AuthResponse>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify(credentials),
    });
    this.setToken(response.token);
    return response;
  }

  /**
   * Curated placements (PLAN 5.8) plus the slot registry — max items and
   * allowed media type per slot come from the server, so the admin UI never
   * has its own copy of those rules to drift from.
   */
  async getCurated(): Promise<{ data: CuratedSlots; slots: CuratedSlotDef[] }> {
    return this.request<{ data: CuratedSlots; slots: CuratedSlotDef[] }>('/api/curated');
  }

  /**
   * Upload artwork for a placement and get its URL back. Nothing is attached
   * until the slot is saved with that URL on a pick — and it never touches
   * the series poster. Manual fetch: request() would force JSON onto multipart.
   */
  async uploadCuratedImage(file: File): Promise<{ imageUrl: string }> {
    const form = new FormData();
    form.set('poster', file);
    const headers: Record<string, string> = {};
    if (this.token) headers.Authorization = `Bearer ${this.token}`;
    const response = await fetch(`${this.baseUrl}/api/curated/image`, {
      method: 'POST',
      headers,
      body: form
    });
    if (!response.ok) {
      throw await response.json().catch(() => ({ error: `Upload failed (${response.status})` }));
    }
    return response.json();
  }

  /** Replace a slot wholesale. An empty list clears it back to automatic. */
  async setCurated(slot: string, items: CuratedRef[]): Promise<{ data: CuratedPick[] }> {
    return this.request<{ data: CuratedPick[] }>(`/api/curated/${slot}`, {
      method: 'PUT',
      body: JSON.stringify({ items }),
    });
  }

  // ── List import + profile banner ───────────────────────

  /** Import a public AniList list by username. No auth on their side needed. */
  async importAniList(username: string): Promise<{ data: ImportReport }> {
    return this.request('/api/users/me/import/anilist', {
      method: 'POST',
      body: JSON.stringify({ username }),
    });
  }

  /**
   * Import a MyAnimeList export (Settings → Export). A file rather than an
   * API because MAL has no public list API and Jikan's user endpoints are
   * unreliable — and this works for private lists too.
   */
  async importMAL(file: File): Promise<{ data: ImportReport; message: string }> {
    const form = new FormData();
    form.set('file', file);
    const headers: Record<string, string> = {};
    if (this.token) headers.Authorization = `Bearer ${this.token}`;
    const response = await fetch(`${this.baseUrl}/api/users/me/import/mal`, {
      method: 'POST',
      headers,
      body: form
    });
    if (!response.ok) {
      throw await response.json().catch(() => ({ error: `Import failed (${response.status})` }));
    }
    return response.json();
  }

  /** Titles from the member's own lists that have banner art. */
  async getBannerOptions(): Promise<{ data: BannerChoice[] }> {
    return this.request('/api/users/me/banner/options');
  }

  /** Set the profile backdrop; id 0 clears it. */
  async setBanner(mediaType: 'anime' | 'manga', id: number): Promise<{ data: ProfileBanner | null }> {
    return this.request('/api/users/me/banner', {
      method: 'PUT',
      body: JSON.stringify({ mediaType, id }),
    });
  }

  /** Public server switches — whether registration needs an invite code. */
  async getPublicConfig(): Promise<{ data: PublicConfig }> {
    return this.request<{ data: PublicConfig }>('/api/config');
  }

  async register(userData: RegisterForm): Promise<AuthResponse> {
    const { confirmPassword, ...apiData } = userData;
    const response = await this.request<AuthResponse>('/api/auth/register', {
      method: 'POST',
      body: JSON.stringify(apiData),
    });
    this.setToken(response.token);
    return response;
  }

  /**
   * Request a reset link. Resolves the same way for unknown addresses as for
   * real ones — the server refuses to confirm who has an account, so the UI
   * must not try to read a difference out of it.
   */
  async forgotPassword(email: string): Promise<{ message: string }> {
    return this.request<{ message: string }>('/api/auth/forgot-password', {
      method: 'POST',
      body: JSON.stringify({ email }),
    });
  }

  /** Redeem a reset token. Does not sign the user in — they log in after. */
  async resetPassword(token: string, password: string): Promise<{ message: string }> {
    return this.request<{ message: string }>('/api/auth/reset-password', {
      method: 'POST',
      body: JSON.stringify({ token, password }),
    });
  }

  async logout(): Promise<void> {
    try {
      await this.request('/api/auth/logout', { method: 'POST' });
    } finally {
      this.clearToken();
    }
  }

  async getCurrentUser(): Promise<User> {
    const response = await this.request<{ user: User }>('/api/auth/me');
    return response.user;
  }

  // ── Anime ─────────────────────────────────────────────────────────────────

  async getAnime(filters?: SearchFilters): Promise<PaginatedResponse<Anime>> {
    const params = this.buildParams(filters);
    return this.request<PaginatedResponse<Anime>>(`/api/anime${params ? `?${params}` : ''}`);
  }

  async getAnimeById(id: number): Promise<{ data: Anime }> {
    return this.request<{ data: Anime }>(`/api/anime/${id}`);
  }

  async searchAnime(query: string, filters?: Omit<SearchFilters, 'query'>): Promise<PaginatedResponse<Anime>> {
    const params = new URLSearchParams({ q: query });
    if (filters) {
      Object.entries(filters).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          if (Array.isArray(value)) value.forEach(v => params.append(key, String(v)));
          else params.append(key, String(value));
        }
      });
    }
    return this.request<PaginatedResponse<Anime>>(`/api/anime/search?${params}`);
  }

  async getRandomAnime(): Promise<{ data: Anime }> {
    return this.request<{ data: Anime }>('/api/anime/random');
  }

  async getAnimeSchedule(): Promise<{ data: Record<string, Anime[]> }> {
    return this.request<{ data: Record<string, Anime[]> }>('/api/anime/schedule');
  }

  async getTrendingAnime(page = 1, limit = 25): Promise<PaginatedResponse<Anime>> {
    return this.request<PaginatedResponse<Anime>>(`/api/anime/trending?page=${page}&limit=${limit}`);
  }

  async getAiringAnime(page = 1, limit = 25): Promise<PaginatedResponse<Anime>> {
    return this.request<PaginatedResponse<Anime>>(`/api/anime/airing?page=${page}&limit=${limit}`);
  }

  async getSeasonalAnime(year: number, season: string, page = 1, limit = 25): Promise<PaginatedResponse<Anime>> {
    return this.request<PaginatedResponse<Anime>>(`/api/anime/season/${year}/${season}?page=${page}&limit=${limit}`);
  }

  async importAnime(malId: number): Promise<{ data: Anime; created?: boolean; message: string }> {
    return this.request<{ data: Anime; created?: boolean; message: string }>(`/api/anime/import/${malId}`, { method: 'POST' });
  }

  /** Coordinator/admin: search MyAnimeList by title (nothing is imported). */
  async malSearchAnime(query: string): Promise<{ data: MalSearchHit[] }> {
    return this.request(`/api/anime/mal-search?q=${encodeURIComponent(query)}`);
  }

  async importManga(malId: number): Promise<{ data: Manga; created?: boolean; message: string }> {
    return this.request<{ data: Manga; created?: boolean; message: string }>(`/api/manga/import/${malId}`, { method: 'POST' });
  }

  /** Coordinator/admin: search MyAnimeList manga by title (nothing is imported). */
  async malSearchManga(query: string): Promise<{ data: MalSearchHit[] }> {
    return this.request(`/api/manga/mal-search?q=${encodeURIComponent(query)}`);
  }

  /** Coordinator/admin: manual manga entry, the fallback when MAL import is down. */
  async createMangaManual(body: {
    title: string;
    titleRomanian?: string;
    synopsis?: string;
    synopsisRomanian?: string;
    type?: string;
    status?: string;
    year?: number;
    chapters?: number;
    genres?: string[];
    authors?: string[];
  }): Promise<{ data: Manga; message: string }> {
    return this.request('/api/manga', { method: 'POST', body: JSON.stringify(body) });
  }

  /** Coordinator/admin: manual catalog entry, the fallback when MAL import is down. */
  async createAnimeManual(body: {
    title: string;
    titleRomanian?: string;
    synopsis?: string;
    synopsisRomanian?: string;
    type?: string;
    status?: string;
    year?: number;
    episodes?: number;
    genres?: string[];
  }): Promise<{ data: Anime; message: string }> {
    return this.request('/api/anime', { method: 'POST', body: JSON.stringify(body) });
  }

  /** Coordinator/admin: replace a title's poster with an uploaded image.
      Manual fetch — request() would force a JSON content-type onto multipart. */
  async uploadPoster(kind: 'anime' | 'manga', id: number, file: File): Promise<{ imageUrl: string }> {
    const form = new FormData();
    form.set('poster', file);
    const headers: Record<string, string> = {};
    if (this.token) headers.Authorization = `Bearer ${this.token}`;
    const response = await fetch(`${this.baseUrl}/api/${kind}/${id}/poster`, {
      method: 'POST',
      headers,
      body: form
    });
    if (!response.ok) {
      throw await response.json().catch(() => ({ error: `Upload failed (${response.status})` }));
    }
    return response.json();
  }

  // ── Catalog management ─────────────────────────────────────────

  /** Manual field edits; only the provided fields change. */
  async patchAnime(id: number, patch: Partial<Anime>): Promise<{ data: Anime }> {
    return this.request(`/api/anime/${id}`, { method: 'PUT', body: JSON.stringify(patch) });
  }

  async patchManga(id: number, patch: Partial<Manga>): Promise<{ data: Manga }> {
    return this.request(`/api/manga/${id}`, { method: 'PUT', body: JSON.stringify(patch) });
  }

  /** Re-sync every field from Jikan by the stored MAL id. */
  async syncAnimeFromJikan(id: number): Promise<{ data: Anime }> {
    return this.request(`/api/anime/${id}/update`, { method: 'PUT' });
  }

  async syncMangaFromJikan(id: number): Promise<{ data: Manga }> {
    return this.request(`/api/manga/${id}/update`, { method: 'PUT' });
  }

  /** Admin only — episodes/chapters, links, lists and comments cascade. */
  async deleteAnime(id: number): Promise<{ message: string }> {
    return this.request(`/api/anime/${id}`, { method: 'DELETE' });
  }

  async deleteManga(id: number): Promise<{ message: string }> {
    return this.request(`/api/manga/${id}`, { method: 'DELETE' });
  }

  /** Import one Jikan page (25 titles) of a season; call again with the next page while hasNext. */
  async importSeason(
    year: number,
    season: string,
    page = 1
  ): Promise<{ data: { imported: number; skipped: number; page: number; hasNext: boolean } }> {
    return this.request(`/api/admin/import-season/${year}/${season}?page=${page}`, { method: 'POST' });
  }

  // ── Manga ─────────────────────────────────────────────────────────────────

  async getManga(filters?: SearchFilters): Promise<PaginatedResponse<Manga>> {
    const params = this.buildParams(filters);
    return this.request<PaginatedResponse<Manga>>(`/api/manga${params ? `?${params}` : ''}`);
  }

  async getMangaById(id: number): Promise<{ data: Manga }> {
    return this.request<{ data: Manga }>(`/api/manga/${id}`);
  }

  async searchManga(query: string, filters?: Omit<SearchFilters, 'query'>): Promise<PaginatedResponse<Manga>> {
    const params = new URLSearchParams({ q: query });
    if (filters) {
      Object.entries(filters).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          if (Array.isArray(value)) value.forEach(v => params.append(key, String(v)));
          else params.append(key, String(value));
        }
      });
    }
    return this.request<PaginatedResponse<Manga>>(`/api/manga/search?${params}`);
  }

  async getTrendingManga(page = 1, limit = 25): Promise<PaginatedResponse<Manga>> {
    return this.request<PaginatedResponse<Manga>>(`/api/manga/trending?page=${page}&limit=${limit}`);
  }

  // ── User Profile ──────────────────────────────────────────────────────────

  async getMyProfile(): Promise<{ user: User; stats: UserStats; banner: ProfileBanner | null }> {
    return this.request<{ user: User; stats: UserStats; banner: ProfileBanner | null }>('/api/users/me');
  }

  /** Public profile; with a token attached, `network.isFollowing` reflects the viewer. */
  async getPublicUser(
    username: string
  ): Promise<{
    user: Omit<User, 'email'>;
    stats: UserStats;
    network: FollowNetwork;
    /** their chosen series backdrop (PLAN 8.17); null for most members */
    banner: ProfileBanner | null;
  }> {
    return this.request(`/api/users/${encodeURIComponent(username)}`);
  }

  // ── Follows ───────────────────────────────────────────────────────────────

  async followUser(username: string): Promise<{ data: FollowNetwork }> {
    return this.request<{ data: FollowNetwork }>(`/api/users/${encodeURIComponent(username)}/follow`, {
      method: 'POST',
    });
  }

  async unfollowUser(username: string): Promise<{ data: FollowNetwork }> {
    return this.request<{ data: FollowNetwork }>(`/api/users/${encodeURIComponent(username)}/follow`, {
      method: 'DELETE',
    });
  }

  async getFollowers(username: string): Promise<{ data: FollowUser[] }> {
    return this.request<{ data: FollowUser[] }>(`/api/users/${encodeURIComponent(username)}/followers`);
  }

  async getFollowing(username: string): Promise<{ data: FollowUser[] }> {
    return this.request<{ data: FollowUser[] }>(`/api/users/${encodeURIComponent(username)}/following`);
  }

  async getUserReviews(username: string): Promise<{ data: UserReview[] }> {
    return this.request<{ data: UserReview[] }>(`/api/users/${encodeURIComponent(username)}/reviews`);
  }

  async updateProfile(data: ProfileUpdateForm): Promise<{ user: User; message: string }> {
    return this.request<{ user: User; message: string }>('/api/users/me', {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  async uploadAvatar(file: File): Promise<{ user: User; avatarUrl: string; message: string }> {
    const formData = new FormData();
    formData.append('avatar', file);

    const url = `${this.baseUrl}/api/users/me/avatar`;
    const headers: Record<string, string> = {};
    if (this.token) {
      headers.Authorization = `Bearer ${this.token}`;
    }

    const response = await fetch(url, {
      method: 'POST',
      headers,
      body: formData,
    });

    if (!response.ok) {
      const error = await response.json().catch(() => ({
        error: 'Upload Error',
        message: 'Failed to upload avatar',
        statusCode: response.status,
        timestamp: new Date().toISOString(),
      }));
      throw error;
    }

    return response.json();
  }

  async getPublicProfile(username: string): Promise<{ user: Omit<User, 'email'>; stats: UserStats }> {
    return this.request<{ user: Omit<User, 'email'>; stats: UserStats }>(`/api/users/${username}`);
  }

  // ── Watchlist ─────────────────────────────────────────────────────────────

  /**
   * The home "Continuă vizionarea" row: one card per series, already resolved
   * to the episode to open and the second to resume at. Not the watchlist —
   * see ContinueEntry.
   */
  async getContinueWatching(limit?: number): Promise<{ data: ContinueEntry[] }> {
    return this.request(`/api/users/me/continue${limit ? `?limit=${limit}` : ''}`);
  }

  async getWatchlist(status?: string): Promise<{ data: WatchlistEntry[] }> {
    const query = status ? `?status=${status}` : '';
    return this.request<{ data: WatchlistEntry[] }>(`/api/users/me/watchlist${query}`);
  }

  async getWatchlistEntry(animeId: number): Promise<{ data: WatchlistEntry }> {
    return this.request<{ data: WatchlistEntry }>(`/api/users/me/watchlist/${animeId}`);
  }

  async addToWatchlist(animeId: number, data: WatchlistUpdateForm): Promise<{ data: WatchlistEntry; message: string }> {
    return this.request<{ data: WatchlistEntry; message: string }>('/api/users/me/watchlist', {
      method: 'POST',
      body: JSON.stringify({ animeId, ...data }),
    });
  }

  async updateWatchlistEntry(animeId: number, data: WatchlistUpdateForm): Promise<{ data: WatchlistEntry; message: string }> {
    return this.request<{ data: WatchlistEntry; message: string }>(`/api/users/me/watchlist/${animeId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  async removeFromWatchlist(animeId: number): Promise<{ message: string }> {
    return this.request<{ message: string }>(`/api/users/me/watchlist/${animeId}`, { method: 'DELETE' });
  }

  // ── Readlist ──────────────────────────────────────────────────────────────

  async getReadlist(status?: string): Promise<{ data: ReadlistEntry[] }> {
    const query = status ? `?status=${status}` : '';
    return this.request<{ data: ReadlistEntry[] }>(`/api/users/me/readlist${query}`);
  }

  async getReadlistEntry(mangaId: number): Promise<{ data: ReadlistEntry }> {
    return this.request<{ data: ReadlistEntry }>(`/api/users/me/readlist/${mangaId}`);
  }

  async addToReadlist(mangaId: number, data: ReadlistUpdateForm): Promise<{ data: ReadlistEntry; message: string }> {
    return this.request<{ data: ReadlistEntry; message: string }>('/api/users/me/readlist', {
      method: 'POST',
      body: JSON.stringify({ mangaId, ...data }),
    });
  }

  async updateReadlistEntry(mangaId: number, data: ReadlistUpdateForm): Promise<{ data: ReadlistEntry; message: string }> {
    return this.request<{ data: ReadlistEntry; message: string }>(`/api/users/me/readlist/${mangaId}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  async removeFromReadlist(mangaId: number): Promise<{ message: string }> {
    return this.request<{ message: string }>(`/api/users/me/readlist/${mangaId}`, { method: 'DELETE' });
  }

  // ── Custom lists (the /liste feature) ─────────────────────────────────────

  async getPublicLists(): Promise<{ data: UserList[] }> {
    return this.request('/api/lists');
  }

  async getMyLists(): Promise<{ data: UserList[] }> {
    return this.request('/api/lists/mine');
  }

  /** Public lists work for guests; private ones only for their owner. */
  async getList(id: number): Promise<{ data: UserList; items: UserListItem[] }> {
    return this.request(`/api/lists/${id}`);
  }

  async createList(body: { title: string; description?: string; isPublic?: boolean }): Promise<{ data: UserList }> {
    return this.request('/api/lists', { method: 'POST', body: JSON.stringify(body) });
  }

  async updateList(id: number, body: { title: string; description?: string; isPublic?: boolean }): Promise<{ message: string }> {
    return this.request(`/api/lists/${id}`, { method: 'PUT', body: JSON.stringify(body) });
  }

  async deleteList(id: number): Promise<{ message: string }> {
    return this.request(`/api/lists/${id}`, { method: 'DELETE' });
  }

  // ── Translation requests ("Cereri") ────────────────────────────────────────
  async getRequests(params: { status?: string; sort?: string; page?: number } = {}): Promise<{
    data: TranslationRequest[];
    pagination: { page: number; pages: number; total: number; perPage: number };
  }> {
    const p = new URLSearchParams();
    if (params.status) p.set('status', params.status);
    if (params.sort) p.set('sort', params.sort);
    if (params.page) p.set('page', String(params.page));
    return this.request(`/api/requests?${p}`);
  }

  async searchRequests(q: string): Promise<{ data: (MalSearchHit & { medium: 'anime' | 'manga' })[] }> {
    return this.request(`/api/requests/search?q=${encodeURIComponent(q)}`);
  }

  async createRequest(body: {
    medium: 'anime' | 'manga';
    title: string;
    malId?: number;
    imageUrl?: string;
    note?: string;
  }): Promise<{ data: TranslationRequest; merged: boolean; message?: string }> {
    return this.request('/api/requests', { method: 'POST', body: JSON.stringify(body) });
  }

  async voteRequest(id: number): Promise<{ voteCount: number; voted: boolean }> {
    return this.request(`/api/requests/${id}/vote`, { method: 'POST' });
  }

  async unvoteRequest(id: number): Promise<{ voteCount: number; voted: boolean }> {
    return this.request(`/api/requests/${id}/vote`, { method: 'DELETE' });
  }

  async setRequestStatus(id: number, status: RequestStatusKey): Promise<{ data: TranslationRequest }> {
    return this.request(`/api/requests/${id}/status`, {
      method: 'PATCH',
      body: JSON.stringify({ status })
    });
  }

  async likeList(id: number): Promise<{ likeCount: number; liked: boolean }> {
    return this.request(`/api/lists/${id}/like`, { method: 'POST' });
  }

  async unlikeList(id: number): Promise<{ likeCount: number; liked: boolean }> {
    return this.request(`/api/lists/${id}/like`, { method: 'DELETE' });
  }

  async addListItem(
    listId: number,
    body: { mediaType: 'anime' | 'manga'; mediaId: number; note?: string }
  ): Promise<{ data: UserListItem }> {
    return this.request(`/api/lists/${listId}/items`, { method: 'POST', body: JSON.stringify(body) });
  }

  async updateListItem(listId: number, itemId: number, note: string | null): Promise<{ message: string }> {
    return this.request(`/api/lists/${listId}/items/${itemId}`, {
      method: 'PUT',
      body: JSON.stringify({ note }),
    });
  }

  async removeListItem(listId: number, itemId: number): Promise<{ message: string }> {
    return this.request(`/api/lists/${listId}/items/${itemId}`, { method: 'DELETE' });
  }

  // ── Comments ──────────────────────────────────────────────────────────────

  async getAnimeComments(animeId: number, page = 1, limit = 20, episodeId?: number, reviewId?: number): Promise<{ data: CommentType[]; pagination: any }> {
    const scope = reviewId ? `&reviewId=${reviewId}` : episodeId ? `&episodeId=${episodeId}` : '';
    return this.request(`/api/anime/${animeId}/comments?page=${page}&limit=${limit}${scope}`);
  }

  async postAnimeComment(animeId: number, content: string, episodeId?: number, reviewId?: number): Promise<{ data: CommentType }> {
    return this.request(`/api/anime/${animeId}/comments`, {
      method: 'POST',
      body: JSON.stringify({ content, ...(reviewId ? { reviewId } : episodeId ? { episodeId } : {}) }),
    });
  }

  async getMangaComments(mangaId: number, page = 1, limit = 20, chapterId?: number, reviewId?: number): Promise<{ data: CommentType[]; pagination: any }> {
    const scope = reviewId ? `&reviewId=${reviewId}` : chapterId ? `&chapterId=${chapterId}` : '';
    return this.request(`/api/manga/${mangaId}/comments?page=${page}&limit=${limit}${scope}`);
  }

  async postMangaComment(mangaId: number, content: string, chapterId?: number, reviewId?: number): Promise<{ data: CommentType }> {
    return this.request(`/api/manga/${mangaId}/comments`, {
      method: 'POST',
      body: JSON.stringify({ content, ...(reviewId ? { reviewId } : chapterId ? { chapterId } : {}) }),
    });
  }

  // ── Reviews & history ─────────────────────────────────────────────────────

  async getAnimeReviews(animeId: number): Promise<{ data: Review[] }> {
    return this.request(`/api/anime/${animeId}/reviews`);
  }

  async getMangaReviews(mangaId: number): Promise<{ data: Review[] }> {
    return this.request(`/api/manga/${mangaId}/reviews`);
  }

  async getMyHistory(days = 14): Promise<{ data: HistoryDay[] }> {
    return this.request(`/api/users/me/history?days=${days}`);
  }

  async editComment(commentId: number, content: string): Promise<{ data: CommentType }> {
    return this.request(`/api/comments/${commentId}`, {
      method: 'PUT',
      body: JSON.stringify({ content }),
    });
  }

  async deleteComment(commentId: number): Promise<{ message: string }> {
    return this.request(`/api/comments/${commentId}`, { method: 'DELETE' });
  }

  async voteComment(commentId: number, voteType: 'like' | 'dislike'): Promise<{ message: string; voteType: 'like' | 'dislike' | null }> {
    return this.request(`/api/comments/${commentId}/vote`, {
      method: 'POST',
      body: JSON.stringify({ voteType }),
    });
  }

  async reportComment(commentId: number): Promise<{ message: string }> {
    return this.request(`/api/comments/${commentId}/report`, { method: 'POST' });
  }

  /** Report a problem with an episode — dead source, wrong video, bad skip
   *  markers. Unlike comment reports this carries the member's own words. */
  async reportEpisode(episodeId: number, body: string): Promise<{ message: string }> {
    return this.request(`/api/episodes/${episodeId}/report`, {
      method: 'POST',
      body: JSON.stringify({ body })
    });
  }

  async listEpisodeReports(status = 'open'): Promise<{ data: EpisodeReport[]; total: number }> {
    return this.request(`/api/admin/episode-reports?status=${encodeURIComponent(status)}`);
  }

  async resolveEpisodeReport(id: number): Promise<{ message: string }> {
    return this.request(`/api/admin/episode-reports/${id}/resolve`, { method: 'POST' });
  }

  async getReplies(commentId: number): Promise<{ data: CommentType[] }> {
    return this.request(`/api/comments/${commentId}/replies`);
  }

  async postReply(commentId: number, content: string): Promise<{ data: CommentType }> {
    return this.request(`/api/comments/${commentId}/reply`, {
      method: 'POST',
      body: JSON.stringify({ content }),
    });
  }

  // ── Episodes ──────────────────────────────────────────────────────────────

  async getEpisodes(animeId: number): Promise<{ data: Episode[] }> {
    return this.request(`/api/anime/${animeId}/episodes`);
  }

  async getEpisode(animeId: number, episodeNumber: number): Promise<{ data: Episode & { links: ContentLink[] } }> {
    return this.request(`/api/anime/${animeId}/episodes/${episodeNumber}`);
  }

  async createEpisode(animeId: number, data: Partial<Episode>): Promise<{ data: Episode }> {
    return this.request(`/api/anime/${animeId}/episodes`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async updateEpisode(animeId: number, episodeNumber: number, data: Partial<Episode>): Promise<{ data: Episode }> {
    return this.request(`/api/anime/${animeId}/episodes/${episodeNumber}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  /**
   * Pull episode titles, air dates and MAL's filler/recap marks for one series.
   *
   * Explicitly triggered, because the nightly job only polls series whose status
   * is airing/upcoming — a completed one added by hand never gets filled in
   * otherwise. Rejects with 502 when MAL is unreachable, which is often.
   */
  async syncEpisodesFromMAL(
    animeId: number
  ): Promise<{ added: number; updated: number; found: number }> {
    return this.request(`/api/anime/${animeId}/episodes/sync`, { method: 'POST' });
  }

  async deleteEpisode(animeId: number, episodeNumber: number): Promise<{ message: string }> {
    return this.request(`/api/anime/${animeId}/episodes/${episodeNumber}`, {
      method: 'DELETE',
    });
  }

  /** Resolve the best extract source to a playable manifest; 404 = use embeds. */
  async getEpisodeStream(episodeId: number): Promise<{ data: { stream: ResolvedStream; source: StreamSourceInfo } }> {
    const r = await this.request<{ data: { stream: ResolvedStream; source: StreamSourceInfo } }>(
      `/api/episodes/${episodeId}/stream`
    );
    // our own tracks are stored API-relative (/uploads/...); <track src> needs
    // them absolute against the backend, not the frontend origin
    if (r.data.stream.subtitles) {
      r.data.stream.subtitles = r.data.stream.subtitles.map((s) =>
        s.url.startsWith('/') ? { ...s, url: `${this.baseUrl}${s.url}` } : s
      );
    }
    return r;
  }

  async addEpisodeLink(
    episodeId: number,
    data: {
      hostingUrl: string;
      quality?: string;
      language?: string;
      kind?: 'embed' | 'extract';
      provider?: string;
      providerRef?: string;
      priority?: number;
    }
  ): Promise<{ data: ContentLink }> {
    return this.request(`/api/episodes/${episodeId}/links`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async updateContentLink(
    linkId: number,
    patch: { quality?: string; isActive?: boolean; priority?: number }
  ): Promise<{ data: ContentLink }> {
    return this.request(`/api/links/${linkId}`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    });
  }

  async deleteContentLink(linkId: number): Promise<{ message: string }> {
    return this.request(`/api/links/${linkId}`, {
      method: 'DELETE',
    });
  }

  // ── Admin panel ──────────────────────────────────────────

  /** Prove a source resolves and plays before saving it. Never throws on a dead source — check `ok`. */
  async testSource(data: {
    kind: 'embed' | 'extract';
    hostingUrl?: string;
    provider?: string;
    providerRef?: string;
  }): Promise<{ data: TestSourceResult }> {
    return this.request('/api/admin/test-source', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async getAdminHealthReport(): Promise<{ data: AdminHealthReport }> {
    return this.request('/api/admin/health-report');
  }

  /** Every link on an episode, including disabled ones (public endpoints hide those). */
  async getAdminEpisodeLinks(episodeId: number): Promise<{ data: ContentLink[] }> {
    return this.request(`/api/admin/episodes/${episodeId}/links`);
  }

  // ── Chapter pages — native manga reader ────────────────────────

  async getChapterPages(chapterId: number, lang?: string): Promise<{ data: ChapterPages }> {
    const r = await this.request<{ data: ChapterPages }>(
      `/api/chapters/${chapterId}/pages${lang ? `?lang=${encodeURIComponent(lang)}` : ''}`
    );
    // extracted editions come back as API-relative proxy paths —
    // the <img> tags need them absolute against the backend, not the frontend
    r.data.pages = r.data.pages.map((p) => (p.startsWith('/') ? `${this.baseUrl}${p}` : p));
    return r;
  }

  /** Replace one language edition's whole page list (admin/translator). */
  async setChapterPages(chapterId: number, data: { language?: string; urls: string[] }): Promise<{ message: string }> {
    return this.request(`/api/chapters/${chapterId}/pages`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  async deleteChapterPages(chapterId: number, lang = 'ro'): Promise<{ message: string }> {
    return this.request(`/api/chapters/${chapterId}/pages?lang=${encodeURIComponent(lang)}`, {
      method: 'DELETE',
    });
  }

  /** Upload page images for one edition (PLAN 5.7/6.3) — replaces the whole
      set, ordered by filename; stored on R2 when configured, else locally.
      Manual fetch: request() forces a JSON content-type. */
  async uploadChapterPages(
    chapterId: number,
    lang: string,
    files: File[]
  ): Promise<{ message: string; count: number; urls: string[]; storage: 'r2' | 'local' }> {
    const form = new FormData();
    form.set('lang', lang);
    for (const f of files) form.append('pages', f, f.name);
    const headers: Record<string, string> = {};
    if (this.token) headers.Authorization = `Bearer ${this.token}`;
    const response = await fetch(`${this.baseUrl}/api/chapters/${chapterId}/pages/upload`, {
      method: 'POST',
      headers,
      body: form
    });
    if (!response.ok) {
      throw await response.json().catch(() => ({ error: `Upload failed (${response.status})` }));
    }
    return response.json();
  }

  // ── Moderation ─────────────────────────────────────────────────

  async getAdminReports(opts: { limit?: number; offset?: number } = {}): Promise<{ data: AdminReportedComment[]; total: number }> {
    const params = new URLSearchParams();
    if (opts.limit) params.set('limit', String(opts.limit));
    if (opts.offset) params.set('offset', String(opts.offset));
    const qs = params.toString();
    return this.request(`/api/admin/reports${qs ? `?${qs}` : ''}`);
  }

  /** Keep the comment, clear the report flag. */
  async dismissReport(commentId: number): Promise<{ message: string }> {
    return this.request(`/api/admin/comments/${commentId}/dismiss`, { method: 'POST' });
  }

  /** Moderator delete — any comment, not just your own. */
  async adminDeleteComment(commentId: number): Promise<{ message: string }> {
    return this.request(`/api/admin/comments/${commentId}`, { method: 'DELETE' });
  }

  async findUsers(q: string): Promise<{ data: AdminUser[] }> {
    return this.request(`/api/admin/users?q=${encodeURIComponent(q)}`);
  }

  /** Admin/moderator: catalog + team headcounts for the panel's stat strip. */
  async getAdminOverview(): Promise<{ data: AdminOverview }> {
    return this.request('/api/admin/overview');
  }

  /** Admin/moderator: everyone holding a team role. */
  async getAdminTeam(): Promise<{ data: TeamMember[] }> {
    return this.request('/api/admin/team');
  }

  async setUserBan(userId: number, banned: boolean): Promise<{ message: string }> {
    return this.request(`/api/admin/users/${userId}/ban`, {
      method: 'POST',
      body: JSON.stringify({ banned }),
    });
  }

  /** Admin only. */
  async setUserRole(userId: number, role: string): Promise<{ message: string }> {
    return this.request(`/api/admin/users/${userId}/role`, {
      method: 'PUT',
      body: JSON.stringify({ role }),
    });
  }

  /**
   * One page of a health-report gap list. The report itself carries only the
   * first page — on a fresh catalog these run to thousands of episodes.
   */
  async getAdminHealthGaps(
    kind: 'source' | 'rosub',
    opts?: { limit?: number; offset?: number }
  ): Promise<{ data: { total: number; episodes: AdminEpisodeGap[] } }> {
    const params = new URLSearchParams({ kind });
    if (opts?.limit != null) params.set('limit', String(opts.limit));
    if (opts?.offset != null) params.set('offset', String(opts.offset));
    return this.request(`/api/admin/health-gaps?${params}`);
  }

  /**
   * Disk on the box that serves the API. `staging` is the part that grows with
   * uploads; the filesystem numbers are 0 on a non-Linux host (unsupported
   * there, not an error).
   */
  async getAdminStorage(): Promise<{
    data: { stagingBytes: number; stagingDirs: number; diskTotalBytes: number; diskFreeBytes: number };
  }> {
    return this.request('/api/admin/storage');
  }

  /**
   * Admin only. Per-user in-flight release cap: null restores the server
   * default, 0 removes the cap for that person.
   */
  async setUserReleaseCap(userId: number, cap: number | null): Promise<{ message: string }> {
    return this.request(`/api/admin/users/${userId}/release-cap`, {
      method: 'PUT',
      body: JSON.stringify({ cap }),
    });
  }

  // ── Live chat ──────────────────────────────────────────────────

  /** The backlog a freshly-opened panel shows, oldest-first. */
  async getChatMessages(limit?: number): Promise<{ data: ChatMessage[]; viewers: number }> {
    return this.request(`/api/chat/messages${limit ? `?limit=${limit}` : ''}`);
  }

  async sendChatMessage(body: {
    body: string;
    replyToUser?: string;
    replyToExcerpt?: string;
    replyToId?: number;
  }): Promise<{ data: ChatMessage }> {
    return this.request('/api/chat/messages', { method: 'POST', body: JSON.stringify(body) });
  }

  /** Own message, or any message for moderators/admins. */
  async deleteChatMessage(id: number): Promise<{ message: string }> {
    return this.request(`/api/chat/messages/${id}`, { method: 'DELETE' });
  }

  /**
   * SSE endpoint for live messages. EventSource cannot send an Authorization
   * header, so the token goes in the query string (the server scopes
   * TokenFromQuery to this route group).
   */
  chatStreamUrl(): string {
    return this.fileUrl('/api/chat/stream');
  }

  // ── Chat moderation (timeouts/bans — chat only, never the account) ────────

  /** Staff only. `data` is null when the member is not restricted. */
  async getChatRestriction(username: string): Promise<{ data: ChatRestriction | null }> {
    return this.request(`/api/chat/restrictions/${encodeURIComponent(username)}`);
  }

  /** `seconds` of 0 is a permanent ban; anything else is a lapsing timeout. */
  async setChatRestriction(
    username: string,
    seconds: number,
    reason?: string
  ): Promise<{ message: string }> {
    return this.request(`/api/chat/restrictions/${encodeURIComponent(username)}`, {
      method: 'PUT',
      body: JSON.stringify({ seconds, reason: reason ?? null })
    });
  }

  async clearChatRestriction(username: string): Promise<{ message: string }> {
    return this.request(`/api/chat/restrictions/${encodeURIComponent(username)}`, {
      method: 'DELETE'
    });
  }

  // ── Subtitles (our own tracks —) ─────────────────────────────────

  async getEpisodeSubtitles(episodeId: number): Promise<{ data: Subtitle[] }> {
    return this.request(`/api/episodes/${episodeId}/subtitles`);
  }

  /** Reference a track hosted elsewhere. For a local file, use
      uploadEpisodeSubtitle — it converts, which this cannot. */
  async addEpisodeSubtitle(
    episodeId: number,
    data: { language: string; label?: string; format?: 'vtt' | 'srt' | 'ass'; url: string }
  ): Promise<{ data: Subtitle }> {
    return this.request(`/api/episodes/${episodeId}/subtitles`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  /**
   * Attach a subtitle file straight to an episode, for one whose source was
   * linked by hand rather than going through the release pipeline.
   *
   * The server parses .srt/.ass/.vtt and stores WebVTT, because <track> renders
   * nothing else — a raw .srt would be accepted and then never show. Re-uploading
   * the same language replaces the track.
   *
   * Raw fetch, not request(): FormData needs the browser to set the multipart
   * boundary, and request() would stamp application/json over it.
   */
  async uploadEpisodeSubtitle(
    episodeId: number,
    file: File,
    opts: { language?: string; label?: string } = {}
  ): Promise<{ data: Subtitle; cues: number }> {
    const form = new FormData();
    form.set('file', file);
    form.set('language', opts.language ?? 'ro');
    if (opts.label) form.set('label', opts.label);
    const headers: Record<string, string> = {};
    if (this.token) headers.Authorization = `Bearer ${this.token}`;
    const response = await fetch(`${this.baseUrl}/api/episodes/${episodeId}/subtitles/upload`, {
      method: 'POST',
      headers,
      body: form
    });
    if (!response.ok) {
      throw await response.json().catch(() => ({ error: `Upload failed (${response.status})` }));
    }
    return response.json();
  }

  async deleteSubtitle(subtitleId: number): Promise<{ message: string }> {
    return this.request(`/api/subtitles/${subtitleId}`, {
      method: 'DELETE',
    });
  }

  // ── Playback progress ──────────────────────────────────────────

  async getPlaybackProgress(
    episodeId: number
  ): Promise<{ data: { position: number; duration?: number } | null }> {
    return this.request(`/api/episodes/${episodeId}/progress`);
  }

  /** Throttled save from the player; crossing ~90% auto-marks watched server-side. */
  async savePlaybackProgress(
    episodeId: number,
    data: { position: number; duration?: number }
  ): Promise<{ data: { position: number; watched: boolean } }> {
    return this.request(`/api/episodes/${episodeId}/progress`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  /**
   * Count one view of an episode — what the home leaderboards rank by.
   *
   * Safe to call on every episode open: the server keys views on
   * (user, episode) and drops repeats, so re-watches and refreshes cost nothing
   * and cannot inflate the count. `counted` reports whether this call was the
   * one that landed.
   */
  async recordEpisodeView(episodeId: number): Promise<{ data: { counted: boolean } }> {
    return this.request(`/api/episodes/${episodeId}/view`, { method: 'POST' });
  }

  // ── Skip marks ─────────────────────────────────────────────────

  /** Our marks, falling back to AniSkip server-side; nulls when unknown. */
  async getEpisodeSkip(episodeId: number): Promise<{ data: EpisodeSkipMarks }> {
    return this.request(`/api/episodes/${episodeId}/skip`);
  }

  async setSkipMark(
    episodeId: number,
    data: { kind: 'intro' | 'outro' } & SkipRange
  ): Promise<{ data: unknown }> {
    return this.request(`/api/episodes/${episodeId}/skip`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async deleteSkipMark(episodeId: number, kind: 'intro' | 'outro'): Promise<{ message: string }> {
    return this.request(`/api/episodes/${episodeId}/skip/${kind}`, {
      method: 'DELETE',
    });
  }

  // ── Releases ────────────────────

  async getReleases(opts?: { state?: string; all?: boolean; assigned?: boolean }): Promise<{ data: Release[] }> {
    const params = new URLSearchParams();
    if (opts?.state) params.set('state', opts.state);
    if (opts?.all) params.set('all', '1');
    if (opts?.assigned) params.set('assigned', '1');
    const qs = params.toString();
    return this.request(`/api/releases${qs ? `?${qs}` : ''}`);
  }

  /**
   * How many unpublished releases the caller holds vs. how many they may hold.
   * Staging is sized by translators × cap × file size, so the upload form shows
   * this before you pick a 1.4 GB file rather than after.
   */
  async getReleaseQuota(): Promise<{ data: { used: number; limit: number; canUpload: boolean } }> {
    return this.request('/api/releases/quota');
  }

  /** The assignable verifiers plus the caller's last explicit pick (the default). */
  async getVerifiers(): Promise<{ data: VerifierOption[]; lastVerifierId: number | null }> {
    return this.request('/api/releases/verifiers');
  }

  /** Reroute a release's verification (uploader on own, coordinator/admin on any). */
  async assignVerifier(releaseId: number, verifierId: number | null): Promise<{ message: string }> {
    return this.request(`/api/releases/${releaseId}/verifier`, {
      method: 'PUT',
      body: JSON.stringify({ verifierId })
    });
  }

  async getRelease(id: number): Promise<{ data: Release }> {
    return this.request(`/api/releases/${id}`);
  }

  /**
   * Upload a video straight to Cloudflare R2 with a presigned URL.
   *
   * The bytes never touch our server, so neither our disk nor Cloudflare's
   * 100 MiB proxy body limit is involved — the browser is talking to
   * r2.cloudflarestorage.com directly and our API only handed out permission.
   * The release form then references the object by key and the server pulls it
   * into staging at datacenter speed.
   *
   * XHR rather than fetch, for the same reason as everywhere else here: fetch
   * cannot report upload progress, and a multi-gigabyte upload without a bar is
   * indistinguishable from a hang.
   *
   * Returns the object key plus the size, both of which the release form sends
   * so the server can verify the object is complete before using it.
   */
  async uploadVideoToR2(
    file: File,
    onProgress?: (fraction: number) => void,
    opts: { signal?: AbortSignal } = {}
  ): Promise<{ key: string; size: number }> {
    const minted = await this.request<{
      data: { key: string; url: string; expiresIn: number };
    }>('/api/uploads/video/presign', {
      method: 'POST',
      body: JSON.stringify({ filename: file.name, size: file.size })
    });
    const { key, url } = minted.data;

    await new Promise<void>((resolve, reject) => {
      if (opts.signal?.aborted) {
        reject(uploadAborted());
        return;
      }
      const xhr = new XMLHttpRequest();
      const onAbort = () => xhr.abort();
      opts.signal?.addEventListener('abort', onAbort, { once: true });
      const done = () => opts.signal?.removeEventListener('abort', onAbort);

      xhr.open('PUT', url);
      // Must match the content type the URL was signed for, or R2 rejects the
      // signature. No Authorization header: the credential is in the URL.
      xhr.setRequestHeader('Content-Type', 'application/octet-stream');
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) onProgress?.(e.loaded / e.total);
      };
      xhr.onabort = () => {
        done();
        reject(uploadAborted());
      };
      xhr.onload = () => {
        done();
        if (xhr.status >= 200 && xhr.status < 300) resolve();
        // R2 answers with XML, not JSON — surface the status rather than trying
        // to parse it.
        else reject({ error: `R2 a respins încărcarea (HTTP ${xhr.status})` });
      };
      xhr.onerror = () => {
        done();
        // Almost always the bucket's CORS policy: a presigned PUT is executed by
        // the browser, so the bucket must allow PUT from this origin.
        reject({ error: 'Încărcarea către R2 a eșuat (verifică politica CORS a bucketului)' });
      };
      xhr.send(file);
    });

    onProgress?.(1);
    return { key, size: file.size };
  }

  /**
   * Multipart staging upload. Uses XHR because fetch has no upload progress,
   * and a 3GB video without a progress bar is indistinguishable from a hang.
   *
   * Pass `videoObjectKey` (from uploadVideoToR2) rather than `video` for
   * anything that has to cross Cloudflare — the form then carries only fields
   * and the small subtitle files, which fit inside the edge body limit.
   */
  createRelease(
    fields: {
      /** 'manga' switches to the bring-your-own-pages variant (4.6) */
      medium?: 'anime' | 'manga';
      /** either an existing catalog title… */
      animeId?: number;
      mangaId?: number;
      /** …or a proposed one — the coordinator imports & links it at publish */
      proposedTitle?: string;
      episodeNumber?: number;
      chapterNumber?: number;
      /** under ~100MiB only — see uploadVideoToR2 for anything larger */
      video?: File;
      /** object key from uploadVideoToR2; the video stays in the bucket */
      videoObjectKey?: string;
      videoSize?: number;
      sub?: File;
      roSub?: File;
      /** manga: finished RO page images (+ optional EN originals) */
      pages?: File[];
      enPages?: File[];
      /** who verifies this one; omitted = the uploader's last pick */
      verifierId?: number;
    },
    onProgress?: (fraction: number) => void
  ): Promise<{ data: Release; autoSub?: boolean }> {
    const form = new FormData();
    if (fields.medium) form.set('medium', fields.medium);
    if (fields.animeId) form.set('animeId', String(fields.animeId));
    if (fields.mangaId) form.set('mangaId', String(fields.mangaId));
    if (fields.proposedTitle) form.set('proposedTitle', fields.proposedTitle);
    if (fields.episodeNumber) form.set('episodeNumber', String(fields.episodeNumber));
    if (fields.chapterNumber) form.set('chapterNumber', String(fields.chapterNumber));
    if (fields.video) form.set('video', fields.video);
    if (fields.videoObjectKey) form.set('videoObjectKey', fields.videoObjectKey);
    if (fields.videoSize) form.set('videoSize', String(fields.videoSize));
    if (fields.sub) form.set('sub', fields.sub);
    if (fields.roSub) form.set('roSub', fields.roSub);
    for (const f of fields.pages ?? []) form.append('pages', f, f.name);
    for (const f of fields.enPages ?? []) form.append('enPages', f, f.name);
    if (fields.verifierId) form.set('verifierId', String(fields.verifierId));

    return new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      // medium rides in the query string as well as the body: the server has
      // to decide the in-flight cap before it parses a multi-gigabyte upload,
      // and the cap is anime-only.
      xhr.open('POST', `${this.baseUrl}/api/releases?medium=${fields.medium ?? 'anime'}`);
      if (this.token) xhr.setRequestHeader('Authorization', `Bearer ${this.token}`);
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable && onProgress) onProgress(e.loaded / e.total);
      };
      xhr.onload = () => {
        let body: unknown = null;
        try {
          body = JSON.parse(xhr.responseText);
        } catch {
          /* non-JSON error body */
        }
        if (xhr.status >= 200 && xhr.status < 300) resolve(body as { data: Release; autoSub?: boolean });
        else reject(body ?? { message: `Upload failed (${xhr.status})` });
      };
      xhr.onerror = () => reject({ message: 'Upload failed (network error)' });
      xhr.send(form);
    });
  }

  /**
   * Queue an optional hardsub burn — the RO track rendered into the picture.
   *
   * For hosts we can only embed: an `<iframe>` cannot carry our subtitle, so for
   * those the only way it reaches a viewer is in the pixels. Entirely opt-in —
   * publishing works exactly as before whether or not a burn ever happens.
   *
   * Idempotent: queueing something already queued or running is a no-op, so a
   * double-click cannot enqueue twice.
   */
  async queueHardsub(releaseId: number): Promise<{ message: string }> {
    return this.request(`/api/releases/${releaseId}/hardsub`, { method: 'POST' });
  }

  /** Stop a running burn, or drop a queued one. */
  async stopHardsub(releaseId: number): Promise<{ message: string }> {
    return this.request(`/api/releases/${releaseId}/hardsub`, { method: 'DELETE' });
  }

  /** State + live encode fraction + queue position, for polling. */
  async getHardsubStatus(releaseId: number): Promise<{
    data: {
      state: 'idle' | 'queued' | 'running' | 'done' | 'failed';
      progress: number;
      position: number;
      error?: string;
      ready: boolean;
    };
  }> {
    return this.request(`/api/releases/${releaseId}/hardsub`);
  }

  /**
   * State of the MKV→MP4 rewrap that runs at ingest.
   *
   * An .mkv upload cannot be played by any browser, so the preview polls this
   * and shows progress instead of a dead player until the MP4 exists. 'idle'
   * means there was nothing to convert — the common case.
   */
  async getRemuxStatus(releaseId: number): Promise<{
    data: {
      state: 'idle' | 'queued' | 'running' | 'done' | 'failed';
      progress: number;
      position: number;
      error?: string;
    };
  }> {
    return this.request(`/api/releases/${releaseId}/remux`);
  }

  async getReleaseEvents(id: number): Promise<{ data: SubtitleEvent[] }> {
    return this.request(`/api/releases/${id}/events`);
  }

  /** Kick off Claude auto-translate for the release's untranslated rows. */
  async autoTranslateRelease(id: number): Promise<{ message: string; pending: number }> {
    return this.request(`/api/releases/${id}/translate`, { method: 'POST' });
  }

  /** Progress + result of the current/last auto-translate run (for the editor's
      progress bar and error surfacing). */
  async getTranslationStatus(
    id: number
  ): Promise<{ running: boolean; total: number; filled: number; done: boolean; error?: string }> {
    return this.request(`/api/releases/${id}/translate/status`);
  }

  /** Per-row autosave from the /translate editor; marks the row human-edited. */
  async saveReleaseEvent(id: number, idx: number, roText: string): Promise<{ message: string }> {
    return this.request(`/api/releases/${id}/events/${idx}`, {
      method: 'PUT',
      body: JSON.stringify({ roText }),
    });
  }

  /**
   * Staging video URL for a native <video src>. Media elements can't send
   * headers, so the JWT rides along as ?token= (accepted only on this route).
   */
  releaseVideoUrl(id: number): string {
    const token = this.token ? `?token=${encodeURIComponent(this.token)}` : '';
    return `${this.baseUrl}/api/releases/${id}/video${token}`;
  }

  /**
   * One staged manga page for the verify flipper (1-based). Same ?token=
   * mechanism as the video: <img src> can't send headers.
   */
  releasePageUrl(id: number, lang: 'ro' | 'en', idx: number): string {
    const token = this.token ? `?token=${encodeURIComponent(this.token)}` : '';
    return `${this.baseUrl}/api/releases/${id}/page/${lang}/${idx}${token}`;
  }

  /** Manga releases: the uploader replaces one staged edition after review notes. */
  async reuploadReleasePages(
    id: number,
    lang: 'ro' | 'en',
    files: File[]
  ): Promise<{ message: string; language: string; count: number }> {
    const form = new FormData();
    form.set('lang', lang);
    for (const f of files) form.append('pages', f, f.name);
    const res = await fetch(`${this.baseUrl}/api/releases/${id}/pages`, {
      method: 'POST',
      headers: this.token ? { Authorization: `Bearer ${this.token}` } : {},
      body: form,
    });
    const body = await res.json();
    if (!res.ok) throw body;
    return body;
  }

  /** The RO draft as WebVTT text — turned into a blob URL for the live <track>. */
  async getReleaseDraftVtt(id: number): Promise<string> {
    const res = await fetch(`${this.baseUrl}/api/releases/${id}/draft.vtt`, {
      headers: this.token ? { Authorization: `Bearer ${this.token}` } : {},
    });
    if (!res.ok) throw new Error(`draft.vtt failed (${res.status})`);
    return res.text();
  }

  async submitRelease(id: number): Promise<{ message: string; translated: number; total: number }> {
    return this.request(`/api/releases/${id}/submit`, { method: 'POST' });
  }

  async approveRelease(id: number): Promise<{ message: string }> {
    return this.request(`/api/releases/${id}/approve`, { method: 'POST' });
  }

  async requestReleaseChanges(id: number, notes: string): Promise<{ message: string }> {
    return this.request(`/api/releases/${id}/request-changes`, {
      method: 'POST',
      body: JSON.stringify({ notes }),
    });
  }

  /**
   * Coordinator/admin: publish an approved release — confirm/override the
   * series & episode mapping and optionally attach the episode's sources.
   */
  async publishRelease(
    id: number,
    body: {
      animeId?: number;
      episodeNumber?: number;
      /** manga releases publish to a chapter instead */
      mangaId?: number;
      chapterNumber?: number;
      sources?: { hostingUrl: string; kind?: 'embed' | 'extract'; provider?: string; providerRef?: string; quality?: string; language?: string }[];
    } = {}
  ): Promise<{ message: string; episodeId?: number; animeId?: number; chapterId?: number; mangaId?: number }> {
    return this.request(`/api/releases/${id}/publish`, {
      method: 'POST',
      body: JSON.stringify(body),
    });
  }

  async deleteRelease(id: number): Promise<{ message: string }> {
    return this.request(`/api/releases/${id}`, { method: 'DELETE' });
  }

  // ── Chapters ──────────────────────────────────────────────────────────────

  async getChapters(mangaId: number): Promise<{ data: Chapter[] }> {
    return this.request(`/api/manga/${mangaId}/chapters`);
  }

  async getChapter(mangaId: number, chapterNumber: number): Promise<{ data: Chapter & { links: ContentLink[] } }> {
    return this.request(`/api/manga/${mangaId}/chapters/${chapterNumber}`);
  }

  async createChapter(mangaId: number, data: Partial<Chapter>): Promise<{ data: Chapter }> {
    return this.request(`/api/manga/${mangaId}/chapters`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async updateChapter(mangaId: number, chapterNumber: number, data: Partial<Chapter>): Promise<{ data: Chapter }> {
    return this.request(`/api/manga/${mangaId}/chapters/${chapterNumber}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  async deleteChapter(mangaId: number, chapterNumber: number): Promise<{ message: string }> {
    return this.request(`/api/manga/${mangaId}/chapters/${chapterNumber}`, {
      method: 'DELETE',
    });
  }

  async addChapterLink(
    chapterId: number,
    data: {
      hostingUrl: string;
      quality?: string;
      language?: string;
      kind?: 'embed' | 'extract';
      provider?: string;
      providerRef?: string;
      priority?: number;
    }
  ): Promise<{ data: ContentLink }> {
    return this.request(`/api/chapters/${chapterId}/links`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  // ── Token management ──────────────────────────────────────────────────────

  // The token lives in localStorage for the browser's own calls and is mirrored
  // into a cookie for the server's. SSR load functions run before any
  // JavaScript, so localStorage is invisible to them — and with the catalog now
  // behind requireAuth, an SSR load with no credential gets a 401 and the page
  // fails. hooks.server.ts reads this cookie and attaches it to server-to-server
  // requests; see the note there.
  //
  // Not httpOnly, deliberately: the same value already sits in localStorage, so
  // marking the cookie httpOnly would buy nothing against XSS while breaking
  // the one thing it is for. Scoped SameSite=Lax so it rides top-level
  // navigations (which is exactly when SSR needs it) but not cross-site posts.
  setToken(token: string): void {
    this.token = token;
    if (typeof window !== 'undefined') {
      localStorage.setItem('auth_token', token);
      const secure = location.protocol === 'https:' ? '; Secure' : '';
      // 7d, matching the backend's default JWT_EXPIRES_IN. An expired cookie
      // outliving its token only means SSR gets a 401 it already handles.
      document.cookie = `ak_token=${encodeURIComponent(token)}; Path=/; Max-Age=604800; SameSite=Lax${secure}`;
    }
  }

  clearToken(): void {
    this.token = null;
    if (typeof window !== 'undefined') {
      localStorage.removeItem('auth_token');
      const secure = location.protocol === 'https:' ? '; Secure' : '';
      document.cookie = `ak_token=; Path=/; Max-Age=0; SameSite=Lax${secure}`;
    }
  }

  getToken(): string | null {
    return this.token;
  }

  isAuthenticated(): boolean {
    return !!this.token;
  }

  // ── Community ─────────────────────────────────────────────────────────────

  async getCommunityMembers(): Promise<{ data: CommunityMember[] }> {
    return this.request('/api/community/members');
  }

  async getCommunityReviews(): Promise<{ data: CommunityReview[] }> {
    return this.request('/api/community/reviews');
  }

  async getCommunityActivity(): Promise<{ data: ActivityEvent[]; friends: number }> {
    return this.request('/api/community/activity');
  }

  async getCommunityStats(): Promise<CommunityStats> {
    return this.request('/api/community/stats');
  }

  async getCommunityTeam(): Promise<{ data: CommunityTeamMember[] }> {
    return this.request('/api/community/team');
  }

  async getHallOfFame(window: 'month' | 'all' = 'all'): Promise<HallOfFame> {
    return this.request(`/api/community/hall-of-fame?window=${window}`);
  }

  async getEpisodeCredits(animeId: number, num: number): Promise<{ data: ReleaseCredits }> {
    return this.request(`/api/anime/${animeId}/episodes/${num}/credits`);
  }

  async getChapterCredits(mangaId: number, num: string | number): Promise<{ data: ReleaseCredits }> {
    return this.request(`/api/manga/${mangaId}/chapters/${num}/credits`);
  }

  async getForumThreads(category?: string): Promise<{ data: ForumThread[] }> {
    const q = category && category !== 'Toate' ? `?category=${encodeURIComponent(category)}` : '';
    return this.request(`/api/community/forum${q}`);
  }

  async getForumThread(id: number): Promise<{ data: { thread: ForumThread; replies: ForumReply[] } }> {
    return this.request(`/api/community/forum/${id}`);
  }

  async createForumThread(body: { category: string; title: string; body: string }): Promise<{ data: ForumThread }> {
    return this.request('/api/community/forum', { method: 'POST', body: JSON.stringify(body) });
  }

  async createForumReply(id: number, body: string): Promise<{ data: ForumReply }> {
    return this.request(`/api/community/forum/${id}/replies`, { method: 'POST', body: JSON.stringify({ body }) });
  }

  async pinForumThread(id: number, pinned: boolean): Promise<{ message: string }> {
    return this.request(`/api/community/forum/${id}/pin`, { method: 'POST', body: JSON.stringify({ pinned }) });
  }

  async lockForumThread(id: number, locked: boolean): Promise<{ message: string }> {
    return this.request(`/api/community/forum/${id}/lock`, { method: 'POST', body: JSON.stringify({ locked }) });
  }

  async deleteForumThread(id: number): Promise<{ message: string }> {
    return this.request(`/api/community/forum/${id}`, { method: 'DELETE' });
  }

  // ── Weekly programme ("Programul săptămânii") ─────────────────────────────

  /** `upcoming` returns the whole plan ahead (the admin editor); otherwise a
      window of `days` from the start of today (what /home draws). */
  async getSchedule(opts: { days?: number; upcoming?: boolean } = {}): Promise<{
    data: ScheduleSlot[];
  }> {
    const p = new URLSearchParams();
    if (opts.days) p.set('days', String(opts.days));
    if (opts.upcoming) p.set('upcoming', '1');
    const q = p.toString();
    return this.request(`/api/schedule${q ? `?${q}` : ''}`);
  }

  async createScheduleSlot(body: ScheduleSlotInput): Promise<{ data: ScheduleSlot }> {
    return this.request('/api/schedule', { method: 'POST', body: JSON.stringify(body) });
  }

  async updateScheduleSlot(
    id: number,
    body: Omit<ScheduleSlotInput, 'animeId'>
  ): Promise<{ data: ScheduleSlot }> {
    return this.request(`/api/schedule/${id}`, { method: 'PUT', body: JSON.stringify(body) });
  }

  async deleteScheduleSlot(id: number): Promise<{ message: string }> {
    return this.request(`/api/schedule/${id}`, { method: 'DELETE' });
  }

  // ── Custom chat emotes ────────────────────────────────────────────────────

  /** `all` includes disabled ones — admin/coordinator only, ignored otherwise. */
  async getEmotes(all = false): Promise<{ data: Emote[] }> {
    return this.request(`/api/emotes${all ? '?all=1' : ''}`);
  }

  async createEmote(code: string, file: File): Promise<{ data: Emote }> {
    const form = new FormData();
    form.append('code', code);
    form.append('image', file);
    return this.request('/api/emotes', { method: 'POST', body: form });
  }

  async setEmoteActive(id: number, isActive: boolean): Promise<{ data: Emote }> {
    return this.request(`/api/emotes/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ isActive })
    });
  }

  async deleteEmote(id: number): Promise<{ message: string }> {
    return this.request(`/api/emotes/${id}`, { method: 'DELETE' });
  }

  // ── GIFs (Giphy, proxied + cached server-side) ────────────────────────────

  /** Empty `q` returns trending. 503 = no API key configured; 429 = the free
      tier's hourly quota is spent, which only affects *finding* a new GIF. */
  async searchGifs(q = ''): Promise<{ data: GiphyGif[] }> {
    const p = new URLSearchParams();
    if (q.trim()) p.set('q', q.trim());
    const qs = p.toString();
    return this.request(`/api/gifs${qs ? `?${qs}` : ''}`);
  }

  // ── Announcements ("Știri & anunțuri") ────────────────────────────────────

  /** `drafts` also returns unpublished rows — admin/moderator only, ignored
      for everyone else so the editor and the home feed share one endpoint. */
  async getAnnouncements(opts: { limit?: number; drafts?: boolean } = {}): Promise<{
    data: Announcement[];
  }> {
    const p = new URLSearchParams();
    if (opts.limit) p.set('limit', String(opts.limit));
    if (opts.drafts) p.set('drafts', '1');
    const q = p.toString();
    return this.request(`/api/announcements${q ? `?${q}` : ''}`);
  }

  async createAnnouncement(body: AnnouncementInput): Promise<{ data: Announcement }> {
    return this.request('/api/announcements', { method: 'POST', body: JSON.stringify(body) });
  }

  async updateAnnouncement(id: number, body: AnnouncementInput): Promise<{ data: Announcement }> {
    return this.request(`/api/announcements/${id}`, { method: 'PUT', body: JSON.stringify(body) });
  }

  async getAnnouncement(idOrSlug: string | number): Promise<{ data: Announcement }> {
    return this.request(`/api/announcements/${encodeURIComponent(String(idOrSlug))}`);
  }

  async getAnnouncementComments(
    id: number,
    page = 1,
    limit = 20
  ): Promise<{ data: CommentType[]; pagination: any }> {
    return this.request(`/api/announcements/${id}/comments?page=${page}&limit=${limit}`);
  }

  async postAnnouncementComment(id: number, content: string): Promise<{ data: CommentType }> {
    return this.request(`/api/announcements/${id}/comments`, {
      method: 'POST',
      body: JSON.stringify({ content })
    });
  }

  /** Cover / in-body image upload; returns the stored /uploads/ path. */
  async uploadAnnouncementImage(file: File): Promise<{ imageUrl: string }> {
    const form = new FormData();
    form.append('image', file);
    return this.request('/api/announcements/image', { method: 'POST', body: form });
  }

  async deleteAnnouncement(id: number): Promise<{ message: string }> {
    return this.request(`/api/announcements/${id}`, { method: 'DELETE' });
  }

  // ── Notifications ─────────────────────────────────────────────────────────

  async getNotifications(): Promise<{ data: Notification[]; unread: number }> {
    return this.request('/api/notifications');
  }

  async getUnreadCount(): Promise<{ unread: number }> {
    return this.request('/api/notifications/unread-count');
  }

  async markAllNotificationsRead(): Promise<{ message: string }> {
    return this.request('/api/notifications/read-all', { method: 'POST' });
  }

  async markNotificationRead(id: number): Promise<{ message: string }> {
    return this.request(`/api/notifications/${id}/read`, { method: 'POST' });
  }

  /**
   * Resolve a relative URL (like /uploads/avatars/...) to a full backend URL
   */
  resolveUrl(path: string): string {
    return mediaUrl(path);
  }

  /** Build an authed URL for a file download. `<a download>` can't send an
      Authorization header, so the token rides in the query (the /releases routes
      accept it via TokenFromQuery). */
  fileUrl(path: string): string {
    const t = typeof localStorage !== 'undefined' ? localStorage.getItem('auth_token') : null;
    if (!t) return `${this.baseUrl}${path}`;
    const sep = path.includes('?') ? '&' : '?';
    return `${this.baseUrl}${path}${sep}token=${encodeURIComponent(t)}`;
  }

  // ── Helpers ───────────────────────────────────────────────────────────────

  private buildParams(filters?: Record<string, any>): string {
    if (!filters) return '';
    const params = new URLSearchParams();
    Object.entries(filters).forEach(([key, value]) => {
      if (value !== undefined && value !== null) {
        if (Array.isArray(value)) value.forEach(v => params.append(key, String(v)));
        else params.append(key, String(value));
      }
    });
    return params.toString();
  }
}

export const api = new ApiClient(API_BASE_URL);
export default api;
