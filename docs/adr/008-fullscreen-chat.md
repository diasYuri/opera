# ADR 008 — Fullscreen interactive chat regions

Status: accepted. Supersedes the inline/scrollback layout in ADR 005 at the
explicit request for fullscreen interactive chat.

Interactive Bubble Tea chat owns the alternate screen. Bubble Tea restores the
previous screen and terminal modes on exit. The accessible canonical-input path
and headless text/JSONL output remain append-only and do not enter alternate screen.

The shell owns terminal dimensions and settles region sizes after transitions.
The existing Bubbles conversation viewport owns history navigation; the existing
textarea owns editing, cursor and internal scrolling. The composer and all its
auxiliary blocks reserve rows first; the viewport receives the remaining height.
Small terminals reduce auxiliary rows and history before editable input. Temporary
viewers borrow focus and Esc restores editing. No new layout framework is needed.

Page keys navigate history independently of editing. Mouse wheel events target
history only inside its rows. Incoming messages follow the bottom only when the
reader was already there. Height changes retain reading offsets and drafts;
width changes reflow message content. Rendering uses Bubble Tea's differential
renderer without explicit screen clears; unchanged viewport content is retained.

Unix PTY and Windows ConPTY fixtures assert alternate-screen entry/exit and mode
restoration. Deterministic tests cover dimensions, composer growth/internal scroll,
Unicode, streaming while reading older messages, mouse targeting and focus.
