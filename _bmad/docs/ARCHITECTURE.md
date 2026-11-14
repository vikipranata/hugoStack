# Architecture: hugoStack

## Paradigm
**Filesystem-as-Database (FaaDB)**. The application is a thin wrapper around the Hugo directory structure. State is persisted directly to `.md` and `.json` files.

## Technical Stack
- **Backend**: Go (Golang) - chosen for single-binary distribution and performance.
- **Frontend**: Vanilla JS + vq-design-system (CSS) - zero-build runtime for maximum simplicity.
- **Auth**: Bcrypt-hashed passwords stored in `config.json`.
- **Storage**: Local filesystem $\rightarrow$ Git $\rightarrow$ GitHub Pages.

## Data Flow
1. **Content Edit**: UI $\rightarrow$ Go API $\rightarrow$ `content/posts/<slug>/index.md`.
2. **Translation**: UI Trigger $\rightarrow$ Go API $\rightarrow$ LLM API $\rightarrow$ `content/posts/<slug>/index.id.md`.
3. **Deployment**: CMS $\rightarrow$ Git Commit $\rightarrow$ Push to `main` $\rightarrow$ GitHub Action $\rightarrow$ Deploy to `hugo` branch.

## Folder Structure
```text
/
├── bin/                # Compiled Go binary
├── config.json         # App settings & Auth
├── content/            # Hugo content root
│   └── posts/
│       └── <slug>/
│           ├── index.md
│           ├── index.id.md
│           └── uploads/  # Page bundle assets
├── static/             # Hugo static assets
├── ui/                 # CMS Frontend (HTML/JS/CSS)
└── .env                # Environment variables (LLM keys, etc.)
```

## API Design (REST)
- `POST /api/auth/login` - Session creation.
- `GET /api/posts` - List all posts.
- `GET/PUT /api/posts/:slug` - Get/Update post content.
- `POST /api/posts/:slug/translate` - Trigger LLM translation to ID.
- `POST /api/deploy` - Trigger Git push.

## Invariants
- **AD-1**: All content must reside in Hugo-compatible page bundles.
- **AD-2**: No external database allowed; JSON is the only configuration format.
- **AD-3**: Translation always creates/updates a sibling file with `.id.md` suffix.
