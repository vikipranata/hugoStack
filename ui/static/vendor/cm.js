// CodeMirror 6 entry: one esbuild-bundled copy of the whole CM6 + markdown
// graph (single module instance — no esm.sh dual-instance issues), re-exported
// for app/editor.js and exposed on window.__CM__ for debugging.
import {
  EditorView,
  basicSetup,
  keymap,
  markdown,
  markdownLanguage,
  HighlightStyle,
  syntaxHighlighting,
  tags,
} from "./cm/cm.bundle.js";

export {
  EditorView,
  basicSetup,
  keymap,
  markdown,
  markdownLanguage,
  HighlightStyle,
  syntaxHighlighting,
  tags,
};

window.__CM__ = { EditorView, basicSetup, keymap, markdown, markdownLanguage, HighlightStyle, syntaxHighlighting, tags };
