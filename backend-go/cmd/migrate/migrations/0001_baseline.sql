--
-- PostgreSQL database dump
--


-- Dumped from database version 15.15 (Debian 15.15-1.pgdg13+1)
-- Dumped by pg_dump version 15.15 (Debian 15.15-1.pgdg13+1)

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
    broadcast_time character varying(10)
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
-- Name: anime_translations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.anime_translations (
    id integer NOT NULL,
    anime_id integer NOT NULL,
    language character varying(10) NOT NULL,
    title character varying(500),
    synopsis text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: anime_translations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.anime_translations_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: anime_translations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.anime_translations_id_seq OWNED BY public.anime_translations.id;


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
    created_at timestamp without time zone DEFAULT now() NOT NULL
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
-- Name: episodes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.episodes (
    id integer NOT NULL,
    anime_id integer NOT NULL,
    episode_number integer NOT NULL,
    title character varying(500),
    air_date date,
    duration integer,
    created_at timestamp without time zone DEFAULT now() NOT NULL
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
    updated_at timestamp without time zone DEFAULT now() NOT NULL
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
-- Name: manga_translations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.manga_translations (
    id integer NOT NULL,
    manga_id integer NOT NULL,
    language character varying(10) NOT NULL,
    title character varying(500),
    synopsis text,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: manga_translations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.manga_translations_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: manga_translations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.manga_translations_id_seq OWNED BY public.manga_translations.id;


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
    favorites jsonb
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
-- Name: anime_translations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anime_translations ALTER COLUMN id SET DEFAULT nextval('public.anime_translations_id_seq'::regclass);


--
-- Name: chapters id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chapters ALTER COLUMN id SET DEFAULT nextval('public.chapters_id_seq'::regclass);


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
-- Name: episodes id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.episodes ALTER COLUMN id SET DEFAULT nextval('public.episodes_id_seq'::regclass);


--
-- Name: follows id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.follows ALTER COLUMN id SET DEFAULT nextval('public.follows_id_seq'::regclass);


--
-- Name: manga id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.manga ALTER COLUMN id SET DEFAULT nextval('public.manga_id_seq'::regclass);


--
-- Name: manga_translations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.manga_translations ALTER COLUMN id SET DEFAULT nextval('public.manga_translations_id_seq'::regclass);


--
-- Name: readlist id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.readlist ALTER COLUMN id SET DEFAULT nextval('public.readlist_id_seq'::regclass);


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
-- Name: anime_translations anime_translations_anime_id_language_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anime_translations
    ADD CONSTRAINT anime_translations_anime_id_language_key UNIQUE (anime_id, language);


--
-- Name: anime_translations anime_translations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anime_translations
    ADD CONSTRAINT anime_translations_pkey PRIMARY KEY (id);


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
-- Name: manga_translations manga_translations_manga_id_language_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.manga_translations
    ADD CONSTRAINT manga_translations_manga_id_language_key UNIQUE (manga_id, language);


--
-- Name: manga_translations manga_translations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.manga_translations
    ADD CONSTRAINT manga_translations_pkey PRIMARY KEY (id);


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
-- Name: anime_score_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX anime_score_idx ON public.anime USING btree (score);


--
-- Name: anime_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX anime_status_idx ON public.anime USING btree (status);


--
-- Name: anime_year_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX anime_year_idx ON public.anime USING btree (year);


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
-- Name: comments_root_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX comments_root_id_idx ON public.comments USING btree (root_id);


--
-- Name: follows_follower_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX follows_follower_id_idx ON public.follows USING btree (follower_id);


--
-- Name: follows_following_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX follows_following_id_idx ON public.follows USING btree (following_id);


--
-- Name: manga_score_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX manga_score_idx ON public.manga USING btree (score);


--
-- Name: manga_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX manga_status_idx ON public.manga USING btree (status);


--
-- Name: readlist_user_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX readlist_user_id_idx ON public.readlist USING btree (user_id);


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
-- Name: anime_translations anime_translations_anime_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.anime_translations
    ADD CONSTRAINT anime_translations_anime_id_fkey FOREIGN KEY (anime_id) REFERENCES public.anime(id) ON DELETE CASCADE;


--
-- Name: chapters chapters_manga_id_manga_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.chapters
    ADD CONSTRAINT chapters_manga_id_manga_id_fk FOREIGN KEY (manga_id) REFERENCES public.manga(id);


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
    ADD CONSTRAINT content_links_chapter_id_chapters_id_fk FOREIGN KEY (chapter_id) REFERENCES public.chapters(id);


--
-- Name: content_links content_links_episode_id_episodes_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_links
    ADD CONSTRAINT content_links_episode_id_episodes_id_fk FOREIGN KEY (episode_id) REFERENCES public.episodes(id);


--
-- Name: episodes episodes_anime_id_anime_id_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.episodes
    ADD CONSTRAINT episodes_anime_id_anime_id_fk FOREIGN KEY (anime_id) REFERENCES public.anime(id);


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
-- Name: manga_translations manga_translations_manga_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.manga_translations
    ADD CONSTRAINT manga_translations_manga_id_fkey FOREIGN KEY (manga_id) REFERENCES public.manga(id) ON DELETE CASCADE;


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


