# AGENTS.md

Guidance for AI coding agents and human contributors working in this repository.

## Project

c4-forge is an open-source, self-hosted tool for modeling software architecture with the C4 model. A team runs it on its own server and edits diagrams together in real time.

- What we build and why: [`docs/requirements.md`](docs/requirements.md). Requirement IDs (e.g. `DG-8`, `NF-16`) are referenced throughout the repo.
- How and why key technical choices were made: [`docs/adr/`](docs/adr/).
- What we build next, step by step: [`docs/roadmap.md`](docs/roadmap.md). Work on one roadmap step at a time.

Read the relevant requirements and ADRs before changing anything they cover.

## Current stage

Design is in progress; there is no application code yet. Do not scaffold or generate code unless the task explicitly asks for it.

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
