# Contributing to PortShare

Thanks for taking the time to contribute. This repository is a monorepo holding
every PortShare component.

## Layout

| Path | What it is | Stack |
| --- | --- | --- |
| `server/` | Tunnel gateway, public API, billing, admin API | Go 1.26, Gin, yamux, Postgres |
| `cli/` | Headless CLI + npm/Homebrew/Chocolatey packages | Go 1.25, Cobra |
| `desktop/` | Electron app (`electron/`) and its UI (`frontend/`) | Electron, Vite, React 19 |
| `website/` | Marketing site and account pages | Next.js 16 (App Router) |
| `admin/` | Operator admin panel | Next.js 16 |
| `.github/workflows/` | CI and the release pipeline | GitHub Actions |

## Prerequisites

- **Go** 1.25+ (the `server` module declares 1.26; Go downloads the toolchain
  automatically when `GOTOOLCHAIN=auto`).
- **Node.js** 20+ and npm 10+.
- **PostgreSQL** 16 (or Docker) for the server.
- **Docker** (optional) for compose and workflow linting.

## Local development

```bash
# server (needs DATABASE_URL, PORTSHARE_ROOT_DOMAIN, CORS_ORIGINS)
cd server && go run ./cmd/api

# cli
cd cli && go build -o portshare . && ./portshare http 3000

# website / admin
cd website && npm ci && npm run dev      # http://localhost:3000
cd admin   && npm ci && npm run dev      # http://localhost:3001

# desktop
cd desktop/electron && npm ci && npm run dev
```

## Before opening a pull request

Run the same gates CI runs:

```bash
# server
cd server && go vet ./... && go test -race -cover ./...

# cli
cd cli && go vet ./... && go test ./... && go build ./...

# website / admin / desktop frontend
cd website && npm ci && npm run lint && npm run build
cd admin   && npm ci && npm run lint && npm run build

# workflows (Docker available?)
docker run --rm -v "$PWD:/repo" -w /repo rhysd/actionlint:latest \
  .github/workflows/ci.yaml .github/workflows/release.yaml \
  .github/workflows/chocolatey.yaml .github/workflows/homebrew.yaml

# compose file
docker compose -f server/docker-compose.yaml config
```

On Windows, if `go test -race` reports a toolchain version mismatch, invoke the
Go 1.26 toolchain directly
(`"$(go env GOPATH)/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.0.windows-amd64/bin/go.exe" test -race ./...`).

## Conventions

- Keep tests next to the code (`*_test.go`, `*.test.ts`); every security-relevant
  change should come with a regression test.
- Comments explain **why**, not what. Match the surrounding style.
- One logical change per commit; reference the affected component in the subject
  (e.g. `server: enforce the auth wall on pty tunnels`).
- Never commit build output, `node_modules`, or real `.env` files — the root
  `.gitignore` covers these, and CI does not need them.
- Secrets belong in the environment, not in the tree. `*.env.example` documents
  every variable a deployment needs.

## Security

Read [SECURITY.md](./SECURITY.md) before changing authentication, the PTY
terminal, the transport handshake, or billing endpoints.
