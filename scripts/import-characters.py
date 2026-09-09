#!/usr/bin/env python3
"""Harvest characters from Jikan into game_characters.

    DATABASE_URL=... python3 import-characters.py --top 200
    DATABASE_URL=... python3 import-characters.py 5114 9253

Only Main characters and well-liked Supporting ones are kept. One Piece alone
returns 1,477 entries; importing every walk-on would make both the card pool and
the guessing game worse, because nobody can name a background extra.
"""
import json, os, shutil, subprocess, sys, time

import psycopg2

API = "https://api.jikan.moe/v4/anime/{}/characters"
BUFFER = float(os.getenv("JIKAN_BUFFER", "2.0"))   # Jikan documents 3 req/s
SUPPORT_MIN_FAVS = 200                              # keep only notable supports
UA = "Anime-Kage/1.0 (character import)"


def fetch(mal_id: int, tries: int = 4):
    """Fetched with curl, not a Python HTTP client.

    Measured 2026-08-30: curl gets HTTP 200 from this endpoint while urllib and
    requests both get 504, same URL, same User-Agent, seconds apart. Headers are
    not the difference -- Accept and Accept-Encoding variants all failed -- so it
    is below that, most likely HTTP/2 versus 1.1 through Cloudflare. curl works,
    so curl is what this uses.

    Jikan also 504s genuinely and intermittently, hence the retries.
    """
    if not shutil.which("curl"):
        raise RuntimeError("curl is required")
    last = None
    for n in range(tries):
        p = subprocess.run(
            ["curl", "-s", "-A", UA, "--max-time", "30", API.format(mal_id)],
            capture_output=True, text=True)
        if p.returncode == 0 and p.stdout.strip().startswith("{"):
            try:
                d = json.loads(p.stdout)
                if "data" in d:
                    return d
                last = RuntimeError(d.get("message") or "unexpected payload")
            except json.JSONDecodeError as e:
                last = e
        else:
            last = RuntimeError(f"curl exit {p.returncode}")
        if n < tries - 1:
            time.sleep(BUFFER * (n + 2))
    raise last


def rows_for(anime_id: int, payload: dict):
    out = []
    for e in payload.get("data", []):
        ch = e.get("character") or {}
        cid, name = ch.get("mal_id"), ch.get("name")
        img = ((ch.get("images") or {}).get("jpg") or {}).get("image_url")
        favs = e.get("favorites") or 0
        role = (e.get("role") or "").lower()
        if not (cid and name and img):
            continue
        if role != "main" and favs < SUPPORT_MIN_FAVS:
            continue
        out.append((cid, anime_id, name, img, favs))
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
        else:
            c.execute("""SELECT id, mal_id, title FROM anime
                          WHERE mal_id IS NOT NULL AND score IS NOT NULL
                            AND NOT EXISTS (SELECT 1 FROM game_characters g WHERE g.anime_id = anime.id)
                          ORDER BY score DESC NULLS LAST
                          LIMIT %s""", (top or 100,))
        targets = c.fetchall()

    print(f"{len(targets)} series to check, {BUFFER}s apart")
    added = failed = 0
    for anime_id, mal_id, title in targets:
        try:
            rows = rows_for(anime_id, fetch(mal_id))
        except Exception as e:  # noqa: BLE001 - one bad series must not stop the run
            print(f"  !! {title[:40]}: {type(e).__name__}: {e}")
            failed += 1
            time.sleep(BUFFER)
            continue
        if rows:
            with conn.cursor() as c:
                c.executemany("""
                    INSERT INTO game_characters (mal_char_id, anime_id, name, image_url, favorites)
                    VALUES (%s, %s, %s, %s, %s)
                    ON CONFLICT (mal_char_id) DO UPDATE
                       SET favorites = EXCLUDED.favorites,
                           image_url = EXCLUDED.image_url""", rows)
            added += len(rows)
            print(f"  ok {title[:40]:42s} {len(rows)} character(s)")
        time.sleep(BUFFER)

    print(f"done — {added} characters stored, {failed} errored")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
