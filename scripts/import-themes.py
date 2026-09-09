#!/usr/bin/env python3
"""Harvest opening/ending links from AnimeThemes into game_themes.

Metadata only — the media stays on their CDN. Run it repeatedly; it upserts, so
stopping and restarting costs nothing.

    DATABASE_URL=... python3 import-themes.py --top 300   # the 300 best-scored
    DATABASE_URL=... python3 import-themes.py 52991 5114   # specific mal ids
    DATABASE_URL=... python3 import-themes.py              # everything missing

--top is usually what you want. The catalogue has ~2,100 series and the long
tail is stuff nobody would recognise from its opening, so harvesting all of it
buys puzzles that are not guessable.

AnimeThemes asks for civility rather than publishing a hard limit, so this waits
BUFFER seconds between calls. A full catalogue pass therefore takes hours, which
is fine for a job nothing is waiting on.
"""
import json, os, sys, time, urllib.parse, urllib.request

import psycopg2

API = "https://api.animethemes.moe/anime"
BUFFER = float(os.getenv("THEMES_BUFFER", "1.5"))
UA = "Anime-Kage/1.0 (theme import)"


def fetch(mal_id: int):
    q = urllib.parse.urlencode({
        "filter[has]": "resources",
        "filter[site]": "MyAnimeList",
        "filter[external_id]": mal_id,
        "include": "animethemes.animethemeentries.videos.audio,animethemes.song",
    })
    req = urllib.request.Request(f"{API}?{q}", headers={"User-Agent": UA})
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.load(r)


def rows_for(anime_id: int, payload: dict):
    """One row per theme: the first entry that carries both audio and video."""
    out = []
    for a in payload.get("anime", []):
        for t in a.get("animethemes", []):
            slug = (t.get("slug") or "").upper()
            if not slug:
                continue
            song = (t.get("song") or {}).get("title")
            audio = video = None
            for e in t.get("animethemeentries", []):
                for v in e.get("videos", []):
                    au = v.get("audio") or {}
                    if au.get("link") and not audio:
                        audio, video = au["link"], v.get("link")
            if audio:
                out.append((anime_id, slug, "ED" if slug.startswith("ED") else "OP",
                            song, audio, video))
    return out


def main() -> int:
    dsn = os.getenv("DATABASE_URL")
    if not dsn:
        print("DATABASE_URL is required", file=sys.stderr)
        return 2
    args = sys.argv[1:]
    top = None
    if "--top" in args:
        i = args.index("--top")
        top = int(args[i + 1])
        del args[i:i + 2]
    wanted = [int(a) for a in args]

    conn = psycopg2.connect(dsn)
    conn.autocommit = True
    with conn.cursor() as c:
        if wanted:
            c.execute("SELECT id, mal_id, title FROM anime WHERE mal_id = ANY(%s)", (wanted,))
        elif top:
            c.execute("""SELECT id, mal_id, title FROM anime
                          WHERE mal_id IS NOT NULL AND score IS NOT NULL
                            AND NOT EXISTS (SELECT 1 FROM game_themes g WHERE g.anime_id = anime.id)
                          ORDER BY score DESC NULLS LAST LIMIT %s""", (top,))
        else:
            c.execute("""SELECT id, mal_id, title FROM anime
                          WHERE mal_id IS NOT NULL
                            AND NOT EXISTS (SELECT 1 FROM game_themes g WHERE g.anime_id = anime.id)
                          ORDER BY score DESC NULLS LAST""")
        targets = c.fetchall()

    print(f"{len(targets)} series to check, {BUFFER}s apart")
    added = skipped = failed = 0
    for anime_id, mal_id, title in targets:
        try:
            rows = rows_for(anime_id, fetch(mal_id))
        except Exception as e:  # noqa: BLE001 - one bad series must not stop the run
            print(f"  !! {title[:40]}: {type(e).__name__}: {e}")
            failed += 1
            time.sleep(BUFFER)
            continue
        if not rows:
            skipped += 1
        else:
            with conn.cursor() as c:
                c.executemany("""
                    INSERT INTO game_themes (anime_id, slug, kind, song_title, audio_url, video_url)
                    VALUES (%s, %s, %s, %s, %s, %s)
                    ON CONFLICT (anime_id, slug) DO UPDATE
                       SET song_title = EXCLUDED.song_title,
                           audio_url  = EXCLUDED.audio_url,
                           video_url  = EXCLUDED.video_url""", rows)
            added += len(rows)
            print(f"  ok {title[:40]:42s} {len(rows)} theme(s)")
        time.sleep(BUFFER)

    print(f"done — {added} themes stored, {skipped} without any, {failed} errored")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
