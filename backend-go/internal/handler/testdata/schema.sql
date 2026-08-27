--
-- PostgreSQL database dump
--


-- Dumped from database version 15.18 (Debian 15.18-1.pgdg13+1)
-- Dumped by pg_dump version 15.18 (Debian 15.18-1.pgdg13+1)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: drizzle; Type: SCHEMA; Schema: -; Owner: -
--

CREATE SCHEMA drizzle;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: __drizzle_migrations; Type: TABLE; Schema: drizzle; Owner: -
--

CREATE TABLE drizzle.__drizzle_migrations (
    id integer NOT NULL,
    hash text NOT NULL,
    created_at bigint
);


--
-- Name: __drizzle_migrations_id_seq; Type: SEQUENCE; Schema: drizzle; Owner: -
--

CREATE SEQUENCE drizzle.__drizzle_migrations_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: __drizzle_migrations_id_seq; Type: SEQUENCE OWNED BY; Schema: drizzle; Owner: -
--

ALTER SEQUENCE drizzle.__drizzle_migrations_id_seq OWNED BY drizzle.__drizzle_migrations.id;


--
-- Name: anime; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.anime (
    id integer NOT NULL,
    mal_id integer,
    title character varying(500) NOT NULL,
    title_english character varying(500),
    title_romanian character varying(500),
    synopsis text,
    genres text[],
    studios text[],
    status character varying(50) NOT NULL,
    type character varying(50) NOT NULL,
    episodes integer,
    score numeric(4,2),
    year integer,
    season character varying(20),
    image_url character varying(500),
    trailer_url character varying(500),
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    broadcast_day character varying(20),
    broadcast_time character varying(10),
    translation_glossary text,
    synopsis_romanian text,
    banner_url text,
    slug text
);


--
-- Name: anime_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.anime_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: anime_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.anime_id_seq OWNED BY public.anime.id;


--
-- Name: anime_relations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.anime_relations (
    anime_id integer NOT NULL,
    relation text NOT NULL,
    related_mal_id integer NOT NULL,
    synced_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: announcements; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.announcements (
    id integer NOT NULL,
    tag text NOT NULL,
    title text NOT NULL,
    body text,
    url text,
    is_published boolean DEFAULT true NOT NULL,
    author_id integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: announcements_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.announcements_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: announcements_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.announcements_id_seq OWNED BY public.announcements.id;


--
-- Name: chapter_pages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.chapter_pages (
    id integer NOT NULL,
    chapter_id integer NOT NULL,
    language text DEFAULT 'ro'::text NOT NULL,
    idx integer NOT NULL,
    url text NOT NULL,
    CONSTRAINT chapter_pages_idx_positive CHECK ((idx >= 0))
);


--
-- Name: chapter_pages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.chapter_pages_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: chapter_pages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.chapter_pages_id_seq OWNED BY public.chapter_pages.id;


--
-- Name: chapters; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.chapters (
    id integer NOT NULL,
    manga_id integer NOT NULL,
    chapter_number numeric(5,2) NOT NULL,
    title character varying(500),
    release_date date,
    pages integer,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: chapters_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.chapters_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: chapters_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.chapters_id_seq OWNED BY public.chapters.id;


--
-- Name: chat_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.chat_messages (
    id bigint NOT NULL,
    user_id integer NOT NULL,
    body text NOT NULL,
    reply_to_user text,
    reply_to_excerpt text,
    deleted_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chat_messages_body_check CHECK (((length(body) >= 1) AND (length(body) <= 500)))
);


--
-- Name: chat_messages_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.chat_messages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: chat_messages_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.chat_messages_id_seq OWNED BY public.chat_messages.id;


--
-- Name: chat_restrictions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.chat_restrictions (
    user_id integer NOT NULL,
    expires_at timestamp with time zone,
    reason text,
    created_by integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: comment_votes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comment_votes (
    id integer NOT NULL,
    user_id integer NOT NULL,
    comment_id integer NOT NULL,
    vote_type character varying(10) NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: comment_votes_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.comment_votes_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: comment_votes_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.comment_votes_id_seq OWNED BY public.comment_votes.id;


--
-- Name: comments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.comments (
    id integer NOT NULL,
    user_id integer NOT NULL,
    anime_id integer,
    manga_id integer,
    content text NOT NULL,
    likes_count integer DEFAULT 0 NOT NULL,
    dislikes_count integer DEFAULT 0 NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    is_deleted boolean DEFAULT false NOT NULL,
    is_reported boolean DEFAULT false NOT NULL,
    parent_id integer,
    replies_count integer DEFAULT 0 NOT NULL,
    episode_id integer,
    chapter_id integer,
    watchlist_id integer,
    readlist_id integer,
    root_id integer
);


--
-- Name: comments_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.comments_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: comments_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.comments_id_seq OWNED BY public.comments.id;


--
-- Name: content_links; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.content_links (
    id integer NOT NULL,
    episode_id integer,
    chapter_id integer,
    hosting_url character varying(1000) NOT NULL,
    quality character varying(20),
    language character varying(10) DEFAULT 'ro'::character varying NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    kind text DEFAULT 'embed'::text NOT NULL,
    provider text,
    provider_ref text,
    priority integer DEFAULT 0 NOT NULL,
    last_checked_at timestamp with time zone,
    last_ok boolean,
    CONSTRAINT content_links_extract_ref_check CHECK (((kind <> 'extract'::text) OR ((provider IS NOT NULL) AND (provider_ref IS NOT NULL)))),
    CONSTRAINT content_links_kind_check CHECK ((kind = ANY (ARRAY['embed'::text, 'extract'::text])))
);


--
-- Name: content_links_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.content_links_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: content_links_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.content_links_id_seq OWNED BY public.content_links.id;


--
-- Name: curated_picks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.curated_picks (
    id integer NOT NULL,
    slot text NOT NULL,
    "position" integer NOT NULL,
    anime_id integer,
    manga_id integer,
    created_by integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    image_url text,
    CONSTRAINT curated_picks_one_target CHECK ((((anime_id IS NOT NULL) AND (manga_id IS NULL)) OR ((anime_id IS NULL) AND (manga_id IS NOT NULL))))
);


--
-- Name: curated_picks_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.curated_picks_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: curated_picks_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.curated_picks_id_seq OWNED BY public.curated_picks.id;


--
-- Name: episode_views; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.episode_views (
    user_id integer NOT NULL,
    episode_id integer NOT NULL,
    anime_id integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: episodes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.episodes (
    id integer NOT NULL,
    anime_id integer NOT NULL,
    episode_number integer NOT NULL,
    title character varying(500),
    air_date date,
    duration integer,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    synopsis text,
    is_filler boolean DEFAULT false NOT NULL,
    is_recap boolean DEFAULT false NOT NULL
);


--
-- Name: episodes_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.episodes_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: episodes_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.episodes_id_seq OWNED BY public.episodes.id;


--
-- Name: follows; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.follows (
    id integer NOT NULL,
    follower_id integer NOT NULL,
    following_id integer NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: follows_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.follows_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: follows_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.follows_id_seq OWNED BY public.follows.id;


--
-- Name: forum_replies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.forum_replies (
    id integer NOT NULL,
    thread_id integer NOT NULL,
    user_id integer NOT NULL,
    body text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT forum_replies_body_check CHECK (((char_length(body) >= 1) AND (char_length(body) <= 8000)))
);


--
-- Name: forum_replies_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.forum_replies_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: forum_replies_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.forum_replies_id_seq OWNED BY public.forum_replies.id;


--
-- Name: forum_threads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.forum_threads (
    id integer NOT NULL,
    user_id integer NOT NULL,
    category text NOT NULL,
    title text NOT NULL,
    body text NOT NULL,
    is_pinned boolean DEFAULT false NOT NULL,
    is_locked boolean DEFAULT false NOT NULL,
    reply_count integer DEFAULT 0 NOT NULL,
    last_activity_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT forum_threads_body_check CHECK (((char_length(body) >= 1) AND (char_length(body) <= 8000))),
    CONSTRAINT forum_threads_title_check CHECK (((char_length(title) >= 3) AND (char_length(title) <= 160)))
);


--
-- Name: forum_threads_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.forum_threads_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: forum_threads_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.forum_threads_id_seq OWNED BY public.forum_threads.id;


--
-- Name: invites; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.invites (
    id integer NOT NULL,
    code text NOT NULL,
    discord_user_id text NOT NULL,
    discord_username text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone,
    used_by_user_id integer,
    used_at timestamp with time zone,
    CONSTRAINT invites_used_together CHECK (((used_by_user_id IS NULL) OR (used_at IS NOT NULL)))
);


--
-- Name: invites_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.invites_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: invites_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.invites_id_seq OWNED BY public.invites.id;


--
-- Name: list_likes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.list_likes (
    id integer NOT NULL,
    list_id integer NOT NULL,
    user_id integer NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: list_likes_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.list_likes_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: list_likes_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.list_likes_id_seq OWNED BY public.list_likes.id;


--
-- Name: manga; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.manga (
    id integer NOT NULL,
    mal_id integer,
    title character varying(500) NOT NULL,
    title_english character varying(500),
    title_romanian character varying(500),
    synopsis text,
    genres text[],
    authors text[],
    status character varying(50) NOT NULL,
    type character varying(50) NOT NULL,
    chapters integer,
    volumes integer,
    score numeric(4,2),
    year integer,
    image_url character varying(500),
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    synopsis_romanian text,
    banner_url text,
    slug text
);


--
-- Name: manga_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.manga_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: manga_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.manga_id_seq OWNED BY public.manga.id;


--
-- Name: notifications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notifications (
    id integer NOT NULL,
    user_id integer NOT NULL,
    type text NOT NULL,
    actor_id integer,
    body text NOT NULL,
    link text,
    read_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: notifications_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.notifications_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: notifications_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.notifications_id_seq OWNED BY public.notifications.id;


--
-- Name: password_resets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.password_resets (
    id integer NOT NULL,
    user_id integer NOT NULL,
    token_hash text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    requested_ip text
);


--
-- Name: password_resets_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.password_resets_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: password_resets_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.password_resets_id_seq OWNED BY public.password_resets.id;


--
-- Name: playback_positions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.playback_positions (
    user_id integer NOT NULL,
    episode_id integer NOT NULL,
    position_s double precision NOT NULL,
    duration_s double precision,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT playback_positions_position_check CHECK ((position_s >= (0)::double precision))
);


--
-- Name: readlist; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.readlist (
    id integer NOT NULL,
    user_id integer NOT NULL,
    manga_id integer NOT NULL,
    status character varying(50) NOT NULL,
    score integer,
    chapters_read integer DEFAULT 0 NOT NULL,
    volumes_read integer DEFAULT 0 NOT NULL,
    notes text,
    started_at timestamp without time zone,
    completed_at timestamp without time zone,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: readlist_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.readlist_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: readlist_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.readlist_id_seq OWNED BY public.readlist.id;


--
-- Name: releases; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.releases (
    id integer NOT NULL,
    anime_id integer,
    episode_number integer,
    uploader_id integer NOT NULL,
    reviewer_id integer,
    state text DEFAULT 'draft'::text NOT NULL,
    staging_path text,
    review_notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    proposed_title text,
    assigned_verifier_id integer,
    medium text DEFAULT 'anime'::text NOT NULL,
    manga_id integer,
    chapter_number numeric(5,2),
    published_by integer,
    published_at timestamp with time zone,
    hardsub_state text,
    hardsub_path text,
    hardsub_error text,
    hardsub_queued_at timestamp with time zone,
    hardsub_finished_at timestamp with time zone,
    r2_key text,
    remux_state text,
    remux_error text,
    remux_queued_at timestamp with time zone,
    remux_finished_at timestamp with time zone,
    CONSTRAINT releases_chapter_positive CHECK (((chapter_number IS NULL) OR (chapter_number > (0)::numeric))),
    CONSTRAINT releases_episode_positive CHECK (((episode_number IS NULL) OR (episode_number > 0))),
    CONSTRAINT releases_hardsub_state_check CHECK (((hardsub_state IS NULL) OR (hardsub_state = ANY (ARRAY['queued'::text, 'running'::text, 'done'::text, 'failed'::text])))),
    CONSTRAINT releases_medium_check CHECK ((medium = ANY (ARRAY['anime'::text, 'manga'::text]))),
    CONSTRAINT releases_medium_target CHECK ((((medium = 'anime'::text) AND (manga_id IS NULL) AND (chapter_number IS NULL) AND (episode_number IS NOT NULL)) OR ((medium = 'manga'::text) AND (anime_id IS NULL) AND (episode_number IS NULL) AND (chapter_number IS NOT NULL)))),
    CONSTRAINT releases_series_named CHECK (((anime_id IS NOT NULL) OR (manga_id IS NOT NULL) OR (proposed_title IS NOT NULL))),
    CONSTRAINT releases_state_check CHECK ((state = ANY (ARRAY['draft'::text, 'in_review'::text, 'changes_requested'::text, 'approved'::text, 'published'::text])))
);


--
-- Name: releases_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.releases_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: releases_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.releases_id_seq OWNED BY public.releases.id;


--
-- Name: request_votes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.request_votes (
    id integer NOT NULL,
    request_id integer NOT NULL,
    user_id integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: request_votes_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.request_votes_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: request_votes_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.request_votes_id_seq OWNED BY public.request_votes.id;


--
-- Name: schedule_slots; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.schedule_slots (
    id integer NOT NULL,
    anime_id integer NOT NULL,
    episode_number integer NOT NULL,
    scheduled_at timestamp with time zone NOT NULL,
    note text,
    created_by integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT schedule_slots_episode_number_check CHECK ((episode_number > 0))
);


--
-- Name: schedule_slots_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.schedule_slots_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: schedule_slots_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.schedule_slots_id_seq OWNED BY public.schedule_slots.id;


--
-- Name: schema_migrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.schema_migrations (
    version text NOT NULL,
    applied_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: skip_marks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.skip_marks (
    id integer NOT NULL,
    episode_id integer NOT NULL,
    kind text NOT NULL,
    start_s double precision NOT NULL,
    end_s double precision NOT NULL,
    source text DEFAULT 'manual'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT skip_marks_kind_check CHECK ((kind = ANY (ARRAY['intro'::text, 'outro'::text]))),
    CONSTRAINT skip_marks_range_check CHECK (((start_s >= (0)::double precision) AND (end_s > start_s))),
    CONSTRAINT skip_marks_source_check CHECK ((source = ANY (ARRAY['manual'::text, 'aniskip'::text])))
);


--
-- Name: skip_marks_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.skip_marks_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: skip_marks_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.skip_marks_id_seq OWNED BY public.skip_marks.id;


--
-- Name: subtitle_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.subtitle_events (
    id integer NOT NULL,
    release_id integer NOT NULL,
    idx integer NOT NULL,
    start_ms integer NOT NULL,
    end_ms integer NOT NULL,
    en_text text DEFAULT ''::text NOT NULL,
    ro_text text DEFAULT ''::text NOT NULL,
    edited boolean DEFAULT false NOT NULL,
    CONSTRAINT subtitle_events_range_check CHECK (((start_ms >= 0) AND (end_ms >= start_ms)))
);


--
-- Name: subtitle_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.subtitle_events_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: subtitle_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.subtitle_events_id_seq OWNED BY public.subtitle_events.id;


--
-- Name: subtitles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.subtitles (
    id integer NOT NULL,
    episode_id integer NOT NULL,
    language text NOT NULL,
    label text,
    format text DEFAULT 'vtt'::text NOT NULL,
    url text NOT NULL,
    status text DEFAULT 'published'::text NOT NULL,
    translator_id integer,
    source_sub text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT subtitles_format_check CHECK ((format = ANY (ARRAY['vtt'::text, 'srt'::text, 'ass'::text]))),
    CONSTRAINT subtitles_status_check CHECK ((status = ANY (ARRAY['machine'::text, 'edited'::text, 'published'::text])))
);


--
-- Name: subtitles_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.subtitles_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: subtitles_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.subtitles_id_seq OWNED BY public.subtitles.id;


--
-- Name: translation_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.translation_requests (
    id integer NOT NULL,
    user_id integer NOT NULL,
    medium text NOT NULL,
    mal_id integer,
    title text NOT NULL,
    image_url text,
    note text,
    status text DEFAULT 'pending'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT translation_requests_medium_check CHECK ((medium = ANY (ARRAY['anime'::text, 'manga'::text]))),
    CONSTRAINT translation_requests_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'in_progress'::text, 'approved'::text, 'rejected'::text])))
);


--
-- Name: translation_requests_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.translation_requests_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: translation_requests_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.translation_requests_id_seq OWNED BY public.translation_requests.id;


--
-- Name: user_list_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_list_items (
    id integer NOT NULL,
    list_id integer NOT NULL,
    anime_id integer,
    manga_id integer,
    note text,
    "position" integer DEFAULT 0 NOT NULL,
    added_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT user_list_items_check CHECK (((anime_id IS NOT NULL) <> (manga_id IS NOT NULL)))
);


--
-- Name: user_list_items_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_list_items_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_list_items_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_list_items_id_seq OWNED BY public.user_list_items.id;


--
-- Name: user_lists; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_lists (
    id integer NOT NULL,
    user_id integer NOT NULL,
    title text NOT NULL,
    description text,
    is_public boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT user_lists_title_check CHECK (((char_length(title) >= 1) AND (char_length(title) <= 120)))
);


--
-- Name: user_lists_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_lists_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_lists_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_lists_id_seq OWNED BY public.user_lists.id;


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id integer NOT NULL,
    username character varying(50) NOT NULL,
    email character varying(255) NOT NULL,
    password_hash character varying(255) NOT NULL,
    avatar_url character varying(500),
    bio text,
    favorite_genres text[],
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    role character varying(20) DEFAULT 'user'::character varying NOT NULL,
    favorites jsonb,
    is_banned boolean DEFAULT false NOT NULL,
    last_verifier_id integer,
    release_cap integer,
    banner_anime_id integer,
    banner_manga_id integer,
    CONSTRAINT users_banner_one_target CHECK (((banner_anime_id IS NULL) OR (banner_manga_id IS NULL))),
    CONSTRAINT users_release_cap_nonneg CHECK (((release_cap IS NULL) OR (release_cap >= 0)))
);


--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.users_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;


--
-- Name: watch_history; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.watch_history (
    id integer NOT NULL,
    user_id integer NOT NULL,
    anime_id integer,
    manga_id integer,
    amount integer DEFAULT 1 NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: watch_history_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.watch_history_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: watch_history_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.watch_history_id_seq OWNED BY public.watch_history.id;


--
-- Name: watchlist; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.watchlist (
    id integer NOT NULL,
    user_id integer NOT NULL,
    anime_id integer NOT NULL,
    status character varying(50) NOT NULL,
    score integer,
    episodes_watched integer DEFAULT 0 NOT NULL,
    notes text,
    started_at timestamp without time zone,
    completed_at timestamp without time zone,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: watchlist_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.watchlist_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: watchlist_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.watchlist_id_seq OWNED BY public.watchlist.id;


--
-- Name: __drizzle_migrations id; Type: DEFAULT; Schema: drizzle; Owner: -
--

ALTER TABLE ONLY drizzle.__drizzle_migrations ALTER COLUMN id SET DEFAULT nextval('drizzle.__drizzle_migrations_id_seq'::regclass);


--
-- Name: anime id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anime ALTER COLUMN id SET DEFAULT nextval('public.anime_id_seq'::regclass);


--
-- Name: announcements id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.announcements ALTER COLUMN id SET DEFAULT nextval('public.announcements_id_seq'::regclass);


--
-- Name: chapter_pages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chapter_pages ALTER COLUMN id SET DEFAULT nextval('public.chapter_pages_id_seq'::regclass);


--
-- Name: chapters id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chapters ALTER COLUMN id SET DEFAULT nextval('public.chapters_id_seq'::regclass);


--
-- Name: chat_messages id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_messages ALTER COLUMN id SET DEFAULT nextval('public.chat_messages_id_seq'::regclass);


--
-- Name: comment_votes id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comment_votes ALTER COLUMN id SET DEFAULT nextval('public.comment_votes_id_seq'::regclass);


--
-- Name: comments id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments ALTER COLUMN id SET DEFAULT nextval('public.comments_id_seq'::regclass);


--
-- Name: content_links id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_links ALTER COLUMN id SET DEFAULT nextval('public.content_links_id_seq'::regclass);


--
-- Name: curated_picks id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.curated_picks ALTER COLUMN id SET DEFAULT nextval('public.curated_picks_id_seq'::regclass);


--
-- Name: episodes id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.episodes ALTER COLUMN id SET DEFAULT nextval('public.episodes_id_seq'::regclass);


--
-- Name: follows id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.follows ALTER COLUMN id SET DEFAULT nextval('public.follows_id_seq'::regclass);


--
-- Name: forum_replies id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_replies ALTER COLUMN id SET DEFAULT nextval('public.forum_replies_id_seq'::regclass);


--
-- Name: forum_threads id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_threads ALTER COLUMN id SET DEFAULT nextval('public.forum_threads_id_seq'::regclass);


--
-- Name: invites id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invites ALTER COLUMN id SET DEFAULT nextval('public.invites_id_seq'::regclass);


--
-- Name: list_likes id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.list_likes ALTER COLUMN id SET DEFAULT nextval('public.list_likes_id_seq'::regclass);


--
-- Name: manga id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.manga ALTER COLUMN id SET DEFAULT nextval('public.manga_id_seq'::regclass);


--
-- Name: notifications id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notifications ALTER COLUMN id SET DEFAULT nextval('public.notifications_id_seq'::regclass);


--
-- Name: password_resets id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.password_resets ALTER COLUMN id SET DEFAULT nextval('public.password_resets_id_seq'::regclass);


--
-- Name: readlist id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.readlist ALTER COLUMN id SET DEFAULT nextval('public.readlist_id_seq'::regclass);


--
-- Name: releases id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.releases ALTER COLUMN id SET DEFAULT nextval('public.releases_id_seq'::regclass);


--
-- Name: request_votes id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.request_votes ALTER COLUMN id SET DEFAULT nextval('public.request_votes_id_seq'::regclass);


--
-- Name: schedule_slots id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schedule_slots ALTER COLUMN id SET DEFAULT nextval('public.schedule_slots_id_seq'::regclass);


--
-- Name: skip_marks id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.skip_marks ALTER COLUMN id SET DEFAULT nextval('public.skip_marks_id_seq'::regclass);


--
-- Name: subtitle_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subtitle_events ALTER COLUMN id SET DEFAULT nextval('public.subtitle_events_id_seq'::regclass);


--
-- Name: subtitles id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subtitles ALTER COLUMN id SET DEFAULT nextval('public.subtitles_id_seq'::regclass);


--
-- Name: translation_requests id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.translation_requests ALTER COLUMN id SET DEFAULT nextval('public.translation_requests_id_seq'::regclass);


--
-- Name: user_list_items id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_list_items ALTER COLUMN id SET DEFAULT nextval('public.user_list_items_id_seq'::regclass);


--
-- Name: user_lists id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_lists ALTER COLUMN id SET DEFAULT nextval('public.user_lists_id_seq'::regclass);


--
-- Name: users id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users ALTER COLUMN id SET DEFAULT nextval('public.users_id_seq'::regclass);


--
-- Name: watch_history id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.watch_history ALTER COLUMN id SET DEFAULT nextval('public.watch_history_id_seq'::regclass);


--
-- Name: watchlist id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.watchlist ALTER COLUMN id SET DEFAULT nextval('public.watchlist_id_seq'::regclass);


--
-- Name: __drizzle_migrations __drizzle_migrations_pkey; Type: CONSTRAINT; Schema: drizzle; Owner: -
--

ALTER TABLE ONLY drizzle.__drizzle_migrations
    ADD CONSTRAINT __drizzle_migrations_pkey PRIMARY KEY (id);


--
-- Name: anime anime_mal_id_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anime
    ADD CONSTRAINT anime_mal_id_unique UNIQUE (mal_id);


--
-- Name: anime anime_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anime
    ADD CONSTRAINT anime_pkey PRIMARY KEY (id);


--
-- Name: anime_relations anime_relations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anime_relations
    ADD CONSTRAINT anime_relations_pkey PRIMARY KEY (anime_id, related_mal_id);


--
-- Name: announcements announcements_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.announcements
    ADD CONSTRAINT announcements_pkey PRIMARY KEY (id);


--
-- Name: chapter_pages chapter_pages_idx_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chapter_pages
    ADD CONSTRAINT chapter_pages_idx_unique UNIQUE (chapter_id, language, idx);


--
-- Name: chapter_pages chapter_pages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chapter_pages
    ADD CONSTRAINT chapter_pages_pkey PRIMARY KEY (id);


--
-- Name: chapters chapters_manga_id_chapter_number_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chapters
    ADD CONSTRAINT chapters_manga_id_chapter_number_unique UNIQUE (manga_id, chapter_number);


--
-- Name: chapters chapters_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chapters
    ADD CONSTRAINT chapters_pkey PRIMARY KEY (id);


--
-- Name: chat_messages chat_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_messages
    ADD CONSTRAINT chat_messages_pkey PRIMARY KEY (id);


--
-- Name: chat_restrictions chat_restrictions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_restrictions
    ADD CONSTRAINT chat_restrictions_pkey PRIMARY KEY (user_id);


--
-- Name: comment_votes comment_votes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comment_votes
    ADD CONSTRAINT comment_votes_pkey PRIMARY KEY (id);


--
-- Name: comment_votes comment_votes_user_id_comment_id_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comment_votes
    ADD CONSTRAINT comment_votes_user_id_comment_id_unique UNIQUE (user_id, comment_id);


--
-- Name: comments comments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments
    ADD CONSTRAINT comments_pkey PRIMARY KEY (id);


--
-- Name: content_links content_links_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_links
    ADD CONSTRAINT content_links_pkey PRIMARY KEY (id);


--
-- Name: curated_picks curated_picks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.curated_picks
    ADD CONSTRAINT curated_picks_pkey PRIMARY KEY (id);


--
-- Name: curated_picks curated_picks_slot_position; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.curated_picks
    ADD CONSTRAINT curated_picks_slot_position UNIQUE (slot, "position");


--
-- Name: episode_views episode_views_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.episode_views
    ADD CONSTRAINT episode_views_pkey PRIMARY KEY (user_id, episode_id);


--
-- Name: episodes episodes_anime_id_episode_number_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.episodes
    ADD CONSTRAINT episodes_anime_id_episode_number_unique UNIQUE (anime_id, episode_number);


--
-- Name: episodes episodes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.episodes
    ADD CONSTRAINT episodes_pkey PRIMARY KEY (id);


--
-- Name: follows follows_follower_id_following_id_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.follows
    ADD CONSTRAINT follows_follower_id_following_id_unique UNIQUE (follower_id, following_id);


--
-- Name: follows follows_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.follows
    ADD CONSTRAINT follows_pkey PRIMARY KEY (id);


--
-- Name: forum_replies forum_replies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_replies
    ADD CONSTRAINT forum_replies_pkey PRIMARY KEY (id);


--
-- Name: forum_threads forum_threads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_threads
    ADD CONSTRAINT forum_threads_pkey PRIMARY KEY (id);


--
-- Name: invites invites_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invites
    ADD CONSTRAINT invites_code_key UNIQUE (code);


--
-- Name: invites invites_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invites
    ADD CONSTRAINT invites_pkey PRIMARY KEY (id);


--
-- Name: list_likes list_likes_list_id_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.list_likes
    ADD CONSTRAINT list_likes_list_id_user_id_key UNIQUE (list_id, user_id);


--
-- Name: list_likes list_likes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.list_likes
    ADD CONSTRAINT list_likes_pkey PRIMARY KEY (id);


--
-- Name: manga manga_mal_id_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.manga
    ADD CONSTRAINT manga_mal_id_unique UNIQUE (mal_id);


--
-- Name: manga manga_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.manga
    ADD CONSTRAINT manga_pkey PRIMARY KEY (id);


--
-- Name: notifications notifications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_pkey PRIMARY KEY (id);


--
-- Name: password_resets password_resets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.password_resets
    ADD CONSTRAINT password_resets_pkey PRIMARY KEY (id);


--
-- Name: password_resets password_resets_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.password_resets
    ADD CONSTRAINT password_resets_token_hash_key UNIQUE (token_hash);


--
-- Name: playback_positions playback_positions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.playback_positions
    ADD CONSTRAINT playback_positions_pkey PRIMARY KEY (user_id, episode_id);


--
-- Name: readlist readlist_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.readlist
    ADD CONSTRAINT readlist_pkey PRIMARY KEY (id);


--
-- Name: readlist readlist_user_id_manga_id_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.readlist
    ADD CONSTRAINT readlist_user_id_manga_id_unique UNIQUE (user_id, manga_id);


--
-- Name: releases releases_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.releases
    ADD CONSTRAINT releases_pkey PRIMARY KEY (id);


--
-- Name: request_votes request_votes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.request_votes
    ADD CONSTRAINT request_votes_pkey PRIMARY KEY (id);


--
-- Name: request_votes request_votes_request_id_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.request_votes
    ADD CONSTRAINT request_votes_request_id_user_id_key UNIQUE (request_id, user_id);


--
-- Name: schedule_slots schedule_slots_anime_id_episode_number_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schedule_slots
    ADD CONSTRAINT schedule_slots_anime_id_episode_number_key UNIQUE (anime_id, episode_number);


--
-- Name: schedule_slots schedule_slots_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schedule_slots
    ADD CONSTRAINT schedule_slots_pkey PRIMARY KEY (id);


--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


--
-- Name: skip_marks skip_marks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.skip_marks
    ADD CONSTRAINT skip_marks_pkey PRIMARY KEY (id);


--
-- Name: skip_marks skip_marks_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.skip_marks
    ADD CONSTRAINT skip_marks_unique UNIQUE (episode_id, kind);


--
-- Name: subtitle_events subtitle_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subtitle_events
    ADD CONSTRAINT subtitle_events_pkey PRIMARY KEY (id);


--
-- Name: subtitle_events subtitle_events_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subtitle_events
    ADD CONSTRAINT subtitle_events_unique UNIQUE (release_id, idx);


--
-- Name: subtitles subtitles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subtitles
    ADD CONSTRAINT subtitles_pkey PRIMARY KEY (id);


--
-- Name: subtitles subtitles_published_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subtitles
    ADD CONSTRAINT subtitles_published_unique UNIQUE (episode_id, language, status);


--
-- Name: translation_requests translation_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.translation_requests
    ADD CONSTRAINT translation_requests_pkey PRIMARY KEY (id);


--
-- Name: user_list_items user_list_items_list_id_anime_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_list_items
    ADD CONSTRAINT user_list_items_list_id_anime_id_key UNIQUE (list_id, anime_id);


--
-- Name: user_list_items user_list_items_list_id_manga_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_list_items
    ADD CONSTRAINT user_list_items_list_id_manga_id_key UNIQUE (list_id, manga_id);


--
-- Name: user_list_items user_list_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_list_items
    ADD CONSTRAINT user_list_items_pkey PRIMARY KEY (id);


--
-- Name: user_lists user_lists_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_lists
    ADD CONSTRAINT user_lists_pkey PRIMARY KEY (id);


--
-- Name: users users_email_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_email_unique UNIQUE (email);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: users users_username_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_username_unique UNIQUE (username);


--
-- Name: watch_history watch_history_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.watch_history
    ADD CONSTRAINT watch_history_pkey PRIMARY KEY (id);


--
-- Name: watchlist watchlist_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.watchlist
    ADD CONSTRAINT watchlist_pkey PRIMARY KEY (id);


--
-- Name: watchlist watchlist_user_id_anime_id_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.watchlist
    ADD CONSTRAINT watchlist_user_id_anime_id_unique UNIQUE (user_id, anime_id);


--
-- Name: anime_missing_banner_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX anime_missing_banner_idx ON public.anime USING btree (mal_id) WHERE ((banner_url IS NULL) AND (mal_id IS NOT NULL));


--
-- Name: anime_relations_anime_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX anime_relations_anime_idx ON public.anime_relations USING btree (anime_id);


--
-- Name: anime_relations_related_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX anime_relations_related_idx ON public.anime_relations USING btree (related_mal_id);


--
-- Name: anime_score_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX anime_score_idx ON public.anime USING btree (score);


--
-- Name: anime_slug_uniq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX anime_slug_uniq ON public.anime USING btree (slug) WHERE (slug IS NOT NULL);


--
-- Name: anime_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX anime_status_idx ON public.anime USING btree (status);


--
-- Name: anime_year_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX anime_year_idx ON public.anime USING btree (year);


--
-- Name: announcements_feed_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX announcements_feed_idx ON public.announcements USING btree (created_at DESC) WHERE is_published;


--
-- Name: chapter_pages_lookup_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX chapter_pages_lookup_idx ON public.chapter_pages USING btree (chapter_id, language, idx);


--
-- Name: chat_messages_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX chat_messages_created_idx ON public.chat_messages USING btree (created_at DESC);


--
-- Name: chat_restrictions_expires_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX chat_restrictions_expires_idx ON public.chat_restrictions USING btree (expires_at) WHERE (expires_at IS NOT NULL);


--
-- Name: comments_anime_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX comments_anime_id_idx ON public.comments USING btree (anime_id);


--
-- Name: comments_created_at_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX comments_created_at_idx ON public.comments USING btree (created_at);


--
-- Name: comments_manga_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX comments_manga_id_idx ON public.comments USING btree (manga_id);


--
-- Name: comments_parent_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX comments_parent_id_idx ON public.comments USING btree (parent_id);


--
-- Name: comments_reported_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX comments_reported_idx ON public.comments USING btree (created_at DESC) WHERE ((is_reported = true) AND (is_deleted = false));


--
-- Name: comments_root_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX comments_root_id_idx ON public.comments USING btree (root_id);


--
-- Name: content_links_health_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX content_links_health_idx ON public.content_links USING btree (is_active, last_checked_at);


--
-- Name: curated_picks_slot_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX curated_picks_slot_idx ON public.curated_picks USING btree (slot, "position");


--
-- Name: episode_views_anime_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX episode_views_anime_created_idx ON public.episode_views USING btree (anime_id, created_at DESC);


--
-- Name: follows_follower_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX follows_follower_id_idx ON public.follows USING btree (follower_id);


--
-- Name: follows_following_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX follows_following_id_idx ON public.follows USING btree (following_id);


--
-- Name: forum_replies_thread_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX forum_replies_thread_idx ON public.forum_replies USING btree (thread_id, created_at);


--
-- Name: forum_threads_activity_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX forum_threads_activity_idx ON public.forum_threads USING btree (is_pinned DESC, last_activity_at DESC);


--
-- Name: forum_threads_cat_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX forum_threads_cat_idx ON public.forum_threads USING btree (category, last_activity_at DESC);


--
-- Name: invites_issuer_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX invites_issuer_idx ON public.invites USING btree (discord_user_id, created_at DESC);


--
-- Name: invites_outstanding_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX invites_outstanding_idx ON public.invites USING btree (discord_user_id) WHERE (used_at IS NULL);


--
-- Name: list_likes_list_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX list_likes_list_idx ON public.list_likes USING btree (list_id);


--
-- Name: manga_missing_banner_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX manga_missing_banner_idx ON public.manga USING btree (mal_id) WHERE ((banner_url IS NULL) AND (mal_id IS NOT NULL));


--
-- Name: manga_score_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX manga_score_idx ON public.manga USING btree (score);


--
-- Name: manga_slug_uniq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX manga_slug_uniq ON public.manga USING btree (slug) WHERE (slug IS NOT NULL);


--
-- Name: manga_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX manga_status_idx ON public.manga USING btree (status);


--
-- Name: notifications_unread_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX notifications_unread_idx ON public.notifications USING btree (user_id) WHERE (read_at IS NULL);


--
-- Name: notifications_user_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX notifications_user_idx ON public.notifications USING btree (user_id, created_at DESC);


--
-- Name: password_resets_live_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX password_resets_live_idx ON public.password_resets USING btree (user_id) WHERE (used_at IS NULL);


--
-- Name: password_resets_token_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX password_resets_token_idx ON public.password_resets USING btree (token_hash);


--
-- Name: readlist_user_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX readlist_user_id_idx ON public.readlist USING btree (user_id);


--
-- Name: releases_assigned_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX releases_assigned_idx ON public.releases USING btree (assigned_verifier_id, state);


--
-- Name: releases_hardsub_queue_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX releases_hardsub_queue_idx ON public.releases USING btree (hardsub_queued_at) WHERE (hardsub_state = ANY (ARRAY['queued'::text, 'running'::text]));


--
-- Name: releases_r2_key_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX releases_r2_key_idx ON public.releases USING btree (id) WHERE (r2_key IS NOT NULL);


--
-- Name: releases_remux_queue_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX releases_remux_queue_idx ON public.releases USING btree (remux_queued_at) WHERE (remux_state = 'queued'::text);


--
-- Name: releases_state_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX releases_state_idx ON public.releases USING btree (state, updated_at);


--
-- Name: releases_uploader_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX releases_uploader_idx ON public.releases USING btree (uploader_id);


--
-- Name: request_votes_request_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX request_votes_request_idx ON public.request_votes USING btree (request_id);


--
-- Name: schedule_slots_when_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX schedule_slots_when_idx ON public.schedule_slots USING btree (scheduled_at);


--
-- Name: subtitles_episode_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX subtitles_episode_idx ON public.subtitles USING btree (episode_id, status);


--
-- Name: translation_requests_medium_mal_uniq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX translation_requests_medium_mal_uniq ON public.translation_requests USING btree (medium, mal_id) WHERE (mal_id IS NOT NULL);


--
-- Name: translation_requests_medium_title_uniq; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX translation_requests_medium_title_uniq ON public.translation_requests USING btree (medium, lower(title)) WHERE (mal_id IS NULL);


--
-- Name: user_list_items_list_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX user_list_items_list_idx ON public.user_list_items USING btree (list_id, "position", id);


--
-- Name: user_lists_public_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX user_lists_public_idx ON public.user_lists USING btree (is_public, updated_at DESC);


--
-- Name: user_lists_user_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX user_lists_user_idx ON public.user_lists USING btree (user_id, updated_at DESC);


--
-- Name: watch_history_user_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX watch_history_user_created_idx ON public.watch_history USING btree (user_id, created_at);


--
-- Name: watchlist_user_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX watchlist_user_id_idx ON public.watchlist USING btree (user_id);


--
-- Name: watchlist_user_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX watchlist_user_status_idx ON public.watchlist USING btree (user_id, status);


--
-- Name: anime_relations anime_relations_anime_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anime_relations
    ADD CONSTRAINT anime_relations_anime_id_fkey FOREIGN KEY (anime_id) REFERENCES public.anime(id) ON DELETE CASCADE;


--
-- Name: announcements announcements_author_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.announcements
    ADD CONSTRAINT announcements_author_id_fkey FOREIGN KEY (author_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: chapter_pages chapter_pages_chapter_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chapter_pages
    ADD CONSTRAINT chapter_pages_chapter_id_fkey FOREIGN KEY (chapter_id) REFERENCES public.chapters(id) ON DELETE CASCADE;


--
-- Name: chapters chapters_manga_id_manga_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chapters
    ADD CONSTRAINT chapters_manga_id_manga_id_fk FOREIGN KEY (manga_id) REFERENCES public.manga(id) ON DELETE CASCADE;


--
-- Name: chat_messages chat_messages_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_messages
    ADD CONSTRAINT chat_messages_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: chat_restrictions chat_restrictions_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_restrictions
    ADD CONSTRAINT chat_restrictions_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: chat_restrictions chat_restrictions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chat_restrictions
    ADD CONSTRAINT chat_restrictions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: comment_votes comment_votes_comment_id_comments_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comment_votes
    ADD CONSTRAINT comment_votes_comment_id_comments_id_fk FOREIGN KEY (comment_id) REFERENCES public.comments(id);


--
-- Name: comment_votes comment_votes_user_id_users_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comment_votes
    ADD CONSTRAINT comment_votes_user_id_users_id_fk FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: comments comments_anime_id_anime_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments
    ADD CONSTRAINT comments_anime_id_anime_id_fk FOREIGN KEY (anime_id) REFERENCES public.anime(id) ON DELETE CASCADE;


--
-- Name: comments comments_chapter_id_chapters_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments
    ADD CONSTRAINT comments_chapter_id_chapters_id_fk FOREIGN KEY (chapter_id) REFERENCES public.chapters(id) ON DELETE CASCADE;


--
-- Name: comments comments_episode_id_episodes_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments
    ADD CONSTRAINT comments_episode_id_episodes_id_fk FOREIGN KEY (episode_id) REFERENCES public.episodes(id) ON DELETE CASCADE;


--
-- Name: comments comments_manga_id_manga_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments
    ADD CONSTRAINT comments_manga_id_manga_id_fk FOREIGN KEY (manga_id) REFERENCES public.manga(id) ON DELETE CASCADE;


--
-- Name: comments comments_parent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments
    ADD CONSTRAINT comments_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.comments(id) ON DELETE CASCADE;


--
-- Name: comments comments_readlist_id_readlist_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments
    ADD CONSTRAINT comments_readlist_id_readlist_id_fk FOREIGN KEY (readlist_id) REFERENCES public.readlist(id) ON DELETE CASCADE;


--
-- Name: comments comments_root_id_comments_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments
    ADD CONSTRAINT comments_root_id_comments_id_fk FOREIGN KEY (root_id) REFERENCES public.comments(id) ON DELETE CASCADE;


--
-- Name: comments comments_user_id_users_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments
    ADD CONSTRAINT comments_user_id_users_id_fk FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: comments comments_watchlist_id_watchlist_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.comments
    ADD CONSTRAINT comments_watchlist_id_watchlist_id_fk FOREIGN KEY (watchlist_id) REFERENCES public.watchlist(id) ON DELETE CASCADE;


--
-- Name: content_links content_links_chapter_id_chapters_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_links
    ADD CONSTRAINT content_links_chapter_id_chapters_id_fk FOREIGN KEY (chapter_id) REFERENCES public.chapters(id) ON DELETE CASCADE;


--
-- Name: content_links content_links_episode_id_episodes_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_links
    ADD CONSTRAINT content_links_episode_id_episodes_id_fk FOREIGN KEY (episode_id) REFERENCES public.episodes(id) ON DELETE CASCADE;


--
-- Name: curated_picks curated_picks_anime_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.curated_picks
    ADD CONSTRAINT curated_picks_anime_id_fkey FOREIGN KEY (anime_id) REFERENCES public.anime(id) ON DELETE CASCADE;


--
-- Name: curated_picks curated_picks_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.curated_picks
    ADD CONSTRAINT curated_picks_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: curated_picks curated_picks_manga_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.curated_picks
    ADD CONSTRAINT curated_picks_manga_id_fkey FOREIGN KEY (manga_id) REFERENCES public.manga(id) ON DELETE CASCADE;


--
-- Name: episode_views episode_views_anime_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.episode_views
    ADD CONSTRAINT episode_views_anime_id_fkey FOREIGN KEY (anime_id) REFERENCES public.anime(id) ON DELETE CASCADE;


--
-- Name: episode_views episode_views_episode_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.episode_views
    ADD CONSTRAINT episode_views_episode_id_fkey FOREIGN KEY (episode_id) REFERENCES public.episodes(id) ON DELETE CASCADE;


--
-- Name: episode_views episode_views_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.episode_views
    ADD CONSTRAINT episode_views_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: episodes episodes_anime_id_anime_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.episodes
    ADD CONSTRAINT episodes_anime_id_anime_id_fk FOREIGN KEY (anime_id) REFERENCES public.anime(id) ON DELETE CASCADE;


--
-- Name: follows follows_follower_id_users_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.follows
    ADD CONSTRAINT follows_follower_id_users_id_fk FOREIGN KEY (follower_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: follows follows_following_id_users_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.follows
    ADD CONSTRAINT follows_following_id_users_id_fk FOREIGN KEY (following_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: forum_replies forum_replies_thread_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_replies
    ADD CONSTRAINT forum_replies_thread_id_fkey FOREIGN KEY (thread_id) REFERENCES public.forum_threads(id) ON DELETE CASCADE;


--
-- Name: forum_replies forum_replies_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_replies
    ADD CONSTRAINT forum_replies_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: forum_threads forum_threads_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.forum_threads
    ADD CONSTRAINT forum_threads_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: invites invites_used_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invites
    ADD CONSTRAINT invites_used_by_user_id_fkey FOREIGN KEY (used_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: list_likes list_likes_list_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.list_likes
    ADD CONSTRAINT list_likes_list_id_fkey FOREIGN KEY (list_id) REFERENCES public.user_lists(id) ON DELETE CASCADE;


--
-- Name: list_likes list_likes_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.list_likes
    ADD CONSTRAINT list_likes_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: notifications notifications_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: notifications notifications_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: password_resets password_resets_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.password_resets
    ADD CONSTRAINT password_resets_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: playback_positions playback_positions_episode_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.playback_positions
    ADD CONSTRAINT playback_positions_episode_id_fkey FOREIGN KEY (episode_id) REFERENCES public.episodes(id) ON DELETE CASCADE;


--
-- Name: playback_positions playback_positions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.playback_positions
    ADD CONSTRAINT playback_positions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: readlist readlist_manga_id_manga_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.readlist
    ADD CONSTRAINT readlist_manga_id_manga_id_fk FOREIGN KEY (manga_id) REFERENCES public.manga(id) ON DELETE CASCADE;


--
-- Name: readlist readlist_user_id_users_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.readlist
    ADD CONSTRAINT readlist_user_id_users_id_fk FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: releases releases_anime_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.releases
    ADD CONSTRAINT releases_anime_id_fkey FOREIGN KEY (anime_id) REFERENCES public.anime(id) ON DELETE CASCADE;


--
-- Name: releases releases_assigned_verifier_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.releases
    ADD CONSTRAINT releases_assigned_verifier_id_fkey FOREIGN KEY (assigned_verifier_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: releases releases_manga_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.releases
    ADD CONSTRAINT releases_manga_id_fkey FOREIGN KEY (manga_id) REFERENCES public.manga(id) ON DELETE CASCADE;


--
-- Name: releases releases_published_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.releases
    ADD CONSTRAINT releases_published_by_fkey FOREIGN KEY (published_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: releases releases_reviewer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.releases
    ADD CONSTRAINT releases_reviewer_id_fkey FOREIGN KEY (reviewer_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: releases releases_uploader_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.releases
    ADD CONSTRAINT releases_uploader_id_fkey FOREIGN KEY (uploader_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: request_votes request_votes_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.request_votes
    ADD CONSTRAINT request_votes_request_id_fkey FOREIGN KEY (request_id) REFERENCES public.translation_requests(id) ON DELETE CASCADE;


--
-- Name: request_votes request_votes_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.request_votes
    ADD CONSTRAINT request_votes_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: schedule_slots schedule_slots_anime_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schedule_slots
    ADD CONSTRAINT schedule_slots_anime_id_fkey FOREIGN KEY (anime_id) REFERENCES public.anime(id) ON DELETE CASCADE;


--
-- Name: schedule_slots schedule_slots_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schedule_slots
    ADD CONSTRAINT schedule_slots_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: skip_marks skip_marks_episode_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.skip_marks
    ADD CONSTRAINT skip_marks_episode_id_fkey FOREIGN KEY (episode_id) REFERENCES public.episodes(id) ON DELETE CASCADE;


--
-- Name: subtitle_events subtitle_events_release_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subtitle_events
    ADD CONSTRAINT subtitle_events_release_id_fkey FOREIGN KEY (release_id) REFERENCES public.releases(id) ON DELETE CASCADE;


--
-- Name: subtitles subtitles_episode_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subtitles
    ADD CONSTRAINT subtitles_episode_id_fkey FOREIGN KEY (episode_id) REFERENCES public.episodes(id) ON DELETE CASCADE;


--
-- Name: subtitles subtitles_translator_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subtitles
    ADD CONSTRAINT subtitles_translator_id_fkey FOREIGN KEY (translator_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: translation_requests translation_requests_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.translation_requests
    ADD CONSTRAINT translation_requests_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_list_items user_list_items_anime_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_list_items
    ADD CONSTRAINT user_list_items_anime_id_fkey FOREIGN KEY (anime_id) REFERENCES public.anime(id) ON DELETE CASCADE;


--
-- Name: user_list_items user_list_items_list_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_list_items
    ADD CONSTRAINT user_list_items_list_id_fkey FOREIGN KEY (list_id) REFERENCES public.user_lists(id) ON DELETE CASCADE;


--
-- Name: user_list_items user_list_items_manga_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_list_items
    ADD CONSTRAINT user_list_items_manga_id_fkey FOREIGN KEY (manga_id) REFERENCES public.manga(id) ON DELETE CASCADE;


--
-- Name: user_lists user_lists_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_lists
    ADD CONSTRAINT user_lists_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: users users_banner_anime_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_banner_anime_id_fkey FOREIGN KEY (banner_anime_id) REFERENCES public.anime(id) ON DELETE SET NULL;


--
-- Name: users users_banner_manga_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_banner_manga_id_fkey FOREIGN KEY (banner_manga_id) REFERENCES public.manga(id) ON DELETE SET NULL;


--
-- Name: users users_last_verifier_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_last_verifier_id_fkey FOREIGN KEY (last_verifier_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: watch_history watch_history_anime_id_anime_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.watch_history
    ADD CONSTRAINT watch_history_anime_id_anime_id_fk FOREIGN KEY (anime_id) REFERENCES public.anime(id) ON DELETE CASCADE;


--
-- Name: watch_history watch_history_manga_id_manga_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.watch_history
    ADD CONSTRAINT watch_history_manga_id_manga_id_fk FOREIGN KEY (manga_id) REFERENCES public.manga(id) ON DELETE CASCADE;


--
-- Name: watch_history watch_history_user_id_users_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.watch_history
    ADD CONSTRAINT watch_history_user_id_users_id_fk FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: watchlist watchlist_anime_id_anime_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.watchlist
    ADD CONSTRAINT watchlist_anime_id_anime_id_fk FOREIGN KEY (anime_id) REFERENCES public.anime(id) ON DELETE CASCADE;


--
-- Name: watchlist watchlist_user_id_users_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.watchlist
    ADD CONSTRAINT watchlist_user_id_users_id_fk FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--


