# Production image: static binary on a minimal base.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# api is the server; migrate/populate/autoupdate are ops one-shots
# (docker compose --profile setup run db-migrate, cron for autoupdate).
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/ ./cmd/...

FROM alpine:3.21
# ffmpeg + ffprobe: lift embedded soft-sub tracks out of uploaded video, and
# burn the RO track into a copy for embed-only hosts (PLAN 12).
#
# font-noto is not optional for that burn. libass is compiled into this ffmpeg,
# but the image ships no fonts at all — /usr/share/fonts does not exist and
# fc-list returns 0 — so a burn would silently produce a video with no visible
# text. Noto Sans is also verified to carry the *correct* Romanian glyphs: ș and
# ț with comma below (U+0219/U+021B), not just the Turkish cedilla forms
# (U+015F/U+0163) that many fonts substitute.
RUN adduser -D -H api && apk add --no-cache ca-certificates tzdata ffmpeg font-noto fontconfig
WORKDIR /app
# Create the two writable directories IN THE IMAGE, owned by api. Docker
# initialises a named volume from whatever is at its mountpoint, ownership
# included — if the mountpoint doesn't exist, it makes one owned by root and
# the non-root process cannot write a single avatar or staged release. The
# failure only appears once the volumes are attached, which is to say only in
# production.
RUN mkdir -p uploads staging && chown -R api:api /app
USER api
COPY --from=build --chown=api:api /out/ /app/
EXPOSE 3000
ENTRYPOINT ["/app/api"]
