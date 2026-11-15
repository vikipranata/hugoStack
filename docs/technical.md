# hugoStack — Technical Reference

Authoritative reference for the implemented system. The planning docs in
`_bmad/docs/` (initiative, architecture intent, backlog) predate the final API
shapes; where they disagree, this file and the code win.

## 1. Architecture

**Filesystem-as-Database (FaaDB).** hugoStack is a thin HTTP wrapper around a
Hugo repository's `content/posts/` tree. There is no database:

- **Content** = `.md` page-bundle files, edited in place.
- **App config** = one `config.json` (JSON, human-editable).
- **History/deploy** = the Hugo repo's own git.

```
Browser ── HTTP/JSON ──> Go server (single binary, stdlib net/http)
                            │
                            ├── auth.Middleware (session cookie)
                            ├── HTML routes  -> ui/templates (html/template, loaded at start)
                            ├── /static/     -> ui/static (served from disk, dynamic)
                            ├── content I/O  -> <hugo_path>/content/posts/<slug>/
                            ├── git ops      -> system git (os/exec, no libgit2)
                            └── LLM          -> any OpenAI-compatible /chat/completions
```

Stack:

| Layer | Choice | Why |
|---|---|---|
| Backend | Go 1.27, stdlib only (+ `golang.org/x/crypto/bcrypt`) | single static binary, trivial deploy |
| Frontend | Vanilla JS, no build step, vq-design-system CSS tokens | zero toolchain |
| WYSIWYG | TinyMCE 7.6.0, vendored | offline, no CDN |
| Markdown | CodeMirror 6 (single esbuild bundle) + marked preview + turndown bridge | offline, lossless round-trip |
| Auth | bcrypt + HttpOnly session cookie | single-user, no IdP |

## 2. Data model

A post is a Hugo **page bundle** directory:

```
content/posts/<slug>/
├── index.md          # English (canonical)
├── index.id.md       # Indonesian (optional)
└── uploads/          # page-bundle assets (uploaded images)
    └── screenshot-1.png
```

Language ↔ file mapping (fixed, in `main.go`):

| lang | file |
|---|---|
| `en` | `index.md` |
| `id` | `index.id.md` |

Frontmatter is YAML between `---` fences. hugoStack treats it opaquely
except for three fields:

- **`title`**, **`tags`**, **`description`** — the only fields the editor can
  rewrite (`browser.Rewrite`).
- Everything else — `date`, `modified`, `IDtags`, custom fields, comments, the
  file's CRLF/LF line endings — is **preserved byte-for-byte**. `Rewrite`
  operates line-by-line on the original bytes; it never re-serializes the whole
  frontmatter block.

Writes are **atomic**: `os.CreateTemp` in the same directory + `os.Rename`, so
a crash mid-write never leaves a half-written post.

Markdown body is the **single source of truth**. In WYSIWYG mode TinyMCE holds
HTML; on save (or mode switch) it is converted back to markdown with
turndown (+ GFM plugin) and the markdown is what gets persisted. Switching
WYSIWYG ⇄ Markdown is lossless because both directions operate on the same
markdown string held in editor state.

## 3. HTTP API

Public (no auth):

| Method | Path | Purpose |
|---|---|---|
| GET/POST | `/login` | Login form / credential check → sets `session_id` cookie |
| ANY | `/static/<file>` | Static assets from `ui/static` (templates, CSS, editor, vendored libs) |

Protected (session required; 303 → `/login` otherwise):

| Method | Path | Purpose |
|---|---|---|
| GET | `/` | Post list + git/deploy panel (HTML) |
| GET/POST | `/settings` | Settings form (LLM + git fields) |
| GET | `/edit/<slug>?lang=en\|id` | Editor page (HTML). Missing target file → empty "new draft" editor |
| POST | `/edit/<slug>?lang=…` | Form save (title/description/tags/body) → 303 back |
| GET | `/api/posts` | `[{slug,title,has_en,has_id}]` for every post dir |
| GET | `/api/posts/<slug>?lang=en\|id` | `{slug,file,lang,title,tags,description,body}` |
| PUT | `/api/posts/<slug>?lang=en\|id` | Save `{title,tags,description,body}`. `tags` may be an array **or** a comma string. Missing file is a valid target — seeded from the other language (preserves `date`/`modified`), else a bare scaffold |
| POST | `/upload/<slug>` | Image upload. multipart (`file` field) **or** JSON `{name, data: <base64>}` (paste/drop). Whitelist: png/jpg/jpeg/webp/gif. Returns `{path, snippet}` where `snippet` is `![alt](uploads/<name>)` |
| GET | `/post/<slug>/uploads/<name>` | Serve a page-bundle asset (editor preview images) |
| POST | `/api/translate/<slug>?lang=en\|id` | LLM-translate to the *other* language → `{slug,lang,content,title,tags,description,body}`. **Nothing is written** — result loads as an unsaved draft |
| GET | `/api/git/status` | `{ok,branch,dirty,ahead,behind,upstream}` |
| POST | `/api/git/commit` | `{message}` → `git add -A && git commit`. `{ok,output}` |
| POST | `/api/git/push?ref=<ref>` | `git push origin HEAD:<ref>`. `{ok,output}` |

JSON error style: plain-text body + status code (`400` bad input, `404` not
found, `405` wrong method, `502` LLM failure, `503` LLM not configured).

### Validation / traversal guards

- `slug` must be a single path segment (no `/`, `\`, `..`) — `postDir`/`postFile`
  reject otherwise → `400`.
- `lang` must be exactly `en` or `id` → `400`.
- Upload filenames are sanitized to `[A-Za-z0-9._-]` (`safeFileName`);
  collisions get `-1`, `-2`, … suffixes; extension whitelist enforced.
- `/post/` asset serving: exactly 3 path parts, middle must be `uploads`,
  filename must be a plain name (no `/`, `\`, `.`, `..`).
- Git push refs pass `git.ValidRef` (plain refname, no `+`/`^`/space/magic).

## 4. Frontmatter rewrite (the tricky part)

`internal/browser/frontmatter.go`:

- `Split(content)` → `(frontmatter, body, ok)`. Deliberately does **not** use a
  YAML library: it finds the first `---` line and the next `---` line and
  returns the raw text between them. This is what makes non-target fields
  survive untouched.
- `Rewrite(content, fm{Title,Tags,Description}, newBody)` → new file content.
  Walks the original frontmatter **line by line**, replacing only the
  `title:`, `tags:`, `description:` lines (re-serializing just those), and
  splices in the new body. Tags serialize to YAML flow style
  (`tags: [a, b, c]`). `date`/`modified`/anything else passes through verbatim,
  including the file's original line-ending style.
- `TitleOf`, `TagsOf`, `FMField`, `Unquote` are the read-side helpers used by
  the list view and API.

## 5. Auth & sessions

- `POST /login` checks `user == cfg.AdminUser` and
  `bcrypt.CompareHashAndPassword(cfg.AdminPass, pass)`.
- On success: `session_id` cookie, `HttpOnly`, `Path=/`. The token is
  `auth.SessionToken()` — an HMAC of a per-process secret
  (`HUGOSTACK_SESSION_SECRET`, or a random secret if unset). Without a stable
  secret, restarts invalidate all sessions (by design, documented in
  `.env.template`).
- `auth.Middleware` wraps every protected route; unauthenticated → `303 /login`.
- Password change: `go run ./cmd/setpass '<new>'` prints a bcrypt hash to paste
  into `config.json`; Docker first-run bootstraps from `HUGOSTACK_ADMIN_PASS`.

## 6. Git & deploy

`internal/git/git.go` shells out to system git (no libgit2, no new deps):

- Every exec sets `GIT_AUTHOR_NAME/EMAIL` + `GIT_COMMITTER_NAME/EMAIL` (from
  config, defaults `hugoStack` / `hugostack@localhost`) and
  `GIT_TERMINAL_PROMPT=0` so a missing credential **fails fast** instead of
  hanging the API.
- `Status` → branch, dirty count, ahead/behind vs upstream (`origin/<branch>`
  fallback), has-upstream.
- `Commit` → `git add -A` + `git commit -m`. "nothing to commit" is a soft OK.
- `Push` → `git push origin HEAD:<destRef>`.
- Every result returns git's combined stdout+stderr **verbatim** so SSH/auth
  failures are visible in the deploy panel.

The deploy panel (index page) polls `GET /api/git/status` and offers:

- **Commit** — commit pending changes with a message.
- **Sync to Pages** — commit if dirty, push `branch_pages` (default `main`) → GitHub Pages.
- **Release** — commit if dirty, push `branch_release` (default `hugo`) → the is-a.dev pipeline.

Docker note: bind-mounted repos are host-owned while the container runs as
root, so the image sets `safe.directory=*` in **`/etc/gitconfig`** (system
config). Env-based `safe.directory` does not reach git's upload-pack children
for local-path remotes, which is why it must be system-level. See `Dockerfile`.

## 7. LLM translation

`internal/llm/llm.go` — minimal OpenAI-compatible client:

- `POST {base}/chat/completions`, Bearer auth, 120 s timeout, 1 MiB response cap.
- Non-2xx → structured error including the response body.
- No retry/streaming (single-doc translation; add when bulk is needed).

`POST /api/translate/<slug>?lang=X` (in `main.go`):

- Requires all three of `llm_base_url`/`llm_api_key`/`llm_model`, else `503`.
- System prompt: strict translator — translate only `title`/`description`,
  keep `tags`/`date`/`modified` verbatim, preserve **all** Markdown syntax
  (code blocks, tables, link URLs, image paths, comments, fences).
- Response must contain frontmatter or the endpoint returns `502` (a doc without
  frontmatter can't survive the save flow, and a silent fallback would lose the
  translated title/description).
- An optional outer code fence the model adds is stripped (`stripOuterFence`).
- **The endpoint never writes to disk.** The editor stashes the result and loads
  it into the target-language editor as an *unsaved draft* with a review banner;
  the normal Save flow (which seeds a missing file from the other language) is
  the only write path.

## 8. Editor (frontend)

`ui/static/app/editor.js` (no build step, loads vendored libs):

- **Two modes, one source of truth.**
  - WYSIWYG: TinyMCE 7 initialized on the post body.
  - Markdown: CodeMirror 6 with `@codemirror/lang-markdown` highlighting, plus a
    live preview pane (marked) with a draggable divider (pointer-capture,
    clamped 20–80%, arrow-key nudge) and proportional scroll sync.
  - Switching converts via turndown (WYSIWYG→MD) / marked (MD→WYSIWYG); the
    markdown string in `state.md` is what Save persists, verbatim.
- **Image upload in both modes** → `POST /upload/<slug>` (button, paste, and
  drag-drop). Returns a relative `uploads/<name>` snippet inserted at the
  cursor. For display, `uploads/` refs are rewritten to
  `/post/<slug>/uploads/<name>` so preview images load, and rewritten back on
  save.
- **Ctrl/Cmd+S** saves; a `beforeunload` guard warns on unsaved changes.
- Frontmatter lives in the right sidebar (title, description, tags, read-only
  file path, language chips) on wide viewports; stacks below on narrow ones.

### Vendored libraries (offline, no runtime CDN)

`ui/static/vendor/`:

- `tinymce/` — TinyMCE 7.6.0: core, silver theme, oxide + content skins,
  `models/dom`, **`icons/default/icons.min.js`** (TinyMCE auto-loads this from
  its base URL — omitting it 404s and aborts init), 9 plugins (image, table,
  link, autolink, fullscreen, code, codesample, lists, media).
- `cm/` — CodeMirror 6 as **one** esbuild bundle (`cm.bundle.js`) + a thin
  `cm.js` entry re-exporting `EditorView`, `basicSetup`, `markdown`,
  `markdownLanguage`, `HighlightStyle`, `syntaxHighlighting`, `tags`.
  Single bundle = single module graph = no dual-instance issues.
- `marked/` — marked (UMD) for the live preview.
- `turndown/` — turndown + turndown-plugin-gfm for WYSIWYG→Markdown.

> **CM6 API note (bit us once):** in current `@codemirror/view`,
> `EditorView.updateListener` is a **Facet**, not a callable static. Use
> `EditorView.updateListener.of(fn)`, not `EditorView.updateListener(fn)` —
> the latter throws `is not a function` and silently kills the Markdown pane.
> See `editor.js` (`initCM`).

## 9. Configuration

`config.json` (path: `HUGOSTACK_CONFIG`, default CWD / `/app/config.json` in
Docker). Loaded once at start, cached in memory; the Settings page mutates the
in-memory copy and persists via `config.Save`.

| Key | Notes |
|---|---|
| `hugo_path` | Hugo repo root. Restart to change. |
| `port` | Listen port. Restart to change. |
| `admin_user` / `admin_pass` | `admin_pass` is a bcrypt hash. |
| `llm_base_url` / `llm_api_key` / `llm_model` | All three required for Translate. |
| `branch_pages` / `branch_release` | Defaults `main` / `hugo` (filled by `applyDefaults`). |
| `git_user` / `git_email` | Commit identity. |

**Env precedence** (`config.applyEnv`): `HUGOSTACK_PORT`, `HUGOSTACK_HUGO_PATH`,
`HUGOSTACK_GIT_USER`, `HUGOSTACK_GIT_EMAIL` override the config file at load.
In Docker the config file is ephemeral (bootstrap regenerates it from env on
first start), so env is the durable source there.

## 10. Security model

Single-user, LAN/trusted-network oriented. Threats it handles:

- Path traversal in slug/lang/upload/asset routes (validated, §3).
- Path injection in upload filenames (sanitized, whitelisted ext).
- Git credential hangs (`GIT_TERMINAL_PROMPT=0`, `ValidRef`).
- LLM prompt-injection → disk (translation never auto-saves; human reviews).

It deliberately does **not** do: CSRF tokens, rate limiting, TLS termination,
per-user accounts, or content sanitization beyond the upload extension
whitelist. Behind a reverse proxy, add TLS and, if exposed beyond a trusted
network, consider a stronger auth layer.

## 11. Build, run, test

```bash
go build ./...        # build all
go vet ./...
go test ./...         # unit tests (browser, git, llm, server)
go run ./cmd/server   # dev server (port from config.json / HUGOSTACK_PORT)
go run ./cmd/setpass 'pw'   # print a bcrypt hash for config.json
```

Docker:

```bash
cp .env.template .env && docker compose up -d --build
```

## 12. Troubleshooting

| Symptom | Cause / fix |
|---|---|
| Editor pane blank, no TinyMCE | `icons/default/icons.min.js` missing from vendor → 404 aborts init. Check the vendored icons dir. |
| Markdown pane never appears | `EditorView.updateListener` called as a function. Must be `.of(fn)`. |
| `bind: address already in use` | An orphan `go run` child holds the port (killing the `go run` parent orphans the compiled child). Find and kill the child, or use a free port. |
| Sync/Release fails with an SSH error | No git credentials in the container. Mount `~/.ssh` read-only or switch the remote to a token URL (see README). |
| Logins drop on restart | `HUGOSTACK_SESSION_SECRET` unset → random per-start secret. Set it. |
| `git: detected dubious ownership` | Host-owned bind mount + root container. The image sets `safe.directory=*` in `/etc/gitconfig`; if you run outside Docker, add it to your system git config. |
| Config change not taking effect | `port`/`hugo_path` need a restart; Settings-page LLM/git edits apply immediately; direct `config.json` edits apply on next start. |

## 13. Project layout

```
cmd/server/        main.go (routes+handlers), templates.go (template load)
cmd/setpass/       bcrypt hash helper
internal/auth/     bcrypt + session token + middleware
internal/browser/  post scanner (browser.go) + frontmatter split/rewrite (frontmatter.go)
internal/config/   JSON config load/save + env precedence
internal/git/      system-git wrapper (status/commit/push/ref-validation)
internal/llm/      OpenAI-compatible chat client
ui/templates/      shell, index, editor, settings, login (html/template)
ui/static/app/     editor.js (mode switching, upload, save, translate)
ui/static/         style.css (vq-design-system tokens) + vendor/ (offline libs)
docs/              this reference + screenshots/
_bmad/             planning docs (initiative / architecture intent / backlog)
```
