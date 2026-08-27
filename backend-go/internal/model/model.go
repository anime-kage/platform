// Package model defines the API's JSON shapes. Field names and optionality
// mirror shared/types.ts exactly — that file remains the cross-boundary
// contract, and the SvelteKit frontend must not notice the Go rewrite.
// db tags are scany column mappings; dotted prefixes ("anime.id") scan
// joined columns into the nested struct.
package model

import "time"

type User struct {
	ID             int           `db:"id" json:"id"`
	Username       string        `db:"username" json:"username"`
	Email          string        `db:"email" json:"email,omitempty"`
	AvatarURL      *string       `db:"avatar_url" json:"avatarUrl,omitempty"`
	Bio            *string       `db:"bio" json:"bio,omitempty"`
	FavoriteGenres []string      `db:"favorite_genres" json:"favoriteGenres"`
	Favorites      []FavoriteRef `db:"-" json:"favorites"`
	Role           string        `db:"role" json:"role"`
	CreatedAt      time.Time     `db:"created_at" json:"createdAt"`
	UpdatedAt      time.Time     `db:"updated_at" json:"updatedAt"`
}

type FavoriteRef struct {
	Type string `json:"type"` // "anime" | "manga"
	ID   int    `json:"id"`
}

type Anime struct {
	ID            int      `db:"id" json:"id"`
	MalID         *int     `db:"mal_id" json:"malId,omitempty"`
	Title         string   `db:"title" json:"title"`
	TitleEnglish  *string  `db:"title_english" json:"titleEnglish,omitempty"`
	TitleRomanian *string  `db:"title_romanian" json:"titleRomanian,omitempty"`
	Synopsis      *string  `db:"synopsis" json:"synopsis,omitempty"`
	SynopsisRo    *string  `db:"synopsis_romanian" json:"synopsisRomanian,omitempty"`
	Genres        []string `db:"genres" json:"genres"`
	Studios       []string `db:"studios" json:"studios"`
	Status        string   `db:"status" json:"status"`
	Type          string   `db:"type" json:"type"`
	Episodes      *int     `db:"episodes" json:"episodes,omitempty"`
	Score         *float64 `db:"score" json:"score,omitempty"`
	Year          *int     `db:"year" json:"year,omitempty"`
	Season        *string  `db:"season" json:"season,omitempty"`
	ImageURL      *string  `db:"image_url" json:"imageUrl,omitempty"`
	TrailerURL    *string  `db:"trailer_url" json:"trailerUrl,omitempty"`
	BroadcastDay  *string  `db:"broadcast_day" json:"broadcastDay,omitempty"`
	BroadcastTime *string  `db:"broadcast_time" json:"broadcastTime,omitempty"`
	// URL slug ("91-days"). NULL until the backfill has run; every link falls
	// back to the numeric id, so an absent slug is never a broken page.
	Slug      *string   `db:"slug" json:"slug,omitempty"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt time.Time `db:"updated_at" json:"updatedAt"`
}

// Normalize replaces nil slices with empty ones — the old backend always
// returned arrays, and frontend code iterates them unguarded.
func (a *Anime) Normalize() {
	if a.Genres == nil {
		a.Genres = []string{}
	}
	if a.Studios == nil {
		a.Studios = []string{}
	}
}

type Manga struct {
	ID            int      `db:"id" json:"id"`
	MalID         *int     `db:"mal_id" json:"malId,omitempty"`
	Title         string   `db:"title" json:"title"`
	TitleEnglish  *string  `db:"title_english" json:"titleEnglish,omitempty"`
	TitleRomanian *string  `db:"title_romanian" json:"titleRomanian,omitempty"`
	Synopsis      *string  `db:"synopsis" json:"synopsis,omitempty"`
	SynopsisRo    *string  `db:"synopsis_romanian" json:"synopsisRomanian,omitempty"`
	Genres        []string `db:"genres" json:"genres"`
	Authors       []string `db:"authors" json:"authors"`
	Status        string   `db:"status" json:"status"`
	Type          string   `db:"type" json:"type"`
	Chapters      *int     `db:"chapters" json:"chapters,omitempty"`
	Volumes       *int     `db:"volumes" json:"volumes,omitempty"`
	Score         *float64 `db:"score" json:"score,omitempty"`
	Year          *int     `db:"year" json:"year,omitempty"`
	ImageURL      *string  `db:"image_url" json:"imageUrl,omitempty"`
	// URL slug, same contract as Anime.Slug
	Slug      *string   `db:"slug" json:"slug,omitempty"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt time.Time `db:"updated_at" json:"updatedAt"`
}

func (m *Manga) Normalize() {
	if m.Genres == nil {
		m.Genres = []string{}
	}
	if m.Authors == nil {
		m.Authors = []string{}
	}
}

type Episode struct {
	ID            int     `db:"id" json:"id"`
	AnimeID       int     `db:"anime_id" json:"animeId"`
	EpisodeNumber int     `db:"episode_number" json:"episodeNumber"`
	Title         *string `db:"title" json:"title,omitempty"`
	AirDate       *string `db:"air_date" json:"airDate,omitempty"` // "YYYY-MM-DD", matching the SQL date column
	Duration      *int    `db:"duration" json:"duration,omitempty"`
	Synopsis      *string `db:"synopsis" json:"synopsis,omitempty"`
	// MAL's filler/recap marks, editable by the team (see migration 0036)
	IsFiller  bool          `db:"is_filler" json:"isFiller"`
	IsRecap   bool          `db:"is_recap" json:"isRecap"`
	Links     []ContentLink `db:"-" json:"links"`
	CreatedAt time.Time     `db:"created_at" json:"createdAt"`
}

// PublishedRelease is a published RO-subtitle release paired with its catalog
// title — the homepage "Ultimele lansări" feed. We never publish without a RO
// track, so "latest released" == latest published release (anime or manga).
type PublishedRelease struct {
	ID            int       `json:"id"`
	Medium        string    `json:"medium"` // 'anime' | 'manga'
	EpisodeNumber *int      `json:"episodeNumber,omitempty"`
	ChapterNumber *string   `json:"chapterNumber,omitempty"`
	PublishedAt   time.Time `json:"publishedAt"`
	Anime         *Anime    `json:"anime,omitempty"`
	Manga         *Manga    `json:"manga,omitempty"`
}

type Chapter struct {
	ID            int           `db:"id" json:"id"`
	MangaID       int           `db:"manga_id" json:"mangaId"`
	ChapterNumber string        `db:"chapter_number" json:"chapterNumber"`
	Title         *string       `db:"title" json:"title,omitempty"`
	ReleaseDate   *string       `db:"release_date" json:"releaseDate,omitempty"` // "YYYY-MM-DD"
	Pages         *int          `db:"pages" json:"pages,omitempty"`
	Links         []ContentLink `db:"-" json:"links"`
	CreatedAt     time.Time     `db:"created_at" json:"createdAt"`
}

// ContentLink is a playback/reading source. kind='embed' is the
// iframe fallback; kind='extract' is resolved to an HLS manifest by the
// resolver service via provider+providerRef. lastCheckedAt/lastOk come from
// the source health checker.
type ContentLink struct {
	ID            int        `db:"id" json:"id"`
	EpisodeID     *int       `db:"episode_id" json:"episodeId,omitempty"`
	ChapterID     *int       `db:"chapter_id" json:"chapterId,omitempty"`
	HostingURL    string     `db:"hosting_url" json:"hostingUrl"`
	Quality       *string    `db:"quality" json:"quality,omitempty"`
	Language      string     `db:"language" json:"language"`
	IsActive      bool       `db:"is_active" json:"isActive"`
	Kind          string     `db:"kind" json:"kind"`
	Provider      *string    `db:"provider" json:"provider,omitempty"`
	ProviderRef   *string    `db:"provider_ref" json:"providerRef,omitempty"`
	Priority      int        `db:"priority" json:"priority"`
	LastCheckedAt *time.Time `db:"last_checked_at" json:"lastCheckedAt,omitempty"`
	LastOK        *bool      `db:"last_ok" json:"lastOk,omitempty"`
}

// Subtitle is our own track for an episode — served as <track>
// in our player. Only status='published' rows reach the stream response;
// machine/edited are release-pipeline drafts.
type Subtitle struct {
	ID           int       `db:"id" json:"id"`
	EpisodeID    int       `db:"episode_id" json:"episodeId"`
	Language     string    `db:"language" json:"language"`
	Label        *string   `db:"label" json:"label,omitempty"`
	Format       string    `db:"format" json:"format"`
	URL          string    `db:"url" json:"url"`
	Status       string    `db:"status" json:"status"`
	TranslatorID *int      `db:"translator_id" json:"translatorId,omitempty"`
	SourceSub    *string   `db:"source_sub" json:"sourceSub,omitempty"`
	CreatedAt    time.Time `db:"created_at" json:"createdAt"`
}

// SkipMark is an intro/outro range for an episode. source
// 'manual' is coordinator-set and never overwritten; 'aniskip' is a cached
// AniSkip hit.
type SkipMark struct {
	ID        int       `db:"id" json:"id"`
	EpisodeID int       `db:"episode_id" json:"episodeId"`
	Kind      string    `db:"kind" json:"kind"`
	StartS    float64   `db:"start_s" json:"start"`
	EndS      float64   `db:"end_s" json:"end"`
	Source    string    `db:"source" json:"source"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
}

// Release is one translator's episode in flight through the pipeline
//: video + EN sub in staging, RO draft
// as subtitle_events rows. StagingPath is server-internal — never in JSON.
type Release struct {
	ID int `db:"id" json:"id"`
	// what kind of release this is: 'anime' (episode: video + subtitle
	// events) or 'manga' (chapter: finished page images —)
	Medium string `db:"medium" json:"medium"`
	// nil until the series exists in the catalog — the translator proposes a
	// title and the coordinator links the real one at publish time
	AnimeID       *int     `db:"anime_id" json:"animeId,omitempty"`
	MangaID       *int     `db:"manga_id" json:"mangaId,omitempty"`
	ProposedTitle *string  `db:"proposed_title" json:"proposedTitle,omitempty"`
	EpisodeNumber *int     `db:"episode_number" json:"episodeNumber,omitempty"`
	ChapterNumber *float64 `db:"chapter_number" json:"chapterNumber,omitempty"`
	UploaderID    int      `db:"uploader_id" json:"uploaderId"`
	ReviewerID    *int     `db:"reviewer_id" json:"reviewerId,omitempty"`
	// who this release is routed to for verification — a soft rule: it
	// filters queues, and coordinators/admins can always act and reassign
	AssignedVerifierID *int    `db:"assigned_verifier_id" json:"assignedVerifierId,omitempty"`
	State              string  `db:"state" json:"state"`
	StagingPath        *string `db:"staging_path" json:"-"`
	// R2Key is set when the video lives in the bucket rather than on local disk.
	// Exactly one of R2Key / StagingPath carries the source video.
	R2Key       *string   `db:"r2_key" json:"-"`
	ReviewNotes *string   `db:"review_notes" json:"reviewNotes,omitempty"`
	CreatedAt   time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt   time.Time `db:"updated_at" json:"updatedAt"`
	// joined for list/queue views
	AnimeTitle           *string `db:"anime_title" json:"animeTitle,omitempty"`
	AnimeImage           *string `db:"anime_image" json:"animeImage,omitempty"`
	MangaTitle           *string `db:"manga_title" json:"mangaTitle,omitempty"`
	MangaImage           *string `db:"manga_image" json:"mangaImage,omitempty"`
	UploaderName         *string `db:"uploader_name" json:"uploaderName,omitempty"`
	ReviewerName         *string `db:"reviewer_name" json:"reviewerName,omitempty"`
	AssignedVerifierName *string `db:"assigned_verifier_name" json:"assignedVerifierName,omitempty"`
	// per-release translation progress (subqueried alongside the row)
	TotalEvents int `db:"total_events" json:"totalEvents"`
	DoneEvents  int `db:"done_events" json:"doneEvents"`
	// derived: whether the staging video is still on disk (drafts yes,
	// published/expired no)
	HasVideo bool `db:"-" json:"hasVideo"`
	// derived (manga): staging page counts per edition
	PageCount   int `db:"-" json:"pageCount,omitempty"`
	EnPageCount int `db:"-" json:"enPageCount,omitempty"`
	// derived: an auto-translate run is currently filling this release's rows
	Translating bool `db:"-" json:"translating"`
}

// SubtitleEvent is one line of a release's subtitle draft. Edited marks
// human-touched rows — auto-translate (4.3) only fills roText = ”.
type SubtitleEvent struct {
	ID        int    `db:"id" json:"id"`
	ReleaseID int    `db:"release_id" json:"releaseId"`
	Idx       int    `db:"idx" json:"idx"`
	StartMs   int    `db:"start_ms" json:"startMs"`
	EndMs     int    `db:"end_ms" json:"endMs"`
	EnText    string `db:"en_text" json:"enText"`
	RoText    string `db:"ro_text" json:"roText"`
	Edited    bool   `db:"edited" json:"edited"`
}

// UserList is a curated custom list ("Liste") — editorial collections,
// distinct from the status-tracking watchlist/readlist.
type UserList struct {
	ID          int       `db:"id" json:"id"`
	UserID      int       `db:"user_id" json:"userId"`
	Title       string    `db:"title" json:"title"`
	Description *string   `db:"description" json:"description,omitempty"`
	IsPublic    bool      `db:"is_public" json:"isPublic"`
	CreatedAt   time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt   time.Time `db:"updated_at" json:"updatedAt"`
	OwnerName   string    `db:"owner_name" json:"ownerName"`
	OwnerAvatar *string   `db:"owner_avatar_url" json:"ownerAvatarUrl,omitempty"`
	ItemCount   int       `db:"item_count" json:"itemCount"`
	Covers      []string  `db:"covers" json:"covers"` // first item posters, list-card fans
	LikeCount   int       `db:"like_count" json:"likeCount"`
	Liked       bool      `db:"liked" json:"liked"` // whether the requesting viewer liked it
}

// TranslationRequest is a member's ask for a series to be subtitled ("Cereri"),
// with its community vote tally and (once resolved) canonical MAL identity.
type TranslationRequest struct {
	ID            int       `db:"id" json:"id"`
	UserID        int       `db:"user_id" json:"userId"`
	Medium        string    `db:"medium" json:"medium"` // 'anime' | 'manga'
	MalID         *int      `db:"mal_id" json:"malId,omitempty"`
	Title         string    `db:"title" json:"title"`
	ImageURL      *string   `db:"image_url" json:"imageUrl,omitempty"`
	Note          *string   `db:"note" json:"note,omitempty"`
	Status        string    `db:"status" json:"status"`
	CreatedAt     time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt     time.Time `db:"updated_at" json:"updatedAt"`
	RequesterName string    `db:"requester_name" json:"requesterName"`
	VoteCount     int       `db:"vote_count" json:"voteCount"`
	Voted         bool      `db:"voted" json:"voted"` // whether the requesting viewer voted
}

// UserListItem is one title on a custom list — exactly one of AnimeID/MangaID
// is set (DB CHECK); the display fields are joined from the catalog row.
type UserListItem struct {
	ID       int       `db:"id" json:"id"`
	ListID   int       `db:"list_id" json:"listId"`
	AnimeID  *int      `db:"anime_id" json:"animeId,omitempty"`
	MangaID  *int      `db:"manga_id" json:"mangaId,omitempty"`
	Note     *string   `db:"note" json:"note,omitempty"`
	Position int       `db:"position" json:"position"`
	AddedAt  time.Time `db:"added_at" json:"addedAt"`

	Title         string   `db:"title" json:"title"`
	TitleRomanian *string  `db:"title_romanian" json:"titleRomanian,omitempty"`
	ImageURL      *string  `db:"image_url" json:"imageUrl,omitempty"`
	Year          *int     `db:"year" json:"year,omitempty"`
	Score         *float64 `db:"score" json:"score,omitempty"`
	Genres        []string `db:"genres" json:"genres"`
}

type WatchlistEntry struct {
	ID              int        `db:"id" json:"id"`
	UserID          int        `db:"user_id" json:"userId"`
	AnimeID         int        `db:"anime_id" json:"animeId"`
	Status          string     `db:"status" json:"status"`
	Score           *int       `db:"score" json:"score,omitempty"`
	EpisodesWatched int        `db:"episodes_watched" json:"episodesWatched"`
	Notes           *string    `db:"notes" json:"notes,omitempty"`
	StartedAt       *time.Time `db:"started_at" json:"startedAt,omitempty"`
	CompletedAt     *time.Time `db:"completed_at" json:"completedAt,omitempty"`
	UpdatedAt       time.Time  `db:"updated_at" json:"updatedAt"`
	Anime           Anime      `db:"anime" json:"anime"`

	// What WE actually have, as opposed to Anime.Episodes, which is the
	// series total from MAL and counts episodes that may not exist here yet.
	// AvailableEpisodes counts the ones with a playable source; NextEpisode is
	// the lowest such number still ahead of the viewer, nil when caught up.
	AvailableEpisodes int  `db:"available_episodes" json:"availableEpisodes"`
	NextEpisode       *int `db:"next_episode" json:"nextEpisode,omitempty"`
}

// ContinueEntry is one card in the "Continuă vizionarea" row.
//
// It is driven by playback, not by the watchlist: the row answers "where was
// I?", and the honest answer lives in playback_positions. Deriving it from
// watchlist.status instead used to hide two ordinary cases — a series you
// never explicitly added (status stays 'plan-to-watch'), and an episode you
// are part-way through but have not finished.
type ContinueEntry struct {
	Anime Anime `db:"anime" json:"anime"`
	// the episode to open: the unfinished one you left off in, else the next
	// one you have not seen
	EpisodeID     int `db:"episode_id" json:"episodeId"`
	EpisodeNumber int `db:"episode_number" json:"episodeNumber"`
	// Resume point inside that episode, and how long it runs. Both are zero
	// for a fresh episode — the player treats 0 as "from the start".
	PositionS float64  `db:"position_s" json:"positionS"`
	DurationS *float64 `db:"duration_s" json:"durationS,omitempty"`
	// episodes of this series we have published, and how many are behind you
	AvailableEpisodes int       `db:"available_episodes" json:"availableEpisodes"`
	WatchedEpisodes   int       `db:"watched_episodes" json:"watchedEpisodes"`
	LastActivity      time.Time `db:"last_activity" json:"lastActivity"`
}

type ReadlistEntry struct {
	ID           int        `db:"id" json:"id"`
	UserID       int        `db:"user_id" json:"userId"`
	MangaID      int        `db:"manga_id" json:"mangaId"`
	Status       string     `db:"status" json:"status"`
	Score        *int       `db:"score" json:"score,omitempty"`
	ChaptersRead int        `db:"chapters_read" json:"chaptersRead"`
	VolumesRead  int        `db:"volumes_read" json:"volumesRead"`
	Notes        *string    `db:"notes" json:"notes,omitempty"`
	StartedAt    *time.Time `db:"started_at" json:"startedAt,omitempty"`
	CompletedAt  *time.Time `db:"completed_at" json:"completedAt,omitempty"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updatedAt"`
	Manga        Manga      `db:"manga" json:"manga"`
}

// CommentUser is the embedded author on comments and reviews.
type CommentUser struct {
	ID        int     `db:"id" json:"id"`
	Username  string  `db:"username" json:"username"`
	AvatarURL *string `db:"avatar_url" json:"avatarUrl,omitempty"`
}

type Comment struct {
	ID              int         `db:"id" json:"id"`
	UserID          int         `db:"user_id" json:"userId"`
	AnimeID         *int        `db:"anime_id" json:"animeId,omitempty"`
	MangaID         *int        `db:"manga_id" json:"mangaId,omitempty"`
	EpisodeID       *int        `db:"episode_id" json:"episodeId,omitempty"`
	ChapterID       *int        `db:"chapter_id" json:"chapterId,omitempty"`
	ParentID        *int        `db:"parent_id" json:"parentId,omitempty"`
	RootID          *int        `db:"root_id" json:"rootId,omitempty"`
	ReplyToUsername *string     `db:"reply_to_username" json:"replyToUsername,omitempty"`
	ReplyToExcerpt  *string     `db:"reply_to_excerpt" json:"replyToExcerpt,omitempty"`
	Content         string      `db:"content" json:"content"`
	LikesCount      int         `db:"likes_count" json:"likesCount"`
	DislikesCount   int         `db:"dislikes_count" json:"dislikesCount"`
	RepliesCount    int         `db:"replies_count" json:"repliesCount"`
	UserVote        *string     `db:"user_vote" json:"userVote"`
	CreatedAt       time.Time   `db:"created_at" json:"createdAt"`
	UpdatedAt       time.Time   `db:"updated_at" json:"updatedAt"`
	User            CommentUser `db:"user" json:"user"`
}

type Review struct {
	EntryID    int         `db:"entry_id" json:"entryId"`
	UserID     int         `db:"user_id" json:"userId"`
	Score      *int        `db:"score" json:"score,omitempty"`
	Notes      string      `db:"notes" json:"notes"`
	UpdatedAt  time.Time   `db:"updated_at" json:"updatedAt"`
	ReplyCount int         `db:"reply_count" json:"replyCount"`
	User       CommentUser `db:"user" json:"user"`
}

type FollowNetwork struct {
	Followers   int  `json:"followers"`
	Following   int  `json:"following"`
	IsFollowing bool `json:"isFollowing"`
}

type FollowUser struct {
	ID             int     `db:"id" json:"id"`
	Username       string  `db:"username" json:"username"`
	Bio            *string `db:"bio" json:"bio,omitempty"`
	AvatarURL      *string `db:"avatar_url" json:"avatarUrl,omitempty"`
	Role           string  `db:"role" json:"role"`
	FollowersCount int     `db:"followers_count" json:"followersCount"`
	IsFollowing    bool    `db:"is_following" json:"isFollowing"`
}

// UserReviewTitle is the compact title block on profile reviews.
type UserReviewTitle struct {
	ID            int     `db:"id" json:"id"`
	Title         string  `db:"title" json:"title"`
	TitleRomanian *string `db:"title_romanian" json:"titleRomanian,omitempty"`
	ImageURL      *string `db:"image_url" json:"imageUrl,omitempty"`
	Year          *int    `db:"year" json:"year,omitempty"`
}

type UserReview struct {
	Kind       string          `db:"kind" json:"kind"` // "anime" | "manga"
	EntryID    int             `db:"entry_id" json:"entryId"`
	Score      *int            `db:"score" json:"score,omitempty"`
	Notes      string          `db:"notes" json:"notes"`
	UpdatedAt  time.Time       `db:"updated_at" json:"updatedAt"`
	ReplyCount int             `db:"reply_count" json:"replyCount"`
	Title      UserReviewTitle `db:"title" json:"title"`
}

type HistoryDay struct {
	Date     string `db:"date" json:"date"`
	Episodes int    `db:"episodes" json:"episodes"`
	Chapters int    `db:"chapters" json:"chapters"`
}

type UserStats struct {
	TotalAnimeWatched    int     `json:"totalAnimeWatched"`
	TotalEpisodesWatched int     `json:"totalEpisodesWatched"`
	TotalHoursWatched    int     `json:"totalHoursWatched"`
	TotalMangaRead       int     `json:"totalMangaRead"`
	TotalChaptersRead    int     `json:"totalChaptersRead"`
	AverageAnimeScore    float64 `json:"averageAnimeScore"`
	AverageMangaScore    float64 `json:"averageMangaScore"`
}

// ── Community (/comunitate) ──────────────────────────────────────────────────

// CommunityMember is a real account as it appears on the Members tab: profile
// plus the counts that matter socially, and whether the viewer follows them.
type CommunityMember struct {
	ID          int     `db:"id" json:"id"`
	Username    string  `db:"username" json:"username"`
	AvatarURL   *string `db:"avatar_url" json:"avatarUrl,omitempty"`
	Bio         *string `db:"bio" json:"bio,omitempty"`
	Role        string  `db:"role" json:"role"`
	Followers   int     `db:"followers" json:"followers"`
	ReviewCount int     `db:"review_count" json:"reviewCount"`
	ListCount   int     `db:"list_count" json:"listCount"`
	IsFollowing bool    `db:"is_following" json:"isFollowing"`
}

// CommunityReview is one member's review (a list entry with notes) as it shows
// on the Reviews tab — the same content as a profile review, plus the author.
type CommunityReview struct {
	Kind       string          `db:"kind" json:"kind"` // "anime" | "manga"
	EntryID    int             `db:"entry_id" json:"entryId"`
	Score      *int            `db:"score" json:"score,omitempty"`
	Notes      string          `db:"notes" json:"notes"`
	UpdatedAt  time.Time       `db:"updated_at" json:"updatedAt"`
	ReplyCount int             `db:"reply_count" json:"replyCount"`
	User       CommentUser     `db:"user" json:"user"`
	Title      UserReviewTitle `db:"title" json:"title"`
}

// ActivityEvent is one line of the friends-only Activity feed. Verb is a
// pre-rendered Romanian phrase; Meta is an optional trailing badge ("★ 9").
type ActivityEvent struct {
	Type      string      `json:"type"` // review | list | thread
	User      CommentUser `json:"user"`
	Verb      string      `json:"verb"`
	Target    string      `json:"target"`
	Link      *string     `json:"link,omitempty"`
	Meta      *string     `json:"meta,omitempty"`
	CreatedAt time.Time   `json:"createdAt"`
}

// CommunityStats are the sidebar numbers: members, published Romanian subtitle
// tracks, and the catalog size (anime + manga).
type CommunityStats struct {
	Members   int `json:"members"`
	Subtitles int `json:"subtitles"`
	Titles    int `json:"titles"`
	// Online is members seen in the last few minutes — the community page
	// shows this instead of Subtitles, which reads as noise on a catalog that
	// is mostly imported rather than translated by the team.
	Online int `json:"online"`
}

// HallEntry is one person's standing on the translator/verifier hall of fame:
// how many releases they published (as uploader) or verified (as reviewer),
// within the chosen time window.
type HallEntry struct {
	UserID    int     `db:"user_id" json:"userId"`
	Username  string  `db:"username" json:"username"`
	AvatarURL *string `db:"avatar_url" json:"avatarUrl,omitempty"`
	Role      string  `db:"role" json:"role"`
	Count     int     `db:"count" json:"count"`
}

// ReleaseCredits names the people behind a published episode/chapter: the
// translator (the release's uploader), the verifier (its reviewer) and the
// coordinator who published it. Any may be nil — nothing published yet, an
// approval with no recorded reviewer, or a release published before
// published_by existed.
type ReleaseCredits struct {
	Translator  *CommentUser `json:"translator"`
	Verifier    *CommentUser `json:"verifier"`
	Coordinator *CommentUser `json:"coordinator"`
}

// TeamMember is a staff account on the Echipă tab: who holds a role above
// plain 'user', with the profile bits the card shows.
type TeamMember struct {
	ID        int       `db:"id" json:"id"`
	Username  string    `db:"username" json:"username"`
	AvatarURL *string   `db:"avatar_url" json:"avatarUrl,omitempty"`
	Bio       *string   `db:"bio" json:"bio,omitempty"`
	Role      string    `db:"role" json:"role"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
}

// ForumThread is a persisted forum topic. Body is included on the detail
// endpoint and omitted from the list. Author is joined for the byline/avatar.
type ForumThread struct {
	ID             int         `db:"id" json:"id"`
	Category       string      `db:"category" json:"category"`
	Title          string      `db:"title" json:"title"`
	Body           string      `db:"body" json:"body,omitempty"`
	IsPinned       bool        `db:"is_pinned" json:"isPinned"`
	IsLocked       bool        `db:"is_locked" json:"isLocked"`
	ReplyCount     int         `db:"reply_count" json:"replyCount"`
	LastActivityAt time.Time   `db:"last_activity_at" json:"lastActivityAt"`
	CreatedAt      time.Time   `db:"created_at" json:"createdAt"`
	Author         CommentUser `db:"author" json:"author"`
}

// Emote is a custom chat emote. Width/Height are the uploaded image's natural
// size — the chat renders at a fixed height regardless (that is what makes a
// mixed bag of uploads look uniform), so these exist for the admin list to warn
// about proportions that will render badly.
type Emote struct {
	ID        int       `db:"id" json:"id"`
	Code      string    `db:"code" json:"code"`
	ImageURL  string    `db:"image_url" json:"imageUrl"`
	Width     int       `db:"width" json:"width"`
	Height    int       `db:"height" json:"height"`
	IsActive  bool      `db:"is_active" json:"isActive"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
}

// ScheduleSlot is one entry of the team-decided weekly programme: this episode
// of this series goes live at this instant. The anime fields are joined so a
// row renders without a second lookup.
type ScheduleSlot struct {
	ID            int       `db:"id" json:"id"`
	AnimeID       int       `db:"anime_id" json:"animeId"`
	EpisodeNumber int       `db:"episode_number" json:"episodeNumber"`
	ScheduledAt   time.Time `db:"scheduled_at" json:"scheduledAt"`
	Note          *string   `db:"note" json:"note,omitempty"`
	CreatedByName *string   `db:"created_by_name" json:"createdByName,omitempty"`

	Title         string  `db:"title" json:"title"`
	TitleEnglish  *string `db:"title_english" json:"titleEnglish,omitempty"`
	TitleRomanian *string `db:"title_romanian" json:"titleRomanian,omitempty"`
	ImageURL      *string `db:"image_url" json:"imageUrl,omitempty"`
	// whether that episode already exists on the site, so the UI can link to
	// it once it is live instead of only to the series
	Published bool `db:"published" json:"published"`
}

// RelatedAnime is one neighbour of a series — the next season, the previous
// one, or an alternative retelling. Only titles that exist in our catalog are
// ever built into one of these, so every field is real and every card links
// somewhere. Relation is "" for the anime at the centre of its own chain.
type RelatedAnime struct {
	Relation      string  `db:"relation" json:"relation"`
	ID            int     `db:"id" json:"id"`
	Title         string  `db:"title" json:"title"`
	TitleEnglish  *string `db:"title_english" json:"titleEnglish,omitempty"`
	TitleRomanian *string `db:"title_romanian" json:"titleRomanian,omitempty"`
	ImageURL      *string `db:"image_url" json:"imageUrl,omitempty"`
	Year          *int    `db:"year" json:"year,omitempty"`
	Type          string  `db:"type" json:"type"`
	Status        string  `db:"status" json:"status"`
	Episodes      *int    `db:"episodes" json:"episodes,omitempty"`
	// carried so the season strip and the player's season jump link to the
	// pretty URL instead of bouncing through the numeric-id redirect
	Slug *string `db:"slug" json:"slug,omitempty"`
	// how many episodes we actually host, which is what decides whether a
	// "watch the next season" link can go anywhere
	EpisodeCount int `db:"episode_count" json:"episodeCount"`
}

// AnimeRelations is the payload behind the season strip and the related grid.
// Chain is the PREQUEL→SEQUEL run in watch order (empty when the series stands
// alone); Related is everything that does not linearise — alternatives,
// spin-offs, side stories.
type AnimeRelations struct {
	Chain   []RelatedAnime `json:"chain"`
	Related []RelatedAnime `json:"related"`
}

// Announcement is one entry of the "Știri & anunțuri" strip on /home, written
// by the team. AuthorName is joined for the admin list only — the home card
// shows the tag and the date, not who typed it.
type Announcement struct {
	ID    int    `db:"id" json:"id"`
	Tag   string `db:"tag" json:"tag"`
	Title string `db:"title" json:"title"`
	// The post itself: a Markdown subset (see frontend lib/markdown.ts). Stored
	// as written — it is rendered to elements, never to an HTML string, so no
	// sanitising step stands between the database and the page.
	Body         *string   `db:"body" json:"body,omitempty"`
	CoverURL     *string   `db:"cover_url" json:"coverUrl,omitempty"`
	Slug         *string   `db:"slug" json:"slug,omitempty"`
	URL          *string   `db:"url" json:"url,omitempty"`
	IsPublished  bool      `db:"is_published" json:"isPublished"`
	AuthorName   *string   `db:"author_name" json:"authorName,omitempty"`
	CommentCount int       `db:"comment_count" json:"commentCount"`
	CreatedAt    time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time `db:"updated_at" json:"updatedAt"`
}

// ForumReply is one message under a thread.
type ForumReply struct {
	ID        int         `db:"id" json:"id"`
	Body      string      `db:"body" json:"body"`
	CreatedAt time.Time   `db:"created_at" json:"createdAt"`
	Author    CommentUser `db:"author" json:"author"`
}

// Notification is one row of the in-app inbox. Body is pre-rendered Romanian;
// Actor carries the triggering user's name for the avatar (null for system
// events). Unread mirrors read_at IS NULL. The frontend computes the relative
// timestamp from CreatedAt so it stays fresh without a refetch.
type Notification struct {
	ID        int       `db:"id" json:"id"`
	Type      string    `db:"type" json:"type"`
	Body      string    `db:"body" json:"text"`
	Link      *string   `db:"link" json:"link,omitempty"`
	Actor     *string   `db:"actor" json:"actor,omitempty"`
	Unread    bool      `db:"unread" json:"unread"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
}

// ChatMessage is one live-chat line. Role travels with the message
// so the panel can badge the author without a second lookup — and so a badge
// reflects the role at read time, not a stale copy written at send time.
//
// ReplyToUser/ReplyToExcerpt are a snapshot of what was answered, not a link:
// the quote must not change or disappear when the original does.
type ChatMessage struct {
	ID             int64     `db:"id" json:"id"`
	Body           string    `db:"body" json:"body"`
	ReplyToUser    *string   `db:"reply_to_user" json:"replyToUser,omitempty"`
	ReplyToExcerpt *string   `db:"reply_to_excerpt" json:"replyToExcerpt,omitempty"`
	// The message this one answers, so the client can scroll to it. Nil for
	// replies sent before this existed, and for replies whose target has since
	// been deleted — the quote text still renders, it just is not clickable.
	ReplyToID *int64 `db:"reply_to_id" json:"replyToId,omitempty"`
	CreatedAt      time.Time `db:"created_at" json:"createdAt"`
	UserID         int       `db:"user_id" json:"userId"`
	Username       string    `db:"username" json:"username"`
	Role           string    `db:"role" json:"role"`
	AvatarURL      *string   `db:"avatar_url" json:"avatarUrl,omitempty"`
}

// ChatRestriction is a live-chat timeout or ban — the room's own mute list,
// not an account suspension (see the 0021 migration). A nil ExpiresAt is a
// permanent ban; a timestamp is a timeout that lapses by itself.
type ChatRestriction struct {
	UserID    int        `db:"user_id" json:"userId"`
	Username  string     `db:"username" json:"username"`
	ExpiresAt *time.Time `db:"expires_at" json:"expiresAt,omitempty"`
	Reason    *string    `db:"reason" json:"reason,omitempty"`
	ByName    *string    `db:"by_name" json:"byName,omitempty"`
	CreatedAt time.Time  `db:"created_at" json:"createdAt"`
}

// Permanent reports whether this is a ban rather than a timeout.
func (c ChatRestriction) Permanent() bool { return c.ExpiresAt == nil }

// Invite is one single-use registration code. Minted by the
// Discord bot, redeemed by POST /api/auth/register when INVITE_ONLY is on.
// The Discord id is a snowflake and so is text, not an int.
type Invite struct {
	ID              int        `db:"id" json:"id"`
	Code            string     `db:"code" json:"code"`
	DiscordUserID   string     `db:"discord_user_id" json:"discordUserId"`
	DiscordUsername *string    `db:"discord_username" json:"discordUsername,omitempty"`
	CreatedAt       time.Time  `db:"created_at" json:"createdAt"`
	ExpiresAt       *time.Time `db:"expires_at" json:"expiresAt,omitempty"`
	UsedByUserID    *int       `db:"used_by_user_id" json:"usedByUserId,omitempty"`
	UsedAt          *time.Time `db:"used_at" json:"usedAt,omitempty"`
}

// Claimed reports whether the code has already been spent.
func (i Invite) Claimed() bool { return i.UsedAt != nil }

// EpisodeReport is one member's note that something is wrong with an episode —
// a dead source, bad skip markers, a mismatched video. Carries enough series
// context for the moderation queue to link straight to the episode, so a
// moderator never has to go looking for which show this was.
type EpisodeReport struct {
	ID            int        `db:"id" json:"id"`
	EpisodeID     int        `db:"episode_id" json:"episodeId"`
	EpisodeNumber int        `db:"episode_number" json:"episodeNumber"`
	AnimeID       int        `db:"anime_id" json:"animeId"`
	AnimeSlug     *string    `db:"anime_slug" json:"animeSlug,omitempty"`
	AnimeTitle    string     `db:"anime_title" json:"animeTitle"`
	Body          string     `db:"body" json:"body"`
	Status        string     `db:"status" json:"status"`
	Reporter      *string    `db:"reporter" json:"reporter,omitempty"`
	CreatedAt     time.Time  `db:"created_at" json:"createdAt"`
	ResolvedAt    *time.Time `db:"resolved_at" json:"resolvedAt,omitempty"`
}
