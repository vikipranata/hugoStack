# Backlog: hugoStack

## Epic 1: Foundation & Auth
- **Story 1.1**: Setup Go project structure and basic HTTP server.
- **Story 1.2**: Implement JSON-based config loader.
- **Story 1.3**: Implement Bcrypt session authentication.
- **Story 1.4**: Create Dockerfile and docker-compose.yaml for local dev.

## Epic 2: Content Management (The Editor)
- **Story 2.1**: Implement filesystem scanner to list Hugo posts.
- **Story 2.2**: Build Hybrid Editor UI (Markdown $\leftrightarrow$ WYSIWYG) using vq-design-system.
- **Story 2.3**: Implement API for reading/writing `.md` files.
- **Story 2.4**: Implement asset upload to page bundle `uploads/` folder.

## Epic 3: LLM Translation Workflow
- **Story 3.1**: Integrate LLM API client (OpenAI/Anthropic).
- **Story 3.2**: Implement translation trigger: `index.md` $\rightarrow$ `index.id.md`.
- **Story 3.3**: UI indicators for translation status (Pending/Complete).

## Epic 4: Git & Deployment
- **Story 4.1**: Integrate Go-Git for local commits and pushes.
- **Story 4.2**: Implement workflow: Push to `main` $\rightarrow$ GitHub Pages.
- **Story 4.3**: Configuration for production branch ('hugo').
