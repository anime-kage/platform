#!/usr/bin/env python3
"""Mirror a series from the Hetzner Storage Box to Streamtape, and register the
resulting iframes as content_links.

Per episode: pull the file from the box, POST it to Streamtape into the series
folder, verify the sha256 the server reports against the bytes we sent, insert
an `embed` row, then delete the local copy before moving to the next.

Streamtape can only ever be an iframe source: there is no streamtape resolver in
resolver.Default(), so it cannot feed the AK player. Only an `extract` row does
that, and of the hosts in use only vtbe has a resolver. This script therefore
never writes an extract row.

Resumable: every completed episode is recorded, so an interrupted run costs one
episode rather than the batch.

    streamtape-upload.py --slug naruto --limit 2 --staging
    streamtape-upload.py --slug naruto --staging --apply
"""
import argparse, hashlib, json, os, random, re, subprocess, sys, time

API = 'https://api.streamtape.com'

# Converted size as a fraction of the source, below which the conversion is
# assumed to have truncated the video. See the verify pass for the measurements.
MIN_CONVERT_RATIO = 0.25


def env_val(path, name):
    for line in open(path, encoding='utf-8'):
        if line.startswith(name + '='):
            return line.split('=', 1)[1].strip().strip('"').strip("'")
    sys.exit(f"lipseste {name} din {path}")


def api(path, params, login, key):
    """One GET against the Streamtape API, returning result or exiting."""
    import urllib.parse, urllib.request
    q = urllib.parse.urlencode({**params, 'login': login, 'key': key})
    with urllib.request.urlopen(f"{API}/{path}?{q}", timeout=60) as r:
        d = json.loads(r.read())
    if d.get('status') != 200:
        sys.exit(f"API {path}: {d.get('status')} {d.get('msg')}")
    return d.get('result')


def psql(sql, env_path, compose, service, capture=True):
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


def sha256(path):
    h = hashlib.sha256()
    with open(path, 'rb') as f:
        for chunk in iter(lambda: f.read(1 << 20), b''):
            h.update(chunk)
    return h.hexdigest()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--slug', required=True, help='series slug, also the box folder name')
    ap.add_argument('--limit', type=int, default=0, help='stop after N episodes (0 = all)')
    ap.add_argument('--priority', type=int, default=10003,
                    help='embed priority; higher is offered first')
    # Pacing, same shape as download_anime.py's --stagger/--jitter. A fixed
    # gap is a metronome, which is exactly the pattern rate-limiters notice;
    # the jitter makes the interval irregular. Uploads take minutes each, so
    # this is about not looking automated rather than about throughput.
    ap.add_argument('--pause', type=float, default=3.0,
                    help='base seconds between uploads')
    ap.add_argument('--jitter', type=float, default=2.0,
                    help='random extra seconds added to each pause; 0 for exactly --pause')
    ap.add_argument('--apply', action='store_true', help='write to the database')
    # Register what is already uploaded without sending anything new. Without
    # this, writing the rows for a partial run means --limit 0, which would
    # upload the entire remaining series as a side effect of a database write.
    ap.add_argument('--register-only', action='store_true',
                    help='skip uploading; only write rows for episodes already done')
    ap.add_argument('--convert-wait', type=int, default=600,
                    help='max seconds to wait for conversions before verifying')
    # Re-send a flagged file after remuxing it. Streamtape's encoder truncated
    # naruto_ep009 twice from the same source, deterministically, while the file
    # decoded end to end with no errors. Re-containering it (dropping the
    # redundant mov_text track, which the burned-in subtitles make unnecessary)
    # converted correctly at 46%. Cause unknown; this is the working remedy.
    ap.add_argument('--retry-suspect', action='store_true',
                    help='remux and re-upload files the verify pass flagged')
    ap.add_argument('--staging', action='store_true')
    ap.add_argument('--root', default='/home/alexlv/anime-kage/anime-kage')
    ap.add_argument('--bot', default='/home/alexlv/bot')
    # Kept apart from --bot on purpose: --bot is where the .env with the
    # host credentials lives, while the resume state is bulk and belongs
    # in its own folder. Pointing --bot at the state folder would leave
    # the scripts unable to find their API keys.
    ap.add_argument('--state', default='/home/alexlv/bot/state')
    ap.add_argument('--work', default='/tmp/streamtape-stage')
    a = ap.parse_args()

    if a.staging:
        env, compose, service = os.path.join(a.root, '.env.staging'), \
            os.path.join(a.root, 'docker-compose.staging.yml'), 'postgres-staging'
    else:
        env, compose, service = os.path.join(a.root, '.env'), \
            os.path.join(a.root, 'docker-compose.prod.yml'), 'postgres'
    print(f"  tinta      : {'STAGING' if a.staging else 'PRODUCTIE'}")
    print(f"  scriere DB : {'DA' if a.apply else 'NU (dry run)'}")

    bot_env = os.path.join(a.bot, '.env')
    login = env_val(bot_env, 'STREAMTAPE_LOGIN')
    key = env_val(bot_env, 'STREAMTAPE_KEY')

    state_path = os.path.join(a.state, f'streamtape_{a.slug}.json')
    state = json.load(open(state_path)) if os.path.exists(state_path) else {}
    folder = state.get('folder_id')

    # One folder per series, created once and remembered. Creating it again on a
    # resume would leave a second empty folder behind every time.
    if not folder:
        folder = api('file/createfolder', {'name': a.slug}, login, key)['folderid']
        state['folder_id'] = folder
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
            print(f"  [{n}/{len(todo)}] {name}: nu pot deduce numarul episodului, sar peste")
            continue
        ep = int(m.group(1))
        local = os.path.join(a.work, name)

        subprocess.run(['rclone', 'copy', f'hetzner:anime/{a.slug}/{name}', a.work],
                       check=True, capture_output=True)
        digest = sha256(local)

        # Hand the expected digest to Streamtape so a corrupted upload is
        # rejected server-side rather than stored and discovered by a viewer.
        up = api('file/ul', {'folder': folder, 'sha256': digest}, login, key)['url']
        r = subprocess.run(['curl', '-s', '-m', '3600', '-F',
                            f'file1=@{local};type=video/mp4', up],
                           capture_output=True, text=True)
        try:
            res = json.loads(r.stdout)['result']
        except Exception:
            print(f"  [{n}/{len(todo)}] {name}: raspuns neasteptat: {r.stdout[:150]}")
            os.remove(local)
            continue

        if res.get('sha256') != digest:
            print(f"  [{n}/{len(todo)}] ep{ep}: SHA NEPOTRIVIT, nu inregistrez")
            os.remove(local)
            continue

        done[name] = {'ep': ep, 'id': res['id'], 'url': res['url'], 'sha256': digest,
                      'src_size': os.path.getsize(local)}
        json.dump(state, open(state_path, 'w'), indent=1)
        os.remove(local)
        print(f"  [{n}/{len(todo)}] ep{ep:>3} -> {res['id']}  ({int(res['size'])/1048576:.0f} MB trimis)")
        if n < len(todo):
            nap = a.pause + random.uniform(0, a.jitter)
            time.sleep(nap)

    # Streamtape's converter can truncate a file and still report it as
    # "converted". sha256 cannot catch that: it proves the bytes we sent
    # arrived, and the damage happens afterwards, on their side. naruto_ep009
    # went up intact (185 MB, 23.6 min) and came out as a 16 MB, 3 minute file.
    #
    # The converted size is the signal available through the API, which returns
    # no duration. Measured across 38 real conversions the ratios were 8.7% for
    # the broken one, then 33% and up for every good one, with many at 100%
    # where no re-encode happened. A 25% floor sits in that gap.
    #
    # A proxy, not proof: a conversion that lost the last two minutes would come
    # in near 90% and pass. It catches gross truncation, which is the failure
    # actually observed.
    suspect = {}
    if done:
        # Wait for the encoder queue to drain first. Measuring a file that is
        # still converting reads a partial size, so the check would flag good
        # files and clear bad ones at random. Bounded, because a stuck convert
        # must not hang the run forever.
        for _ in range(a.convert_wait // 15):
            running = api('file/runningconverts', {}, login, key) or []
            mine = [c for c in running if (c.get('name') or '') in done]
            if not mine:
                break
            log_line = ', '.join(f"{c['name']} {float(c.get('progress') or 0)*100:.0f}%" for c in mine[:3])
            print(f"  astept conversia: {len(mine)} in curs ({log_line})", flush=True)
            time.sleep(15)

        listing = api('file/listfolder', {'folder': folder}, login, key)
        got = {f['name']: f['size'] for f in (listing.get('files') or [])}
        # Episodes uploaded before src_size was recorded (or by an older run)
        # still need checking, so fall back to asking the box. One listing, not
        # one call per file.
        if any('src_size' not in v for v in done.values()):
            ls = subprocess.run(['rclone', 'ls', f'hetzner:anime/{a.slug}', '--include', '*.mp4'],
                                capture_output=True, text=True)
            for line in ls.stdout.splitlines():
                parts = line.split(None, 1)
                if len(parts) == 2 and parts[1].strip() in done:
                    done[parts[1].strip()].setdefault('src_size', int(parts[0]))
        for name, v in done.items():
            conv, src = got.get(name), v.get('src_size')
            # Absent from the host is a suspect, not a skip. A file deleted or
            # never stored is at least as broken as a truncated one, and an
            # earlier version of this check quietly passed over it: ep009 was
            # removed from Streamtape and still got registered, leaving a row
            # pointing at nothing.
            if conv is None:
                suspect[name] = 0.0
                continue
            if not src:
                continue
            ratio = conv / src
            v['ratio'] = round(ratio, 3)
            if ratio < MIN_CONVERT_RATIO:
                suspect[name] = ratio
        json.dump(state, open(state_path, 'w'), indent=1)
    if suspect and a.retry_suspect:
        print(f"  reincerc {len(suspect)} fisier(e) prin remux", flush=True)
        for name in list(suspect):
            local = os.path.join(a.work, name)
            fixed = os.path.join(a.work, 'fix_' + name)
            subprocess.run(['rclone', 'copy', f'hetzner:anime/{a.slug}/{name}', a.work],
                           check=True, capture_output=True)
            # Video and audio only, stream-copied: no re-encode, so this costs
            # seconds and cannot degrade the picture. faststart moves the moov
            # atom to the front, which some encoders are happier with.
            r = subprocess.run(['ffmpeg', '-v', 'error', '-y', '-i', local,
                                '-map', '0:v:0', '-map', '0:a:0', '-c', 'copy',
                                '-movflags', '+faststart', fixed], capture_output=True, text=True)
            os.remove(local)
            if r.returncode != 0 or not os.path.exists(fixed):
                print(f"    {name}: remux esuat, sar peste", flush=True)
                continue
            digest = sha256(fixed)
            up = api('file/ul', {'folder': folder, 'sha256': digest}, login, key)['url']
            resp = subprocess.run(['curl', '-s', '-m', '3600', '-F',
                                   f'file1=@{fixed};type=video/mp4', up],
                                  capture_output=True, text=True)
            os.remove(fixed)
            try:
                res = json.loads(resp.stdout)['result']
            except Exception:
                print(f"    {name}: raspuns neasteptat la reincarcare", flush=True)
                continue
            done[name].update({'id': res['id'], 'url': res['url'], 'sha256': digest,
                               'src_size': int(res['size']), 'remuxed': True})
            suspect.pop(name)
            json.dump(state, open(state_path, 'w'), indent=1)
            print(f"    {name}: reincarcat ca {res['id']}", flush=True)
        print("  verifica din nou cu o rulare --register-only dupa ce se converteste", flush=True)

    if suspect:
        print(f"  SUSPECTE ({len(suspect)}), nu le inregistrez:")
        for name, r in sorted(suspect.items()):
            print(f"    {name}  {r*100:.1f}% din marimea sursei")
        print("  reincarca-le cu --retry-suspect")

    # Register the iframes. Embed only, for the reason in the module docstring.
    rows = [(v['ep'], v['id']) for n, v in done.items() if n not in suspect]
    # Episodes whose file is truncated or gone. Holding them back from
    # registration is not enough on its own: whatever link they already carry
    # stays live and still points at the bad file. naruto_ep009 sat active for
    # hours that way, pointing at an upload that had been deleted.
    bad_eps = sorted({done[n]['ep'] for n in suspect if n in done})
    if not rows and not bad_eps:
        print("  nimic de inregistrat"); return
    values = ",\n".join(f"({ep},'{fid}')" for ep, fid in sorted(rows)) or "(NULL,NULL)"
    bad_values = ",".join(str(e) for e in bad_eps)
    sql = f"""
BEGIN;
CREATE TEMP TABLE incoming(ep int, fid text) ON COMMIT DROP;
INSERT INTO incoming(ep, fid) VALUES
{values};

-- Retire only the rows THIS script put there, and only when the link has
-- actually changed. Matching on `hosting_url LIKE '%streamtape%'` retired the
-- catalogue's own third-party streamtape links for the episode as well, and
-- retired the row the previous run inserted before inserting an identical one,
-- which is where the duplicate dead rows came from.
UPDATE content_links cl SET is_active = false
  FROM episodes e JOIN anime a ON a.id = e.anime_id
  JOIN incoming i ON i.ep = e.episode_number
 WHERE cl.episode_id = e.id AND a.slug = '{a.slug}' AND cl.is_active
   AND cl.provider = 'streamtape'
   AND cl.priority = {a.priority}
   AND cl.hosting_url <> 'https://streamtape.com/e/'||i.fid;

INSERT INTO content_links (episode_id, hosting_url, language, is_active, kind, provider, priority)
SELECT e.id, 'https://streamtape.com/e/'||i.fid, 'ro', true, 'embed', 'streamtape', {a.priority}
  FROM incoming i JOIN anime a ON a.slug = '{a.slug}'
  JOIN episodes e ON e.anime_id = a.id AND e.episode_number = i.ep
 WHERE NOT EXISTS (
   SELECT 1 FROM content_links x
    WHERE x.episode_id = e.id AND x.kind = 'embed'
      AND x.hosting_url = 'https://streamtape.com/e/'||i.fid);

-- Retire the links of the flagged episodes too, so a truncated file stops
-- being served while it waits for a remux and re-upload.
UPDATE content_links cl SET is_active = false
  FROM episodes e JOIN anime a ON a.id = e.anime_id
 WHERE cl.episode_id = e.id AND a.slug = '{a.slug}' AND cl.is_active
   AND cl.provider = 'streamtape'
   AND cl.priority = {a.priority}
   AND e.episode_number IN ({bad_values or 'NULL'});

-- Count only the episodes this run touched. Counting every active
-- streamtape row for the series reported 423 on a two-episode run, because
-- the catalogue already carries hundreds of third-party streamtape links.
SELECT 'randuri noi (ale mele): '||count(*) FROM content_links cl
  JOIN episodes e ON e.id = cl.episode_id JOIN anime a ON a.id = e.anime_id
  JOIN incoming i ON i.ep = e.episode_number
 WHERE a.slug='{a.slug}' AND cl.provider='streamtape' AND cl.is_active
   AND cl.hosting_url = 'https://streamtape.com/e/'||i.fid;
{"COMMIT;" if a.apply else "ROLLBACK;"}
"""
    out = psql(sql, env, compose, service)
    print("  " + "\n  ".join(l for l in out.splitlines() if l.strip()))


if __name__ == '__main__':
    main()
