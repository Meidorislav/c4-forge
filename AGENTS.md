# AGENTS.md

Guidance for AI coding agents and human contributors working in this repository.

## Project

c4-forge is an open-source, self-hosted tool for modeling software architecture with the C4 model. A team runs it on its own server and edits diagrams together in real time.

- What we build and why: [`docs/requirements.md`](docs/requirements.md). Requirement IDs (e.g. `DG-8`, `NF-16`) are referenced throughout the repo.
- How and why key technical choices were made: [`docs/adr/`](docs/adr/).
- What we build next, step by step: [`docs/roadmap.md`](docs/roadmap.md). Work on one roadmap step at a time.

Read the relevant requirements and ADRs before changing anything they cover.

## Current stage

Implementation follows [`docs/roadmap.md`](docs/roadmap.md), one step at a time. Do not build ahead of the current step.

## Repository layout

```
cmd/c4forge/            entry point only: signals and exit code
internal/app/           assembles the service (flags, config, logger, dependencies) and runs it
internal/config/        configuration from C4FORGE_* environment variables
internal/server/        HTTP router (chi), middleware, handlers (/healthz, /readyz)
internal/db/            PostgreSQL pool (pgx), startup checks, migrations (goose)
internal/db/migrations/ SQL migrations, embedded into the binary
internal/db/dbtest/     real PostgreSQL databases for integration tests (Testcontainers)
internal/buildinfo/     version and revision of the binary
docker-compose.yml      local PostgreSQL for development
docs/                   requirements, ADRs, roadmap
```

## Build and test

Requires Go (version in `go.mod`), [golangci-lint](https://golangci-lint.run) v2 and Docker.

```bash
make db-up      # start local PostgreSQL (docker compose), wait until healthy
make run        # run the server against it with text logs
make db-down    # stop it, keeping data; make db-reset also deletes the data
make build      # binary in bin/c4forge
make test       # all tests with the race detector
make lint       # linters
make fmt        # format code
make tidy       # tidy go.mod/go.sum
```

- **Configuration** comes from environment variables with the `C4FORGE_` prefix; see `internal/config`. `C4FORGE_DATABASE_URL` is required; `make run` points it at the local database. Override any variable from the shell, e.g. `C4FORGE_HTTP_ADDR=:8081 make run`.
- **Integration tests** use `dbtest.NewDatabase(t)`, which gives each test its own empty database on a PostgreSQL container started once per test binary. Docker must be running; without it these tests are skipped locally and fail in CI. `C4FORGE_TEST_POSTGRES_IMAGE` selects the image (default `postgres:18`).
- **CI** (`.github/workflows/ci.yml`) runs lint, tests with `-race` on PostgreSQL 16 and 18, build and a `go mod tidy` check on every pull request. Keep it green.

## Database migrations

- Add a new file `internal/db/migrations/NNNNN_description.sql` with the next number and a `-- +goose Up` section. Migrations run automatically at startup.
- Migrations are forward-only (ADR-0003): no `Down` sections, and every migration must work on the data of the previous release.
- Never edit a migration that has been merged; add a new one instead.
- Use SQL for schema changes. Write a Go migration only when the change needs logic SQL cannot express well.

## How we work

- **Decide first, then build.** A significant technical decision (library, storage, protocol, architecture) gets an ADR before code depends on it. Keep ADRs short: context, options, decision, consequences.
- **ADRs are immutable once accepted.** Do not rewrite an accepted ADR in substance; write a new one that supersedes it and update the index in `docs/adr/README.md`.
- **Small steps.** One change = one roadmap step with its tests, reviewable on its own. Never generate large parts of the application in one go.
- **Tests come with the change**, not later.
- **Ask when a decision belongs to the maintainer.** Scope, priorities and product behaviour are not for the agent to settle silently.

## Writing

- **Everything in the repository is in English:** docs, ADRs, code, comments, commit messages, UI text.
- Describe features on their own terms. Do not describe c4-forge by comparison with other products ("like X", "an alternative to Y"). Structurizr DSL and Mermaid may be named as formats we are compatible with.
- Keep docs plain and specific. Reference requirement IDs and ADR numbers instead of repeating their content.

## Commits

[Conventional Commits](https://www.conventionalcommits.org/): `type(scope): summary`, e.g. `chore(docs): init ADR-0003`, `feat(engine): apply field-level operations`. Keep one logical change per commit.

## Hard constraints

These come from the requirements; do not work around them.

- **Licensing (NF-16).** The project is Apache-2.0. Dependencies must be permissively licensed (or weak copyleft such as MPL-2.0). No dependency may require a license key, a paid license or a watermark in production. Check the license before adding a dependency.
- **Air-gapped operation (NF-3).** No network calls at runtime: no CDNs, remote fonts, telemetry or license checks. Bundle every asset.
- **Server-side validation (NF-9).** The server validates every change; clients are never trusted to keep the model consistent.
- **Durability (NF-8).** A change acknowledged by the server must already be committed to PostgreSQL (ADR-0002).
- **Stack.** Backend in Go, frontend in React, storage in PostgreSQL (ADR-0001…0003).
