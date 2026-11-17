# ADR 005 — Scrollback and terminal ownership

Status: superseded for interactive layout by [ADR 008](008-fullscreen-chat.md).
The append-only accessible and headless contracts remain in force.

The primary experience never enters alternate screen. Completed blocks commit
to scrollback exactly once by identity/sequence. Streaming response, editor and
status occupy a bounded dynamic region. Resize and temporary selectors/forms/diff
viewers must not reprint completed history. The UI loop alone mutates its state;
effects return Bubble Tea messages.

Enter sends; Alt+Enter inserts a newline, with an explicit documented fallback
for terminals that cannot distinguish it. Ctrl+C cancels active work while keeping
chat open; at idle it exits. Ctrl+D with an empty editor exits. New messages during
execution become follow-ups; steering requires an explicit action.

Use adaptive light/dark themes, blue accent and textual status. Respect NO_COLOR.
Harmonica is limited to short progress/transitions; no artificial typing effect.
No-motion and accessible modes provide equivalent information without continuous
redraw. F0 plain output already uses no color, animation or terminal manipulation.
PTY tests must cover long streams, paste, Unicode, resize and terminal restoration
after errors. Headless paths use text/JSONL directly, without Huh or a TUI loop.
