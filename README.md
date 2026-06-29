<div align="center">

<img src="https://vibexp.io/logo.svg" alt="VibeXP" width="120" />

# VibeXP

**Stop re-explaining everything to your AI.** 🧠

One shared knowledge base your prompts, rules, memory, and past work live in, that every AI tool can read from and write back to. Claude Code, Cursor, ChatGPT, Gemini, VS Code, Codex, and anything that speaks MCP.

Free, open source, and self-hostable.

[🌐 Website](https://vibexp.io?utm_source=github&utm_medium=readme&utm_campaign=brand_link&utm_content=header) · [📚 Docs](https://docs.vibexp.io?utm_source=github&utm_medium=readme&utm_campaign=docs_link&utm_content=header) · [✍️ Blog](https://blog.vibexp.io?utm_source=github&utm_medium=readme&utm_campaign=blog_link&utm_content=header)

⭐ **If this solves a problem for you, please [star the repo](https://github.com/vibexp/vibexp)** so more people find it.

</div>

---

## The problem 😩

The more AI tools you use, the more you lose, and the more your team duplicates:

- 🔁 **You rewrite the same prompts** in every tool, every session.
- 🗣️ **You re-explain your context constantly** because your AI starts from scratch every time.
- 🧱 **Your knowledge is trapped in silos** that never talk to each other, and never reach your teammates.

## The concept 🎬

A short walkthrough of the idea behind VibeXP and how it changes the way you work with AI:

https://github.com/user-attachments/assets/8b211880-1a15-46da-ba47-e451658b80ea

## How VibeXP solves it ✨

VibeXP is one place for everything your AI relies on, connected to your tools over the **Model Context Protocol (MCP)** (Claude Code CLI, Cowork, Codex, Gemini CLI, Cursor, VS Code, ChatGPT, and more). It turns one-off chats into a knowledge base that compounds:

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

---

## Quick start (self-host) 🚀

You need [Docker](https://docs.docker.com/get-docker/) with Compose. This runs the published combined image (the SPA and API are served from one port) plus PostgreSQL (pgvector).

```sh
git clone https://github.com/vibexp/vibexp.git
cd vibexp
docker compose up -d
```

Then open:

- 🖥️ **App:** http://localhost:8080
- ⚙️ **API health:** http://localhost:8080/health

Local evaluation uses a dev-login bypass, so there is nothing to configure to start clicking around.

<details>
<summary><strong>⚠️ Before exposing it to the internet (required config)</strong></summary>

The defaults in `docker-compose.yml` are for local evaluation only. For any real deployment, edit the `app` service environment:

- **`ENCRYPTION_KEY`** exactly 32 bytes. Generate one: `openssl rand -base64 24 | cut -c1-32`
- **`DB_PASSWORD`** change it from the default.
- **`DEV_LOGIN_ENABLED`** set to `false`, set a `SESSION_ENCRYPTION_KEY` (`openssl rand -hex 32`), and configure an identity provider via `AUTH_PROVIDERS` (e.g. `google`, `github`, or a generic `oidc` issuer) with the matching `*_CLIENT_ID` / `*_CLIENT_SECRET` / `*_REDIRECT_URI`.
- **`FRONTEND_BASE_URL`** set to your real public URL (the single origin that serves both the SPA and the API — same-origin, so there is no separate frontend URL and no CORS to configure).
- **Branding / analytics (optional)** rebrand the SPA at deploy time with `VITE_*` env vars (served via `/config.js`, no rebuild) — see the `app` service comments in `docker-compose.yml`.
- **Semantic search & file attachments** are optional, opt-in services. See the comments in `docker-compose.yml` and the [docs](https://docs.vibexp.io?utm_source=github&utm_medium=readme&utm_campaign=docs_link&utm_content=self_host).

Data persists in the `pgdata` volume.

</details>

---

## Connect your AI tools 🔌

VibeXP exposes a single MCP endpoint. Sign in once in the browser, no API keys to copy-paste and babysit. Point your tool at your own deployment's MCP endpoint, for example with Claude Code:

```sh
claude mcp add --transport http vibexp http://localhost:8080/mcp/v1/common
```

Swap `localhost:8080` for your deployment's public URL. Full per-tool instructions (Cursor, VS Code, Gemini CLI, ChatGPT, Codex) are in the **[docs](https://docs.vibexp.io?utm_source=github&utm_medium=readme&utm_campaign=docs_link&utm_content=connect)**.

---

## For developers 🛠️

VibeXP is a monorepo shipped as a single combined Docker image (the backend embeds and serves the frontend SPA + API from one port):

- **`backend/`** Go REST API. Spec-first OpenAPI, PostgreSQL + pgvector, MCP endpoint, pluggable identity-provider auth (Google/GitHub/generic OIDC). Also embeds and serves the built SPA.
- **`frontend/`** Vite + React + TypeScript SPA. Served by the Vite dev server in development; built and embedded into the backend for release.

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

[Website](https://vibexp.io?utm_source=github&utm_medium=readme&utm_campaign=brand_link&utm_content=footer) · [Docs](https://docs.vibexp.io?utm_source=github&utm_medium=readme&utm_campaign=docs_link&utm_content=footer) · [Blog](https://blog.vibexp.io?utm_source=github&utm_medium=readme&utm_campaign=blog_link&utm_content=footer)

</div>
