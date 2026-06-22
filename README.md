<div align="center">

<img src="https://vibexp.io/logo.svg" alt="VibeXP" width="120" />

# VibeXP

**Stop re-explaining everything to your AI.** 🧠

One shared knowledge base your prompts, rules, memory, and past work live in, that every AI tool can read from and write back to. Claude Code, Cursor, ChatGPT, Gemini, VS Code, Codex, and anything that speaks MCP.

Free, open source, and self-hostable.

[🌐 Website](https://vibexp.io) · [📚 Docs](https://docs.vibexp.io) · [✍️ Blog](https://blog.vibexp.io)

⭐ **If this solves a problem for you, please [star the repo](https://github.com/vibexp/vibexp)** so more people find it.

</div>

---

## The problem 😩

The more AI tools you use, the more you lose, and the more your team duplicates:

- 🔁 **You rewrite the same prompts** in every tool, every session.
- 🗣️ **You re-explain your context constantly** because your AI starts from scratch every time.
- 🧱 **Your knowledge is trapped in silos** that never talk to each other, and never reach your teammates.

## How VibeXP solves it ✨

VibeXP is one place for everything your AI relies on, connected to your tools over the **Model Context Protocol (MCP)**. It turns one-off chats into a knowledge base that compounds:

1. **Before a task** your AI reads the relevant prompts, rules, memory, and past work first, so it starts with everything already learned.
2. **As it works** it saves new lessons, updates memory, and stores outputs back into VibeXP.
3. **Every session after** that richer knowledge is waiting, for you and your whole team.

### What lives in VibeXP

| | |
|---|---|
| 📝 **Prompts** | Composable, reusable prompts. Reference one prompt inside another and fill in variables, so you build instead of rewrite. |
| 📐 **Blueprints** | The rules and guidelines that shape your AI's behavior, organized per tool. |
| 🧠 **Memory** | Durable context your AI reads before working and updates as it learns. |
| 📦 **Artifacts** | Versioned outputs you can diff and restore in a click. |
| 📡 **Feeds** | Agents post their work over MCP and you reply in-thread to steer them. |
| 🔎 **Semantic search** | Find anything across prompts, artifacts, blueprints, and memory by meaning, not keywords. |
| 👥 **Teams** | Invite your team and everyone's AI draws from the same knowledge base. |

> Prefer not to host it yourself? The hosted version is at **[vibexp.io](https://vibexp.io)**.

---

## Quick start (self-host) 🚀

You need [Docker](https://docs.docker.com/get-docker/) with Compose. This runs the published images plus PostgreSQL (pgvector).

```sh
git clone https://github.com/vibexp/vibexp.git
cd vibexp
docker compose up -d
```

Then open:

- 🖥️ **App:** http://localhost:5173
- ⚙️ **API health:** http://localhost:8080/health

Local evaluation uses a dev-login bypass, so there is nothing to configure to start clicking around.

<details>
<summary><strong>⚠️ Before exposing it to the internet (required config)</strong></summary>

The defaults in `docker-compose.yml` are for local evaluation only. For any real deployment, edit the `backend` service environment:

- **`ENCRYPTION_KEY`** exactly 32 bytes. Generate one: `openssl rand -base64 24 | cut -c1-32`
- **`DB_PASSWORD`** change it from the default.
- **`DEV_LOGIN_ENABLED`** set to `false` and configure [WorkOS AuthKit](https://workos.com) (`WORKOS_API_KEY`, `WORKOS_CLIENT_ID`, `WORKOS_REDIRECT_URI`).
- **`FRONTEND_BASE_URL` / `CORS_ALLOWED_ORIGINS`** set to your real public URLs.
- **Semantic search & file attachments** are optional, opt-in services. See the comments in `docker-compose.yml` and the [docs](https://docs.vibexp.io).

Data persists in the `pgdata` volume.

</details>

---

## Connect your AI tools 🔌

VibeXP exposes a single MCP endpoint. Sign in once in the browser, no API keys to copy-paste and babysit. For example, with Claude Code against the hosted service:

```sh
claude mcp add --transport http vibexp https://connect.vibexp.io/mcp/v1/common
```

For a self-hosted instance, point your tool at your own deployment's MCP endpoint instead. Full per-tool instructions (Cursor, VS Code, Gemini CLI, ChatGPT, Codex) are in the **[docs](https://docs.vibexp.io)**.

---

## For developers 🛠️

VibeXP is a monorepo with two independently deployable components:

- **`backend/`** Go REST API. Spec-first OpenAPI, PostgreSQL + pgvector, MCP endpoint, WorkOS auth.
- **`frontend/`** Vite + React + TypeScript SPA, served by nginx in production.

<details>
<summary><strong>Local development</strong></summary>

Local development uses the `Makefile` (not the root `docker-compose.yml`, which runs the published images).

```sh
# Backend: Postgres + hot-reload API
make backend-run-dev

# Frontend: Vite dev server on http://localhost:5173
make frontend-run-dev
```

Common checks:

```sh
make backend-test    make backend-lint    make backend-check
make frontend-test   make frontend-lint   make frontend-type-check
```

**Pre-commit hooks are mandatory.** Install them once per clone so every commit is gated on the same checks CI runs:

```sh
pre-commit install
```

See [`CLAUDE.md`](./CLAUDE.md) for the full contributor guide and conventions.

</details>

Contributions are welcome. Branch off `main`, open a PR, and let CI pass. 💚

---

## License 📄

[AGPL-3.0-or-later](./LICENSE).

<div align="center">

If VibeXP saves you from re-explaining yourself to your AI, **[give it a ⭐](https://github.com/vibexp/vibexp)** and tell a teammate.

[Website](https://vibexp.io) · [Docs](https://docs.vibexp.io) · [Blog](https://blog.vibexp.io)

</div>
