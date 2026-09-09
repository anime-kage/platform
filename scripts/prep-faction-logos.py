#!/usr/bin/env python3
"""Turn the raw faction logos in /home/alexlv/Logo into transparent PNGs.

Nine of the ten sources are flat JPEG-style artwork with an opaque backdrop
(five dark, four white); only Black Clover ships real alpha. We key the
backdrop out by flood-filling inward from the border, so a white *inside* the
logo survives -- a global colour key would punch holes through it.

Edge pixels get a partial alpha from how far they sit from the backdrop
colour, which keeps the antialiasing instead of leaving a hard jaggy cut.

The arcade renders on both the dark and the light theme, so a logo that is
essentially one tone gets a soft halo in the opposite tone; without it a
black wordmark disappears on the dark theme and a white one on the light.
"""
import io, pathlib, sys
import numpy as np
from PIL import Image, ImageDraw, ImageFilter

SRC = pathlib.Path("/home/alexlv/Logo")
OUT = pathlib.Path("/home/alexlv/anime-kage/anime-kage/frontend/static/arcade/factions")
MAP = {"One-Piece.png": "onepiece", "Naruto.png": "naruto", "frieren.png": "frieren",
       "jujutsu-kaisen.png": "jjk", "Bleach.png": "bleach", "Gintama.webp": "gintama",
       "Attack on Titan.png": "aot", "HxH.png": "hxh",
       "Demon-Slayer.png": "demonslayer", "Black-Clover.png": "blackclover"}

FILL_TOL = 90      # summed per-channel distance PIL's floodfill tolerates
SOFT_LO, SOFT_HI = 40, 150   # fringe pixels ramp from transparent to opaque here
# The largest a logo is ever drawn is the 104x68 faction card, so 320px still
# leaves headroom for a 3x display; at 512 the ten logos came to 1.2MB.
MAXDIM = 320

# Naruto's source is a wallpaper, not a wordmark: a dark red Konoha symbol on a
# textured near-black ground. Flood fill cannot separate them -- subject and
# backdrop share a luminance -- so that one is keyed on redness instead, and
# the result is brightened because the original is far too dark to read small.
CHROMA = {"naruto": dict(lo=22, hi=55, boost=2.4, drop_bottom=0.14)}


def chroma_key(im, lo, hi, boost, drop_bottom=0.0):
    """Key on how much a pixel's red beats its other channels."""
    px = np.asarray(im.convert("RGB")).astype(np.float32)
    h = px.shape[0]
    red = px[:, :, 0] - np.maximum(px[:, :, 1], px[:, :, 2])
    a = np.clip((red - lo) / (hi - lo), 0, 1)
    if drop_bottom:
        a[int(h * (1 - drop_bottom)):, :] = 0   # a caption sits along the bottom
    rgb = np.clip(px * boost, 0, 255)
    return Image.fromarray(np.dstack([rgb, a * 255]).astype(np.uint8), "RGBA"), None

def key_background(im):
    """Return an RGBA image whose border-connected backdrop is transparent."""
    rgb = im.convert("RGB")
    px = np.asarray(rgb).astype(np.int16)
    h, w = px.shape[:2]

    # A colour that certainly does not occur, so the fill is its own mask.
    magic = (255, 0, 255)
    while ((px[:, :, 0] == magic[0]) & (px[:, :, 1] == magic[1]) & (px[:, :, 2] == magic[2])).any():
        magic = (magic[0], magic[1], magic[2] - 1)

    work = rgb.copy()
    seeds = ([(x, 0) for x in range(w)] + [(x, h - 1) for x in range(w)] +
             [(0, y) for y in range(h)] + [(w - 1, y) for y in range(h)])
    for s in seeds:
        ImageDraw.floodfill(work, s, magic, thresh=FILL_TOL)

    wk = np.asarray(work)
    mask = (wk[:, :, 0] == magic[0]) & (wk[:, :, 1] == magic[1]) & (wk[:, :, 2] == magic[2])
    if not mask.any():
        return im.convert("RGBA"), None

    bg = np.median(px[mask], axis=0)
    dist = np.abs(px - bg).sum(axis=2)

    alpha = np.full((h, w), 255, np.uint8)
    alpha[mask] = 0

    # Soften only the pixels hugging the keyed region, so interior detail that
    # happens to match the backdrop colour keeps its full opacity.
    near = mask.copy()
    for _ in range(3):
        g = near.copy()
        g[1:, :] |= near[:-1, :]; g[:-1, :] |= near[1:, :]
        g[:, 1:] |= near[:, :-1]; g[:, :-1] |= near[:, 1:]
        near = g
    fringe = near & ~mask
    ramp = np.clip((dist - SOFT_LO) / (SOFT_HI - SOFT_LO), 0, 1)
    alpha[fringe] = (ramp[fringe] * 255).astype(np.uint8)

    out = np.dstack([np.asarray(rgb), alpha])
    return Image.fromarray(out, "RGBA"), tuple(int(v) for v in bg)

def halo(im, colour, radius=4):
    pad = radius * 3
    big = Image.new("RGBA", (im.width + pad * 2, im.height + pad * 2), (0, 0, 0, 0))
    big.paste(im, (pad, pad))
    a = big.getchannel("A")
    grown = a.filter(ImageFilter.MaxFilter(radius * 2 + 1)).filter(ImageFilter.GaussianBlur(radius * 0.7))
    glow = Image.new("RGBA", big.size, colour + (0,))
    glow.putalpha(grown.point(lambda v: min(255, int(v * 1.35))))
    return Image.alpha_composite(glow, big)

def main():
    OUT.mkdir(parents=True, exist_ok=True)
    for fn, code in MAP.items():
        src = SRC / fn
        if not src.exists():
            print(f"    {code:12s} MISSING {fn}"); continue
        im = Image.open(src)
        pre = im.convert("RGBA")
        if code in CHROMA:
            im, bg = chroma_key(im, **CHROMA[code])
        elif (np.asarray(pre)[:, :, 3] < 8).mean() < 0.02:    # effectively opaque
            im, bg = key_background(im)
        else:
            im, bg = pre, None

        a = np.asarray(im)
        keep = a[:, :, 3] > 24
        if not keep.any():
            print(f"    {code:12s} keyed away to nothing, skipped"); continue
        ys, xs = np.where(keep)
        im = im.crop((xs.min(), ys.min(), xs.max() + 1, ys.max() + 1))

        a = np.asarray(im).astype(np.float32)
        wgt = a[:, :, 3] / 255.0
        lum = float(((0.2126 * a[:, :, 0] + 0.7152 * a[:, :, 1] + 0.0722 * a[:, :, 2]) * wgt).sum()
                    / max(wgt.sum(), 1))
        tone = "dark" if lum < 110 else ("light" if lum > 165 else "mid")
        if tone == "dark":
            im = halo(im, (255, 255, 255))
        elif tone == "light":
            im = halo(im, (12, 12, 16))

        if max(im.size) > MAXDIM:
            s = MAXDIM / max(im.size)
            im = im.resize((max(1, round(im.width * s)), max(1, round(im.height * s))), Image.LANCZOS)
        # Palette + alpha keeps these at a few tens of KB without a visible
        # difference at the size they are drawn.
        flat = im.convert("RGB").quantize(colors=255, method=Image.FASTOCTREE, dither=Image.NONE)
        flat.putpalette(flat.getpalette()[:255 * 3] + [0, 0, 0])
        idx = np.asarray(flat).copy()
        idx[np.asarray(im)[:, :, 3] < 128] = 255      # last slot is the clear one
        out_im = Image.fromarray(idx, "P")
        out_im.putpalette(flat.getpalette())
        out_im.info["transparency"] = 255
        # Keep whichever is smaller: hard-edged palette alpha loses the soft
        # halo, so anything with real translucency stays full RGBA.
        buf_p, buf_r = io.BytesIO(), io.BytesIO()
        out_im.save(buf_p, "PNG", optimize=True)
        im.save(buf_r, "PNG", optimize=True)
        soft = ((np.asarray(im)[:, :, 3] > 8) & (np.asarray(im)[:, :, 3] < 248)).mean()
        use_pal = soft < 0.06 and buf_p.tell() < buf_r.tell()
        (OUT / f"{code}.png").write_bytes((buf_p if use_pal else buf_r).getvalue())
        kb = (OUT / f"{code}.png").stat().st_size / 1024
        print(f"    {code:12s} {im.size[0]:3d}x{im.size[1]:3d}  lum={lum:5.1f} {tone:5s} "
              f"soft={soft*100:4.1f}%  {'palette' if use_pal else 'rgba   '} {kb:6.1f} KB")

main()
