# CodeXArtifacts

Independent open-source artifact platform based on owner-authorized Pictor source.
The first release keeps Go + React, SQLite, local assets and one server replica.
Do not migrate the backend to TypeScript or Cloudflare native in this scope.

## Working conventions

- Preserve artifact sandbox isolation and shared HTTP/MCP authorization.
- Product name: CodeXArtifacts. Existing lowercase `pictor` protocol identifiers
  and environment variables remain compatible; do not bulk-rename versioned runtimes.
- Brand: approved offset-field mark, berry #AD4968, charcoal and warm paper;
  approved Astryx-inspired website with actual product screenshots. English README and website, bilingual product UI.
- No company-specific hosts, real user data, credentials, private research or
  upstream internal Git history in the publication set.
- Read scoped AGENTS.md before editing a module.
- `make hooks` once per clone; `make verify` for the final candidate.
- Run focused checks during iteration; browser checks for UI and sign-in changes.
- Do not enable remote CI, buy services, or change unrelated deployments.
- Keep changes small and use Conventional Commits.

## Project skills

- [github-issue-create](.agents/skills/github-issue-create/SKILL.md): refine and
  review product need, evidence and acceptance criteria before creating an issue.
- [github-issue-implement](.agents/skills/github-issue-implement/SKILL.md): implement
  a referenced issue with isolation, verification and review; PR/merge/runtime
  actions follow the user's authorized scope.

Skills live in the repository; `make hooks` exposes them to Claude Code through
an ignored `.claude/skills` link while preserving any existing custom entry.
