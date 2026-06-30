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

- **`FRONTEND_BASE_URL`** — set this to your real public URL (e.g. `https://vibexp.example.com`) **first**. It is the single origin serving both the SPA and the API (same-origin: no separate frontend URL, no CORS), and pointing it away from `localhost` is what **turns off the local-eval dev-login bypass** and the auto-enabled local MCP server. Leave it at `localhost` while exposing the app and anyone can sign in as any user.
- **`ENCRYPTION_KEY`** — required, exactly 32 bytes. Generate one: `openssl rand -base64 24 | cut -c1-32`
- **`DB_PASSWORD`** — change it from the default (and keep `POSTGRES_PASSWORD` in sync).
- **`SESSION_ENCRYPTION_KEY`** (`openssl rand -hex 32`) and an **identity provider**: set `AUTH_PROVIDER` to `google`, `github`, or `oidc` with the matching `*_CLIENT_ID` / `*_CLIENT_SECRET` (and `*_REDIRECT_URI` if it differs from `<FRONTEND_BASE_URL>/api/v1/auth/callback`). For several providers at once, mount a `config.yaml` with `auth.providers: [...]`.
- **MCP in production (optional)** — set `OAUTH_AS_ISSUER_URL` (your public HTTPS URL) **and** `MCP_RESOURCE_URI` (`<url>/mcp/v1/common`) to enable the embedded OAuth server that issues MCP tokens.
- **Branding / analytics (optional)** rebrand the SPA at deploy time with `VITE_*` env vars (served via `/config.js`, no rebuild) — see the `app` service comments in `docker-compose.yml`.
- **Semantic search & file attachments** are optional, opt-in services. See the comments in `docker-compose.yml` and the [docs](https://docs.vibexp.io?utm_source=github&utm_medium=readme&utm_campaign=docs_link&utm_content=self_host).

Data persists in the `pgdata` volume.

</details>

---

## Deploy anywhere 🌍

VibeXP is one self-contained binary in one image — **no hosting-platform assumptions**. It reads a single `config.yaml`; the published image bakes a default ([`backend/config.docker.yaml`](backend/config.docker.yaml)) whose every value is a `${VAR:-default}` reference, so **environment variables alone configure a container**. To control every setting instead, mount your own file over `/app/config.yaml` — start from [`backend/config.example.yaml`](backend/config.example.yaml).

**Every deployment needs:** a reachable PostgreSQL with [pgvector](https://github.com/pgvector/pgvector), plus `DB_PASSWORD` and a 32-byte `ENCRYPTION_KEY`. For internet-facing use, also set `FRONTEND_BASE_URL` (your public origin), `SESSION_ENCRYPTION_KEY`, and an identity provider (see the section above).

<details>
<summary><strong>🐳 <code>docker run</code> (single container)</strong></summary>

```sh
docker run -p 8080:8080 \
  -e DB_HOST=your-db-host -e DB_PASSWORD=secret \
  -e ENCRYPTION_KEY="$(openssl rand -base64 24 | cut -c1-32)" \
  -e FRONTEND_BASE_URL=https://vibexp.example.com \
  ghcr.io/vibexp/vibexp:latest
```

To replace the baked config entirely, mount your own and pass only secrets via env:

```sh
docker run -p 8080:8080 -v "$PWD/config.yaml:/app/config.yaml:ro" \
  -e DB_PASSWORD=secret -e ENCRYPTION_KEY=... ghcr.io/vibexp/vibexp:latest
```

</details>

<details>
<summary><strong>🧩 Docker Compose</strong></summary>

The bundled [`docker-compose.yml`](docker-compose.yml) runs the image alongside Postgres. Set your secrets in the `app` service `environment:` (or supply them via a `.env` file that Compose substitutes), then:

```sh
docker compose up -d
```

</details>

<details>
<summary><strong>☸️ Kubernetes</strong></summary>

Put secrets in a `Secret` exposed as env (consumed by the baked config's `${VAR}` references), and — when you want full control — mount a `config.yaml` from a `ConfigMap` over `/app/config.yaml`:

```yaml
env:
  - name: DB_HOST
    value: postgres
  - name: FRONTEND_BASE_URL
    value: https://vibexp.example.com
  - name: DB_PASSWORD
    valueFrom: { secretKeyRef: { name: vibexp, key: db-password } }
  - name: ENCRYPTION_KEY
    valueFrom: { secretKeyRef: { name: vibexp, key: encryption-key } }
# optional full-config override:
volumeMounts:
  - { name: config, mountPath: /app/config.yaml, subPath: config.yaml }
volumes:
  - { name: config, configMap: { name: vibexp-config } }
```

</details>

<details>
<summary><strong>📦 Bare binary</strong></summary>

`make build-combined` produces a single self-contained `backend/bin/vibexp` (frontend embedded). It loads `./config.yaml` by default; override with `--config /path/to/config.yaml` or `VIBEXP_CONFIG_FILE=...`. Copy [`backend/config.example.yaml`](backend/config.example.yaml) to `config.yaml` and edit it.

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
