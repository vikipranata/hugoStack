# Initiative: hugoStack CMS

## Vision
A lightweight, developer-centric CMS for Hugo static sites that eliminates the friction of manual markdown management and multi-language synchronization while maintaining a pure filesystem-based source of truth.

## Goals
- **Hybrid Editing**: Provide a seamless transition between WYSIWYG and raw Markdown.
- **Multi-language Sync**: Implement a trigger-based LLM translation workflow for `.md` and `.id.md` files.
- **Zero-DB Architecture**: Use the filesystem and JSON for configuration to ensure portability and simplicity.
- **Git-Integrated Workflow**: Local development flowing to GitHub Pages via a dedicated 'hugo' production branch.
- **Fast Deployment**: Deliver as a single Go binary with Docker support.

## Non-Goals
- No complex user permission systems (Simple Bcrypt session auth only).
- No database migration logic (Pure JSON/Filesystem).
- No built-in hosting (Relies on GitHub Pages).
- No plugin system for third-party extensions.

## Success Criteria
- Ability to create, edit, and translate a post across languages via UI.
- Successful build and deploy to GitHub Pages 'hugo' branch.
- Sub-second page loads for the CMS UI using vq-design-system.
