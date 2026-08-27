# Contributing

This guide assumes you have never used git. If you have, skip to
[The loop](#the-loop) — it is short.

You do not need to be a programmer to help. Fixing a Romanian typo is a real
contribution and follows exactly the same steps as anything else, which is why
it is a good first task.

## Before anything else

1. **Ask for an invite to the GitHub organisation.** You cannot push without it.
2. **Install** [Docker Desktop](https://docs.docker.com/get-started/),
   [VS Code](https://code.visualstudio.com/), and
   [Node 22](https://nodejs.org/) — Node 22 specifically, see below.
3. **Clone the repository** and start it:

   ```bash
   git clone https://github.com/<org>/platform.git
   cd platform
   ./dev.sh
   ```

   The site is then at http://localhost:5173. It is yours alone: a separate,
   empty database on your own machine. Nothing you do here can reach the real
   site or its members.

**Node 22, not newer.** `.npmrc` sets `engine-strict=true` and the production
image is `node:22-alpine`, so `npm install` refuses on other versions rather
than letting your machine drift from the server. `nvm use 22` first.

## The loop

Every change, from a typo to a feature, goes the same way.

**1. Start from an up-to-date `main`, on a new branch.**

```bash
git checkout main
git pull
git checkout -b fix/typo-on-profile
```

Name it `fix/…` or `feat/…` plus a couple of words. The branch is scratch space
— it is deleted after merging, so nothing here is permanent.

**2. Make the change and look at it.** `./dev.sh` reloads as you save.

**3. Check your work before asking anyone else to.**

```bash
cd frontend  && npm test && npm run check
cd backend-go && go test ./... && go vet ./...
```

Run only the half you touched. If `npm run check` reports errors in files you
did not open, they were already there — say so in the pull request rather than
trying to fix them.

**4. Commit.**

```bash
git add -A
git commit -m "Fix Romanian typo on the profile page"
```

Write the message as *what changed and why*, in a full sentence. "fix" and
"update" tell the next person nothing.

**5. Push and open a pull request.**

```bash
git push -u origin fix/typo-on-profile
```

GitHub prints a link. Open it, fill in the template, and submit.

**6. Wait for review.** Change requests are normal and are about the change, not
about you. Push more commits to the same branch and the pull request updates
itself.

**7. Merge.** A maintainer merges and deletes the branch. Your change is live on
the next deploy.

## Working with an AI assistant

Most contributors here use Claude Code inside VS Code, and that is expected
rather than tolerated. Two things make the difference between help and mess:

**Point it at `CLAUDE.md` first.** It documents the architecture and the
decisions behind it, so an assistant that has read it suggests things that fit
this codebase instead of generic advice.

**Read the diff before you commit it.** You are asking for review from a person
whose time is limited, so the pull request should be something you understand
and can explain. `git diff` before every commit. If you cannot say what a line
does, ask the assistant to explain it — that is a fair question and a fast one.

An assistant will occasionally rewrite far more than you asked. Check the file
list in your pull request. If it touches files unrelated to your change, that is
worth undoing before anyone else reads it.

## House rules

- **Never commit `.env` or anything holding a password, token or key.** A secret
  pushed once is public forever. If it happens, say so at once — the fix is
  rotating the credential, not deleting the file.
- **Never edit a migration that already exists** in
  `backend-go/cmd/migrate/migrations/`. They have already run on the live
  database. Add a new numbered file instead.
- **No CSS framework.** Styling is plain CSS with design tokens, by decision.
  Do not add Tailwind or anything like it.
- **Do not change the database schema, nginx config, or `docker-compose.prod.yml`
  without asking first.** Those can take the site down. `CODEOWNERS` routes them
  for review automatically.

## Where things are

```
frontend/      the site — SvelteKit, pages under src/routes/
backend-go/    the API — Go, one file per area under internal/handler/
shared/        TypeScript types used on both sides
monitoring/    Prometheus and Grafana config
nginx/         production reverse proxy
```

`CLAUDE.md` at the root is the fullest description of how it fits together, and
it is the best thing to read before your second contribution.

## If you are stuck

Ask on Discord. Being stuck for an hour on setup is not a sign you are not up to
this — it usually means an instruction here is unclear, and telling us which one
is itself a useful contribution.
