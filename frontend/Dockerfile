# Production image: SvelteKit adapter-node build.
# NOTE: $shared imports are type-only and erased at build time, so the sibling
# shared/ directory is not needed in this build context. A runtime import from
# $shared would fail this build loudly — keep those imports type-only.
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
# Baked into the client bundle: the browser-side API base (Caddy proxies /api).
ARG VITE_API_URL=http://localhost:3000
ENV VITE_API_URL=$VITE_API_URL
RUN npm run build && npm prune --omit=dev

FROM node:22-alpine
RUN adduser -D -H svelte
USER svelte
WORKDIR /app
ENV NODE_ENV=production PORT=3000
COPY --from=build /app/build ./build
COPY --from=build /app/node_modules ./node_modules
COPY --from=build /app/package.json .
EXPOSE 3000
CMD ["node", "build"]
