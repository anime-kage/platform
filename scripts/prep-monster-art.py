#!/usr/bin/env python3
"""Cut the white backdrop off the monster art and size it for Discord.

Every source arrives on white, some as flat RGB and some with an alpha channel
that is not actually used. Discord composites an embed image onto a dark card,
so a white rectangle behind each monster reads as a sticker rather than a
creature.

Flood filled inward from the border rather than keyed on colour, so white
INSIDE the art -- Kurama's teeth, the Titan's eyes -- survives. Edge pixels get
a partial alpha from how far they sit from the backdrop, which keeps the
antialiasing instead of leaving a jagged cut.
"""
import pathlib, sys
import numpy as np
from PIL import Image, ImageDraw

SRC = pathlib.Path("/home/alexlv/monsters")
OUT = pathlib.Path("/home/alexlv/anime-kage/anime-kage/frontend/static/arcade/monsters")
MAP = {"slime": "slime", "hollow": "hollow", "salibaman": "salibaman",
       "goblin": "goblin", "titan": "titan", "chimera-ant": "chimera",
       "kaido": "kaido", "kurama": "kurama"}
# Kaido's backdrop is a grey gradient rather than flat white, so it needs a
# looser tolerance than art that sits on a clean sheet.
TOL = {}
DEFAULT_TOL = 70

# Kurama arrives with a transparency checkerboard PAINTED IN -- somebody saved a
# screenshot of a transparent PNG, so the grey and white squares are real pixels
# on a fixed grid. A border flood fill cannot cross between the two tones and
# tears the art apart instead. Keyed globally on those exact colours instead,
# which is safe here and only here: 45% of the image is checkerboard and only
# 0.6% of light pixels belong to the fox, which is drawn entirely in orange,
# red and black.
DESPECKLE = {"kaido"}
# Both of these arrive with the checkerboard painted in, and each was saved by a
# different tool, so the grey square is a different shade in each. Neither piece
# of art contains white -- Kurama is orange, red and black; Kaido is blue, tan,
# purple and yellow with a black mane -- which is what makes a global key safe
# here and unsafe almost anywhere else.
CHECKER = {
    "kurama": [(255, 255, 255), (238, 238, 239)],
    "kaido":  [(255, 255, 255), (214, 214, 214)],
}


def key_checker(im, tones, tol=24):
    rgba = im.convert("RGBA")
    px = np.asarray(rgba).astype(int)
    mask = np.zeros(px.shape[:2], bool)
    for t in tones:
        mask |= np.abs(px[:, :, :3] - np.array(t)).sum(axis=2) <= tol
    out = px.copy()
    out[mask, 3] = 0
    return Image.fromarray(out.astype(np.uint8), "RGBA")
SOFT_LO, SOFT_HI = 30, 140
MAXDIM = 640


def key_background(im, tol):
    rgb = im.convert("RGB")
    px = np.asarray(rgb).astype(np.int16)
    h, w = px.shape[:2]
    magic = (255, 0, 255)
    while ((px[:, :, 0] == magic[0]) & (px[:, :, 1] == magic[1]) & (px[:, :, 2] == magic[2])).any():
        magic = (magic[0], magic[1], magic[2] - 1)

    work = rgb.copy()
    seeds = ([(x, 0) for x in range(w)] + [(x, h - 1) for x in range(w)] +
             [(0, y) for y in range(h)] + [(w - 1, y) for y in range(h)])
    for s in seeds:
        ImageDraw.floodfill(work, s, magic, thresh=tol)

    wk = np.asarray(work)
    mask = (wk[:, :, 0] == magic[0]) & (wk[:, :, 1] == magic[1]) & (wk[:, :, 2] == magic[2])
    if not mask.any():
        return im.convert("RGBA")

    bg = np.median(px[mask], axis=0)
    dist = np.abs(px - bg).sum(axis=2)
    alpha = np.full((h, w), 255, np.uint8)
    alpha[mask] = 0
    near = mask.copy()
    for _ in range(3):
        g = near.copy()
        g[1:, :] |= near[:-1, :]; g[:-1, :] |= near[1:, :]
        g[:, 1:] |= near[:, :-1]; g[:, :-1] |= near[:, 1:]
        near = g
    fringe = near & ~mask
    ramp = np.clip((dist - SOFT_LO) / (SOFT_HI - SOFT_LO), 0, 1)
    alpha[fringe] = (ramp[fringe] * 255).astype(np.uint8)

    # Respect alpha the source already had.
    orig = np.asarray(im.convert("RGBA"))[:, :, 3]
    alpha = np.minimum(alpha, orig)
    return Image.fromarray(np.dstack([np.asarray(rgb), alpha]), "RGBA")


def despeckle(im, max_region=400, light=205):
    """Drop small pale flecks left stranded along a keyed edge.

    Kaido's backdrop is a grey gradient, so the flood fill stopped at pockets it
    could not reach and left 500-odd specks around the outline. Only regions that
    are BOTH small and touching transparency go: an interior highlight of the
    same colour is surrounded by art, never by a hole, so his teeth and the
    lightning survive.
    """
    from collections import deque
    px = np.asarray(im.convert("RGBA")).astype(int)
    h, w = px.shape[:2]
    clear = px[:, :, 3] < 8
    pale = (px[:, :, :3] > light).all(axis=2) & (px[:, :, 3] > 128)
    seen = np.zeros((h, w), bool)
    out = px.copy()
    dropped = 0
    for y0, x0 in zip(*np.where(pale)):
        if seen[y0, x0]:
            continue
        q, region, touches = deque([(y0, x0)]), [], False
        seen[y0, x0] = True
        while q:
            cy, cx = q.popleft()
            region.append((cy, cx))
            for dy, dx in ((1, 0), (-1, 0), (0, 1), (0, -1)):
                ny, nx = cy + dy, cx + dx
                if not (0 <= ny < h and 0 <= nx < w):
                    touches = True   # the image edge counts as outside
                    continue
                if clear[ny, nx]:
                    touches = True
                elif pale[ny, nx] and not seen[ny, nx]:
                    seen[ny, nx] = True
                    q.append((ny, nx))
        if touches and len(region) <= max_region:
            for cy, cx in region:
                out[cy, cx, 3] = 0
            dropped += len(region)
    if dropped:
        print(f"               despeckled {dropped} stray pale pixels")
    return Image.fromarray(out.astype(np.uint8), "RGBA")


def main() -> int:
    OUT.mkdir(parents=True, exist_ok=True)
    for stem, code in MAP.items():
        src = SRC / f"{stem}.png"
        if not src.exists():
            print(f"    {code}: MISSING {stem}.png"); continue
        raw = Image.open(src)
        # Art that already carries alpha has been cut out by whoever made it,
        # and keying it again is destructive rather than helpful: the Slime is a
        # near-white blue, so a white key ate through its body and left the
        # highlight floating. Trust an existing cut-out; only key art that
        # arrives flat on a backdrop.
        already = np.asarray(raw.convert("RGBA"))[:, :, 3]
        if code in CHECKER:
            im = key_checker(raw, CHECKER[code])
            print(f"    {code:10s} painted checkerboard keyed on colour")
        elif (already < 8).mean() > 0.10:
            im = raw.convert("RGBA")
            print(f"    {code:10s} already transparent, left alone")
        else:
            im = key_background(raw, TOL.get(code, DEFAULT_TOL))
        if code in DESPECKLE:
            im = despeckle(im)
        a = np.asarray(im)
        keep = a[:, :, 3] > 24
        if keep.any():
            ys, xs = np.where(keep)
            im = im.crop((xs.min(), ys.min(), xs.max() + 1, ys.max() + 1))
        if max(im.size) > MAXDIM:
            s = MAXDIM / max(im.size)
            im = im.resize((max(1, round(im.width * s)), max(1, round(im.height * s))), Image.LANCZOS)
        im.save(OUT / f"{code}.png", optimize=True)
        clear = (np.asarray(im)[:, :, 3] < 8).mean() * 100
        print(f"    {code:10s} {im.size[0]:3d}x{im.size[1]:<4d} transparent={clear:5.1f}%  "
              f"{(OUT / f'{code}.png').stat().st_size // 1024:3d} KB")
    return 0


sys.exit(main())
