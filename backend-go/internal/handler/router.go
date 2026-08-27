package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/go-chi/cors"

	"animekage/backend/internal/httpx"
	mw "animekage/backend/internal/middleware"
)

// Routes assembles the full application: middleware, health, static uploads,
// and every /api route. Paths and response shapes mirror the old backend.
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	// NOT chimw.RealIP. It overwrites RemoteAddr from the LEFTMOST
	// X-Forwarded-For — or X-Real-IP, or True-Client-IP — with no trust check
	// whatsoever, which chi's own docs warn about. That let any client pick its
	// own rate-limit bucket by sending a header, defeating the login and import
	// limiters below, and let anyone forge the IP recorded against a password
	// reset request. Caddy sanitizes X-Forwarded-For but NOT X-Real-IP, so
	// sitting behind the proxy did not save us. httpx.ClientIP does the same
	// job with an explicit trust boundary (cfg.TrustedProxies).
	// mw.Logger, not chimw.Logger: chi's prints the full request URI, and two
	// routes carry the session JWT in the query because EventSource and
	// <video>/<track> cannot send an Authorization header. That wrote live
	// credentials into the container log. mw.Logger masks them.
	r.Use(mw.Logger)
	r.Use(chimw.Recoverer)
	// After Recoverer so a panic is still counted (as the 500 it becomes), and
	// inside the router so chi has matched a route pattern by the time the
	// metric is recorded.
	r.Use(mw.Metrics)
	r.Use(mw.SecurityHeaders)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   h.cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(mw.RateLimit(300, 100, h.cfg.TrustedProxies)) // global per-IP ceiling

	r.Get("/health", h.health)

	// Prometheus scrapes this over the Docker network. Deliberately NOT routed
	// by nginx (which proxies only /api/, /health, /uploads/ and /), so it is
	// not reachable from the internet — request paths and latencies are
	// operational detail, not public data.
	r.Handle("/metrics", promhttp.Handler())

	// uploaded files (avatars); directory browsing off
	fs := http.StripPrefix("/uploads/", http.FileServer(http.Dir(h.cfg.UploadsDir)))
	r.Get("/uploads/*", func(w http.ResponseWriter, req *http.Request) {
		fs.ServeHTTP(w, req)
	})

	// the DB role lookup makes role changes (5.5) bite on the next request
	// instead of waiting for the JWT to be reissued at next login
	requireAuth := mw.RequireAuth(h.auth, h.repo.UserRole, h.repo.TouchLastSeen)
	optionalAuth := mw.OptionalAuth(h.auth, h.repo.UserRole)
	contentRole := mw.RequireRole("admin", "translator")
	// catalog imports are the coordinator's tool (4.5b) — deliberately NOT
	// translators, so the catalog can't grow duplicates or wrong titles
	importRole := mw.RequireRole("admin", "coordinator")
	// a translator uploading a release for a series we don't have yet may pull
	// that one MAL/AniList title into the catalog on the spot (same search the
	// Cereri page offers everyone); bulk/manual catalog tools stay coordinator-only
	importSeriesRole := mw.RequireRole("admin", "coordinator", "translator")
	// manual field edits on catalog entries (descriptions, RO titles, posters)
	editRole := mw.RequireRole("admin", "coordinator", "translator")
	adminOnly := mw.RequireRole("admin")
	// who decides what the front page shows (5.4) — the same people who
	// decide what enters the catalog, plus moderators
	curateRole := mw.RequireRole("admin", "coordinator", "moderator")
	// the /verify review gate — verifier is a dedicated role;
	// moderators/admins count as coordinators
	verifierRole := mw.RequireRole("admin", "moderator", "verifier")
	// live-chat moderation (8.6) — staff moderate the room they sit in; the
	// rank rule in chatmod.go stops it being used sideways or upwards
	chatModRole := mw.RequireRole(chatModRoles...)

	r.Route("/api", func(r chi.Router) {
		r.Use(optionalAuth)

		r.Get("/", h.apiIndex)
		// ── What stays readable without a session ──────────────────────────
		// The site is invite-only (July 2026), so everything below this block
		// requires a token. Exactly three things do not, because the front door
		// cannot work without them:
		//
		//   /config    — the register page must know whether to ask for a code
		//                before it renders
		//   /landing   — the three collage covers on the public landing page
		//   /auth/*    — register, login, forgot-password, reset-password
		//
		// /health and /uploads/* sit outside this group and are also open: the
		// first is the container healthcheck, the second is static media whose
		// URLs the landing collage embeds.
		//
		// Adding a route here widens what an unauthenticated scraper can read.
		r.Get("/config", h.publicConfig)
		r.Get("/landing", h.landing)

		// editorially curated placements. Read used to be public
		// because the landing page assembled its collage from it; /landing now
		// does that server-side, so this is members-only like the rest of the
		// catalog. Writing is the coordinator's job, same gate as imports.
		r.Route("/curated", func(r chi.Router) {
			r.Use(requireAuth)
			r.Get("/", h.listCurated)
			r.With(requireAuth, curateRole).Post("/image", h.uploadCuratedImage)
			r.With(requireAuth, curateRole).Put("/{slot}", h.replaceCurated)
		})

		// homepage feed: latest published RO-subtitle releases (anime + manga).
		// Only /home reads it, and /home is behind the login now.
		r.With(requireAuth).Get("/recent-releases", h.recentReleases)

		r.Route("/auth", func(r chi.Router) {
			// login/register get a much tighter bucket: brute force was the
			// old backend's biggest hole
			auth := r.With(mw.RateLimit(10, 10, h.cfg.TrustedProxies))
			auth.Post("/register", h.register)
			auth.Post("/login", h.login)
			// same tight bucket: both are unauthenticated and both are
			// worth guessing at — one enumerates accounts, one guesses tokens
			auth.Post("/forgot-password", h.forgotPassword)
			auth.Post("/reset-password", h.resetPassword)
			r.With(requireAuth).Get("/me", h.me)
			r.With(requireAuth).Post("/logout", h.logout)
		})

		r.Route("/anime", func(r chi.Router) {
			// Invite-only: the catalog is not public. The role-gated writes
			// below keep their own requireAuth so each line still reads as its
			// own contract; the duplication is deliberate and harmless.
			r.Use(requireAuth)
			r.Get("/", h.listAnime)
			r.Get("/search", h.searchAnime)
			r.Get("/schedule", h.schedule)
			r.Get("/random", h.randomAnime)
			r.Get("/most-watched", h.mostWatched)
			r.Get("/trending", h.trendingAnime)
			r.Get("/airing", h.airingAnime)
			r.Get("/season/{year}/{season}", h.seasonalAnime)
			r.Get("/{id}", h.animeByID)
			r.Get("/{id}/reviews", h.animeReviews)

			// catalog writes are role-gated (the old backend forgot this)
			r.With(requireAuth, importSeriesRole).Get("/mal-search", h.malSearchAnime)
			r.With(requireAuth, importSeriesRole).Post("/import/{malId}", h.importAnime)
			r.With(requireAuth, importRole).Post("/", h.createAnimeManual)
			r.With(requireAuth, importRole).Post("/{id}/poster", h.uploadAnimePoster)
			r.With(requireAuth, contentRole).Put("/{id}/update", h.updateAnime)
			r.With(requireAuth, editRole).Put("/{id}", h.patchAnime)
			r.With(requireAuth, adminOnly).Delete("/{id}", h.deleteAnime)

			// season chain + franchise grid (relations synced from AniList)
			r.Get("/{id}/relations", h.animeRelations)

			r.Get("/{id}/episodes", h.listEpisodes)
			r.Get("/{id}/episodes/{num}", h.episodeByNumber)
			r.Get("/{id}/episodes/{num}/credits", h.episodeCredits)
			r.With(requireAuth, contentRole).Post("/{id}/episodes", h.createEpisode)
			// pull titles / air dates / filler marks for one series from MAL —
			// the nightly job only polls airing series, so a completed one
			// added by hand needs this
			r.With(requireAuth, contentRole).Post("/{id}/episodes/sync", h.syncEpisodesFromMAL)
			// editRole, not contentRole: a coordinator who can rewrite the
			// series description can rewrite an episode's too
			r.With(requireAuth, editRole).Put("/{id}/episodes/{num}", h.updateEpisode)
			r.With(requireAuth, adminOnly).Delete("/{id}/episodes/{num}", h.deleteEpisode)

			r.Get("/{id}/comments", h.listAnimeComments)
			r.With(requireAuth).Post("/{id}/comments", h.postAnimeComment)
		})

		r.Route("/manga", func(r chi.Router) {
			r.Use(requireAuth) // invite-only, as /anime
			r.Get("/", h.listManga)
			r.Get("/search", h.searchManga)
			r.Get("/trending", h.trendingManga)
			r.Get("/publishing", h.publishingManga)
			r.Get("/{id}", h.mangaByID)
			r.Get("/{id}/reviews", h.mangaReviews)

			r.With(requireAuth, importSeriesRole).Get("/mal-search", h.malSearchManga)
			r.With(requireAuth, importSeriesRole).Post("/import/{malId}", h.importManga)
			r.With(requireAuth, importRole).Post("/", h.createMangaManual)
			r.With(requireAuth, importRole).Post("/{id}/poster", h.uploadMangaPoster)
			r.With(requireAuth, contentRole).Put("/{id}/update", h.updateManga)
			r.With(requireAuth, editRole).Put("/{id}", h.patchManga)
			r.With(requireAuth, adminOnly).Delete("/{id}", h.deleteManga)

			r.Get("/{id}/chapters", h.listChapters)
			r.Get("/{id}/chapters/{num}", h.chapterByNumber)
			r.Get("/{id}/chapters/{num}/credits", h.chapterCredits)
			r.With(requireAuth, contentRole).Post("/{id}/chapters", h.createChapter)
			r.With(requireAuth, contentRole).Put("/{id}/chapters/{num}", h.updateChapter)
			r.With(requireAuth, adminOnly).Delete("/{id}/chapters/{num}", h.deleteChapter)

			r.Get("/{id}/comments", h.listMangaComments)
			r.With(requireAuth).Post("/{id}/comments", h.postMangaComment)
		})

		// Invite-only, like the catalog: resolving a stream source, reading skip
		// marks or listing our subtitle tracks are all member-only now. These sit
		// directly under /api rather than in a Route group, so they were missed
		// when /anime and /manga were closed — the effect was that anyone could
		// still resolve a playable stream URL without an account.
		//
		// TokenFromQuery on the stream and page routes: a <video>, <track> or
		// <img> element cannot send an Authorization header, so the player passes
		// the JWT in the query string for exactly these.
		r.With(mw.TokenFromQuery, requireAuth).Get("/episodes/{id}/stream", h.episodeStream)
		r.With(requireAuth).Get("/episodes/{id}/progress", h.getProgress)
		r.With(requireAuth).Put("/episodes/{id}/progress", h.saveProgress)
		// one view per member per episode; what the home leaderboards rank by
		r.With(requireAuth).Post("/episodes/{id}/view", h.recordEpisodeView)
		// Free-text and member-submitted, so it gets its own tight bucket
		// rather than the global one — a report form is easy to sit on.
		r.With(requireAuth, mw.RateLimit(10, 5, h.cfg.TrustedProxies)).
			Post("/episodes/{id}/report", h.reportEpisode)
		r.With(requireAuth).Get("/episodes/{id}/skip", h.episodeSkip)
		r.With(requireAuth, contentRole).Post("/episodes/{id}/skip", h.setSkipMark)
		r.With(requireAuth, contentRole).Delete("/episodes/{id}/skip/{kind}", h.deleteSkipMark)
		r.With(requireAuth).Get("/episodes/{id}/subtitles", h.listSubtitles)
		r.With(requireAuth, contentRole).Post("/episodes/{id}/subtitles", h.addSubtitle)
		// hand-attached track for episodes the release pipeline never touched
		r.With(requireAuth, contentRole).Post("/episodes/{id}/subtitles/upload", h.uploadSubtitle)
		// staff download of a published track as SubRip. TokenFromQuery is
		// scoped to this one route: an <a download> can't send a header.
		r.With(mw.TokenFromQuery, requireAuth, contentRole).
			Get("/episodes/{id}/subtitles.srt", h.episodeSubtitleSRT)
		r.With(requireAuth, contentRole).Delete("/subtitles/{id}", h.deleteSubtitle)
		r.With(requireAuth).Get("/chapters/{id}/pages", h.chapterPages)
		r.With(mw.TokenFromQuery, requireAuth).Get("/chapters/{id}/pageimg/{idx}", h.chapterPageImage)
		r.With(requireAuth, contentRole).Put("/chapters/{id}/pages", h.setChapterPages)
		r.With(requireAuth, contentRole).Post("/chapters/{id}/pages/upload", h.uploadChapterPages)
		r.With(requireAuth, contentRole).Delete("/chapters/{id}/pages", h.deleteChapterPages)
		r.With(requireAuth, contentRole).Post("/episodes/{id}/links", h.addEpisodeLink)
		r.With(requireAuth, contentRole).Post("/chapters/{id}/links", h.addChapterLink)
		r.With(requireAuth, contentRole).Put("/links/{id}", h.updateLink)
		r.With(requireAuth, contentRole).Delete("/links/{id}", h.deleteLink)

		// admin panel
		r.Route("/admin", func(r chi.Router) {
			r.Use(requireAuth)
			r.With(contentRole).Post("/test-source", h.testSource)
			r.With(importRole).Post("/import-season/{year}/{season}", h.importSeason)
			r.With(contentRole).Get("/health-report", h.adminHealthReport)
			r.With(contentRole).Get("/storage", h.adminStorage)
			r.With(contentRole).Get("/health-gaps", h.adminHealthGaps)
			r.With(contentRole).Get("/episodes/{id}/links", h.adminEpisodeLinks)

			// moderation queue + panel overview/team
			modRole := mw.RequireRole("admin", "moderator")
			r.With(modRole).Get("/overview", h.adminOverview)
			r.With(modRole).Get("/team", h.adminTeam)
			r.With(modRole).Get("/reports", h.listReports)
			r.With(modRole).Get("/episode-reports", h.listEpisodeReports)
			r.With(modRole).Post("/episode-reports/{id}/resolve", h.resolveEpisodeReport)
			r.With(modRole).Post("/comments/{id}/dismiss", h.dismissReport)
			r.With(modRole).Delete("/comments/{id}", h.modDeleteComment)
			r.With(modRole).Get("/users", h.findUsers)
			r.With(modRole).Post("/users/{id}/ban", h.banUser)
			r.With(adminOnly).Put("/users/{id}/role", h.changeUserRole)
			r.With(adminOnly).Put("/users/{id}/release-cap", h.setUserReleaseCap)
		})

		// release pipeline: staging uploads, the editor's event
		// grid, and the verify gate. Visibility (own vs all) is enforced in
		// the handlers; these gates are the role floor.
		// Chunked video upload (see handler/uploads.go). Separate from /releases
		// because it runs before a release exists: Cloudflare caps bodies at
		// 100 MiB, so a 3 GB episode arrives as a run of sub-limit appends and
		// the release form then references the assembled file by id.
		r.Route("/uploads/video", func(r chi.Router) {
			r.Use(requireAuth, contentRole)
			// direct-to-R2: the browser PUTs straight to the bucket, so neither
			// this server's disk nor Cloudflare's 100 MiB body limit is involved.
			r.Post("/presign", h.presignVideoUpload)
		})

		r.Route("/releases", func(r chi.Router) {
			// TokenFromQuery first: the editor's <video>/<track> elements
			// can't send an Authorization header
			r.Use(mw.TokenFromQuery, requireAuth)
			r.With(contentRole).Post("/", h.createRelease)
			r.With(contentRole).Get("/quota", h.releaseQuotaHandler)
			r.Get("/", h.listReleases)
			r.Get("/verifiers", h.listVerifiers)
			r.Put("/{id}/verifier", h.assignVerifier)
			r.Get("/{id}", h.getRelease)
			r.Get("/{id}/events", h.releaseEvents)
			r.Put("/{id}/events/{idx}", h.updateReleaseEvent)
			r.With(contentRole).Post("/{id}/translate", h.translateRelease)
			r.Get("/{id}/translate/status", h.translationStatusHandler)
			r.Get("/{id}/video", h.releaseVideo)
			// MKV→MP4 rewrap. Read-only and not content-gated: the
			// translator page polls this to know when the preview will play, and
			// the uploader is usually not a coordinator. loadRelease already
			// enforces who may see the release at all.
			r.Get("/{id}/remux", h.remuxStatus)
			r.Get("/{id}/subtitle.srt", h.releaseSubtitleSRT)
			r.Get("/{id}/download", h.releaseDownload)        // video + RO soft-sub (.mkv), to keep
			r.Get("/{id}/download.mp4", h.releaseDownloadMP4) // video only, to upload to a host
			// Optional hardsub: the burned copy, for hosts we can only
			// embed. The publish flow works with or without one.
			r.With(contentRole).Post("/{id}/hardsub", h.queueHardsub)
			r.With(contentRole).Get("/{id}/hardsub", h.hardsubStatus)
			r.With(contentRole).Delete("/{id}/hardsub", h.stopHardsub)
			r.With(contentRole).Get("/{id}/download.hardsub.mp4", h.releaseDownloadHardsub)
			r.Get("/{id}/page/{lang}/{idx}", h.releasePage)
			r.With(contentRole).Post("/{id}/pages", h.reuploadReleasePages)
			r.Get("/{id}/draft.vtt", h.releaseDraftVTT)
			r.Post("/{id}/submit", h.submitRelease)
			r.With(verifierRole).Post("/{id}/approve", h.approveRelease)
			r.With(verifierRole).Post("/{id}/request-changes", h.requestChanges)
			// the publish gate (4.5b) — coordinator/admin, checked in-handler
			r.Post("/{id}/publish", h.publishRelease)
			r.Delete("/{id}", h.deleteRelease)
		})

		// community hub (/comunitate): real member list, all-users reviews,
		// friends-only activity, stats, team roster, persisted forum
		// live chat. Members only — the panel never renders for
		// guests, and a public firehose is not what this is.
		r.Route("/chat", func(r chi.Router) {
			// TokenFromQuery first: EventSource cannot send an Authorization
			// header, so the stream carries its token in the URL.
			r.Use(mw.TokenFromQuery, requireAuth)
			r.Get("/messages", h.chatMessages)
			r.Post("/messages", h.sendChatMessage)
			r.Delete("/messages/{id}", h.deleteChatMessage)
			r.Get("/stream", h.chatStream)

			// timeouts/bans scoped to this room only — never the account
			r.Route("/restrictions", func(r chi.Router) {
				r.Use(chatModRole)
				r.Get("/{username}", h.chatRestriction)
				r.Put("/{username}", h.setChatRestriction)
				r.Delete("/{username}", h.clearChatRestriction)
			})
		})

		r.Route("/community", func(r chi.Router) {
			r.Use(requireAuth)                    // members, reviews, forum — none of it is public now
			r.Get("/members", h.communityMembers) // personalises isFollowing
			r.Get("/reviews", h.communityReviews)
			r.Get("/stats", h.communityStats)
			r.Get("/team", h.communityTeam)
			r.Get("/hall-of-fame", h.communityHallOfFame)
			r.With(requireAuth).Get("/activity", h.communityActivity)

			r.Route("/forum", func(r chi.Router) {
				r.Get("/", h.listForumThreads)
				r.Get("/{id}", h.getForumThread)
				r.With(requireAuth).Post("/", h.createForumThread)
				r.With(requireAuth).Post("/{id}/replies", h.createForumReply)
				r.With(requireAuth).Delete("/{id}", h.deleteForumThread) // author or mod, checked in-handler
				modRole := mw.RequireRole("admin", "moderator")
				r.With(requireAuth, modRole).Post("/{id}/pin", h.pinForumThread)
				r.With(requireAuth, modRole).Post("/{id}/lock", h.lockForumThread)
			})
		})

		// the weekly programme ("Programul săptămânii") — every member reads
		// it, admins and coordinators decide what is in it. Deliberately the
		// same bar as catalog imports: both are "what the site is showing this
		// week" decisions.
		r.Route("/schedule", func(r chi.Router) {
			r.Use(requireAuth)
			r.Get("/", h.listSchedule)
			r.With(importRole).Post("/", h.createScheduleSlot)
			r.With(importRole).Put("/{id}", h.updateScheduleSlot)
			r.With(importRole).Delete("/{id}", h.deleteScheduleSlot)
		})

		// custom chat emotes. Any member reads them (the chat renders them and
		// the picker lists them); admins and coordinators manage them.
		r.Route("/emotes", func(r chi.Router) {
			r.Use(requireAuth)
			emoteRole := mw.RequireRole("admin", "coordinator")
			r.Get("/", h.listEmotes)
			r.With(emoteRole).Post("/", h.createEmote)
			r.With(emoteRole).Patch("/{id}", h.updateEmote)
			r.With(emoteRole).Delete("/{id}", h.deleteEmote)
		})

		// GIF picker (comments, reviews, posts, chat). Members only, and on a
		// tight per-IP bucket of its own: the upstream free tier is 100 calls
		// an hour, so one person holding down the search box must not be able
		// to spend everyone else's.
		r.With(requireAuth, mw.RateLimit(30, 15, h.cfg.TrustedProxies)).
			Get("/gifs", h.searchGifs)

		// site announcements — the "Știri & anunțuri" strip on /home. Any
		// member reads; admin/moderator writes (same bar as pinning a thread).
		r.Route("/announcements", func(r chi.Router) {
			r.Use(requireAuth)
			announceRole := mw.RequireRole("admin", "moderator")
			r.Get("/", h.listAnnouncements)
			r.Get("/{id}", h.getAnnouncement) // id or slug — the post's own page
			r.Get("/{id}/comments", h.listAnnouncementComments)
			r.With(requireAuth).Post("/{id}/comments", h.postAnnouncementComment)
			r.With(announceRole).Post("/", h.createAnnouncement)
			r.With(announceRole).Post("/image", h.uploadAnnouncementImage)
			r.With(announceRole).Put("/{id}", h.updateAnnouncement)
			r.With(announceRole).Delete("/{id}", h.deleteAnnouncement)
		})

		// in-app notifications — the header bell + /notificari inbox
		r.Route("/notifications", func(r chi.Router) {
			r.Use(requireAuth)
			r.Get("/", h.listNotifications)
			r.Get("/unread-count", h.notificationsUnread)
			r.Post("/read-all", h.markAllNotificationsRead)
			r.Post("/{id}/read", h.markNotificationRead)
		})

		// custom user lists ("Liste") — member browse + owner CRUD
		r.Route("/lists", func(r chi.Router) {
			r.Use(requireAuth)
			r.Get("/", h.publicUserLists)
			r.With(requireAuth).Post("/", h.createUserList)
			r.With(requireAuth).Get("/mine", h.myUserLists)
			r.Get("/{id}", h.getUserList) // public or owner (optionalAuth)
			r.With(requireAuth).Post("/{id}/like", h.likeList)
			r.With(requireAuth).Delete("/{id}/like", h.unlikeList)
			r.With(requireAuth).Put("/{id}", h.updateUserList)
			r.With(requireAuth).Delete("/{id}", h.deleteUserList)
			r.With(requireAuth).Post("/{id}/items", h.addUserListItem)
			r.With(requireAuth).Put("/{id}/items/{itemId}", h.updateUserListItem)
			r.With(requireAuth).Delete("/{id}/items/{itemId}", h.removeUserListItem)
		})

		// translation requests ("Cereri") — member browse and submit/vote,
		// coordinator/admin status changes
		r.Route("/requests", func(r chi.Router) {
			r.Use(requireAuth)
			r.Get("/", h.listRequests) // personalises `voted`
			r.With(requireAuth).Get("/search", h.searchRequests)
			r.With(requireAuth).Post("/", h.createRequest)
			r.With(requireAuth).Post("/{id}/vote", h.voteRequest)
			r.With(requireAuth).Delete("/{id}/vote", h.unvoteRequest)
			r.With(requireAuth, importRole).Patch("/{id}/status", h.setRequestStatus)
		})

		r.Route("/users", func(r chi.Router) {
			// Profiles, lists, ratings and history are Letterboxd-style visible
			// to *members*, not to the internet: on an invite-only site "public
			// profile" means public within the community.
			r.Use(requireAuth)

			// /me/* is static, so chi matches it before /{username}
			r.Route("/me", func(r chi.Router) {
				r.Use(requireAuth)
				r.Get("/", h.myProfile)
				r.Put("/", h.updateMyProfile)
				r.Post("/avatar", h.uploadAvatar)
				r.Get("/history", h.myHistory)

				// list import. Tight bucket: each call fans out
				// to AniList or parses a multi-MB upload, so it is far more
				// expensive than a normal write.
				imp := r.With(mw.RateLimit(6, 3, h.cfg.TrustedProxies))
				imp.Post("/import/anilist", h.importAniList)
				imp.Post("/import/mal", h.importMAL)

				// profile backdrop
				r.Get("/banner/options", h.myBannerOptions)
				r.Put("/banner", h.setMyBanner)

				r.Get("/continue", h.myContinueWatching)

				r.Get("/watchlist", h.myWatchlist)
				r.Post("/watchlist", h.upsertWatchlist)
				r.Get("/watchlist/{animeId}", h.myWatchlistEntry)
				r.Put("/watchlist/{animeId}", h.updateWatchlist)
				r.Delete("/watchlist/{animeId}", h.removeWatchlist)

				r.Get("/readlist", h.myReadlist)
				r.Post("/readlist", h.upsertReadlist)
				r.Get("/readlist/{mangaId}", h.myReadlistEntry)
				r.Put("/readlist/{mangaId}", h.updateReadlist)
				r.Delete("/readlist/{mangaId}", h.removeReadlist)
			})

			r.Get("/{username}", h.publicProfile)
			r.With(requireAuth).Post("/{username}/follow", h.follow)
			r.With(requireAuth).Delete("/{username}/follow", h.unfollow)
			r.Get("/{username}/followers", func(w http.ResponseWriter, req *http.Request) {
				h.followList(w, req, "followers")
			})
			r.Get("/{username}/following", func(w http.ResponseWriter, req *http.Request) {
				h.followList(w, req, "following")
			})
			r.Get("/{username}/reviews", h.userReviews)
			r.Get("/{username}/history", h.publicHistory)
			r.Get("/{username}/watchlist", h.publicWatchlist)
			r.Get("/{username}/readlist", h.publicReadlist)
		})

		r.Route("/comments", func(r chi.Router) {
			r.Use(requireAuth)
			r.Get("/{id}/replies", h.commentReplies)
			r.With(requireAuth).Post("/{id}/reply", h.postReply)
			r.With(requireAuth).Put("/{id}", h.editComment)
			r.With(requireAuth).Delete("/{id}", h.deleteComment)
			r.With(requireAuth).Post("/{id}/vote", h.voteComment)
			r.With(requireAuth).Post("/{id}/report", h.reportComment)
		})
	})

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusNotFound, map[string]string{
			"error":   "Not Found",
			"message": "The requested resource was not found",
		})
	})

	return r
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	dbOK := h.pool.Ping(r.Context()) == nil
	status, database := "ok", "connected"
	if !dbOK {
		status, database = "degraded", "disconnected"
	}
	httpx.JSON(w, http.StatusOK, map[string]string{
		"status":    status,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"service":   "anime-kage-api",
		"database":  database,
	})
}

// GET /api/config — the handful of server switches the client must know
// about before it can render. Public and deliberately tiny; nothing secret
// belongs here.
func (h *Handler) publicConfig(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"inviteOnly": h.cfg.InviteOnly},
	})
}

func (h *Handler) apiIndex(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{
		"message": "Anime-Kage API",
		"version": "2.0.0",
		"endpoints": map[string]string{
			"health": "/health",
			"auth":   "/api/auth/*",
			"anime":  "/api/anime/*",
			"manga":  "/api/manga/*",
			"users":  "/api/users/*",
		},
	})
}
