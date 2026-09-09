#!/usr/bin/env python3
"""Mirror a series from the Hetzner Storage Box to mp4upload, and register the
resulting iframes as content_links.

Sibling of streamtape-upload.py, but the API differs in three ways that matter:

  * Upload is two steps. /api/upload/server hands back a server URL and a
    sess_id; the file is POSTed there, and only afterwards can it be moved into
    a folder with /api/file/set_folder. Streamtape takes the folder id at
    upload time, mp4upload does not.
  * There is no sha256 on upload. Integrity is checked by comparing the size
    the API reports back against the bytes we sent.
  * file/info reports the ORIGINAL size, not a converted one, so the
    converted/source ratio that catches Streamtape's truncated encodes has no
    equivalent here. A short upload is caught; a bad re-encode is not
    detectable through this API.

Priority 10002 by default: below streamtape (10003) and above vtbe (10001),
which is the order asked for.

    mp4upload-upload.py --slug naruto --limit 1 --staging
    mp4upload-upload.py --slug naruto --staging --apply
"""
import argparse, json, os, random, re, subprocess, sys, time
import urllib.parse, urllib.request

API = 'https://www.mp4upload.com/api'


def env_val(path, name):
    for line in open(path, encoding='utf-8'):
        if line.startswith(name + '='):
            return line.split('=', 1)[1].strip().strip('"').strip("'")
    sys.exit(f"lipseste {name} din {path}")


# mp4upload rejects Python's default User-Agent with a 403, while the identical
# request through curl succeeds. Send a real one.
UA = 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36'


def fetch(url):
    req = urllib.request.Request(url, headers={'User-Agent': UA, 'Accept': 'application/json'})
    with urllib.request.urlopen(req, timeout=60) as r:
        return json.loads(r.read())


def api(path, params, key):
    q = urllib.parse.urlencode({**params, 'key': key})
    d = fetch(f"{API}/{path}?{q}")
    if d.get('status') != 200:
        sys.exit(f"API {path}: {d.get('status')} {d.get('msg')}")
    return d.get('result')


def psql(sql, env_path, compose, service):
    user = db = None
    for line in open(env_path, encoding='utf-8'):
        if line.startswith('DB_USER='): user = line.split('=', 1)[1].strip()
        if line.startswith('DB_NAME='): db = line.split('=', 1)[1].strip()
    cmd = ['docker', 'compose', '--env-file', env_path, '-f', compose, 'exec', '-T',
           service, 'psql', '-U', user, '-d', db, '-v', 'ON_ERROR_STOP=1', '-t', '-A', '-F', '|']
    r = subprocess.run(cmd, input=sql, capture_output=True, text=True)
    if r.returncode != 0:
        sys.exit(f"psql failed:\n{r.stderr}")
    return r.stdout.strip()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--slug', required=True)
    ap.add_argument('--limit', type=int, default=0)
    ap.add_argument('--priority', type=int, default=10002)
    ap.add_argument('--pause', type=float, default=3.0)
    ap.add_argument('--jitter', type=float, default=2.0)
    ap.add_argument('--apply', action='store_true')
    ap.add_argument('--register-only', action='store_true')
    ap.add_argument('--staging', action='store_true')
    ap.add_argument('--root', default='/home/alexlv/anime-kage/anime-kage')
    ap.add_argument('--bot', default='/home/alexlv/bot')
    # Kept apart from --bot on purpose: --bot is where the .env with the
    # host credentials lives, while the resume state is bulk and belongs
    # in its own folder. Pointing --bot at the state folder would leave
    # the scripts unable to find their API keys.
    ap.add_argument('--state', default='/home/alexlv/bot/state')
    ap.add_argument('--work', default='/tmp/mp4upload-stage')
    a = ap.parse_args()

    if a.staging:
        env, compose, service = os.path.join(a.root, '.env.staging'), \
            os.path.join(a.root, 'docker-compose.staging.yml'), 'postgres-staging'
    else:
        env, compose, service = os.path.join(a.root, '.env'), \
            os.path.join(a.root, 'docker-compose.prod.yml'), 'postgres'
    print(f"  tinta      : {'STAGING' if a.staging else 'PRODUCTIE'}")
    print(f"  scriere DB : {'DA' if a.apply else 'NU (dry run)'}")

    key = env_val(os.path.join(a.bot, '.env'), 'MP4UPLOAD_API_KEY')
    state_path = os.path.join(a.state, f'mp4upload_{a.slug}.json')
    state = json.load(open(state_path)) if os.path.exists(state_path) else {}

    folder = state.get('fld_id')
    if not folder:
        folder = api('folder/create', {'name': a.slug}, key)['fld_id']
        state['fld_id'] = folder
        json.dump(state, open(state_path, 'w'), indent=1)
    print(f"  folder     : {folder}")

    done = state.setdefault('done', {})
    listing = subprocess.run(['rclone', 'lsf', f'hetzner:anime/{a.slug}', '--include', '*.mp4'],
                             capture_output=True, text=True)
    files = sorted(l.strip() for l in listing.stdout.splitlines() if l.strip())
    # Distinguish "the listing failed" from "the listing is empty". rclone
    # returning non-zero leaves stdout blank, and treating that as an empty
    # series marked five real series permanently failed when the Storage Box
    # ran out of SFTP connections under three concurrent scripts.
    if listing.returncode != 0:
        sys.exit(f"rclone lsf a esuat pentru {a.slug}: {listing.stderr.strip()[:200]}")
    if not files:
        sys.exit(f"niciun .mp4 in hetzner:anime/{a.slug}")

    todo = [] if a.register_only else [f for f in files if f not in done]
    if a.limit:
        todo = todo[:a.limit]
    print(f"  pe box     : {len(files)} | deja urcate: {len(done)} | de urcat acum: {len(todo)}")

    os.makedirs(a.work, exist_ok=True)
    for n, name in enumerate(todo, 1):
        m = re.search(r'(\d{1,4})(?!.*\d)', os.path.splitext(name)[0])
        if not m:
            print(f"  [{n}/{len(todo)}] {name}: nu pot deduce episodul, sar peste", flush=True)
            continue
        ep = int(m.group(1))
        local = os.path.join(a.work, name)
        subprocess.run(['rclone', 'copy', f'hetzner:anime/{a.slug}/{name}', a.work],
                       check=True, capture_output=True)
        src_size = os.path.getsize(local)

        # sess_id sits beside `result` in the envelope rather than inside it,
        # so this reads the raw response instead of going through api().
        env_json = fetch(f"{API}/upload/server?{urllib.parse.urlencode({'key': key})}")
        url, sess = env_json['result'], env_json['sess_id']

        resp = subprocess.run(['curl', '-s', '-m', '3600',
                               '-F', f'sess_id={sess}',
                               '-F', f'file=@{local};type=video/mp4', url],
                              capture_output=True, text=True)
        os.remove(local)
        try:
            res = json.loads(resp.stdout)
            entry = res[0] if isinstance(res, list) else res
            code = entry['file_code']
            if entry.get('file_status') != 'OK':
                raise ValueError(entry.get('file_status'))
        except Exception as exc:
            print(f"  [{n}/{len(todo)}] ep{ep}: upload esuat ({exc}): {resp.stdout[:120]}", flush=True)
            continue

        # No hash from this API, so size is the only integrity signal available.
        info = api('file/info', {'file_code': code}, key) or []
        got = next((f for f in info if str(f.get('filecode') or f.get('file_code')) == code), None)
        reported = int(got.get('size') or 0) if got else 0
        if reported and reported != src_size:
            print(f"  [{n}/{len(todo)}] ep{ep}: MARIME DIFERITA "
                  f"({reported} vs {src_size}), nu inregistrez", flush=True)
            continue

        api('file/set_folder', {'file_code': code, 'fld_id': folder}, key)
        done[name] = {'ep': ep, 'code': code, 'src_size': src_size}
        json.dump(state, open(state_path, 'w'), indent=1)
        print(f"  [{n}/{len(todo)}] ep{ep:>3} -> {code}  ({src_size/1048576:.0f} MB)", flush=True)
        if n < len(todo):
            time.sleep(a.pause + random.uniform(0, a.jitter))

    rows = [(v['ep'], v['code']) for v in done.values()]
    if not rows:
        print("  nimic de inregistrat"); return
    values = ",\n".join(f"({ep},'{c}')" for ep, c in sorted(rows))
    sql = f"""
BEGIN;
CREATE TEMP TABLE incoming(ep int, code text) ON COMMIT DROP;
INSERT INTO incoming(ep, code) VALUES
{values};

-- Only this script's own rows, and only when the link actually changed. The
-- old `LIKE '%mp4upload.com%'` also retired the catalogue's third-party
-- mp4upload links for the episode, and retired the previous run's identical
-- row before re-inserting it.
UPDATE content_links cl SET is_active = false
  FROM episodes e JOIN anime a ON a.id = e.anime_id
  JOIN incoming i ON i.ep = e.episode_number
 WHERE cl.episode_id = e.id AND a.slug = '{a.slug}' AND cl.is_active
   AND cl.provider = 'mp4upload'
   AND cl.priority = {a.priority}
   AND cl.hosting_url <> 'https://www.mp4upload.com/embed-'||i.code||'.html';

INSERT INTO content_links (episode_id, hosting_url, language, is_active, kind, provider, priority)
SELECT e.id, 'https://www.mp4upload.com/embed-'||i.code||'.html', 'ro', true,
       'embed', 'mp4upload', {a.priority}
  FROM incoming i JOIN anime a ON a.slug = '{a.slug}'
  JOIN episodes e ON e.anime_id = a.id AND e.episode_number = i.ep
 WHERE NOT EXISTS (
   SELECT 1 FROM content_links x
    WHERE x.episode_id = e.id AND x.kind = 'embed'
      AND x.hosting_url = 'https://www.mp4upload.com/embed-'||i.code||'.html');

SELECT 'randuri noi (ale mele): '||count(*) FROM content_links cl
  JOIN episodes e ON e.id = cl.episode_id JOIN anime a ON a.id = e.anime_id
  JOIN incoming i ON i.ep = e.episode_number
 WHERE a.slug='{a.slug}' AND cl.provider='mp4upload' AND cl.is_active
   AND cl.hosting_url = 'https://www.mp4upload.com/embed-'||i.code||'.html';
{"COMMIT;" if a.apply else "ROLLBACK;"}
"""
    print("  " + "\n  ".join(l for l in psql(sql, env, compose, service).splitlines() if l.strip()))
    print("  APLICAT" if a.apply else "  DRY RUN")


if __name__ == '__main__':
    main()
