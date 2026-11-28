# Keyboard and accessible terminal behavior

Interactive chat fills the alternate screen with an independent message viewport
and a fixed composer. Exit restores the prior screen. Finished blocks are retained
once; streaming follows the bottom only when already viewing the latest output. Resize, Unicode,
multiline paste and history are covered by deterministic/PTY fixtures.

The chat follows a minimal, single-column layout inspired by Codex and pi. A
compact banner identifies the session; the composer uses thin horizontal rules
and grows from one to five lines as you type or paste. The footer shows the
current provider/model and workspace, with shortcuts for the active state.
Temporary viewers take keyboard focus; Esc returns it to the composer.

| Action | Keyboard / command |
| --- | --- |
| Send | Enter |
| New line | Alt+Enter or Ctrl+J |
| Recall input | Alt+Up / Alt+Down |
| Scroll message history | PageUp / PageDown or mouse wheel over history |
| Latest messages | Ctrl+End |
| Select a model | `/model`, arrows, Enter; Esc closes |
| Set activity display | `/activity`, arrows, Enter; Esc closes |
| Inspect tool details | Ctrl+O |
| Inspect/help | `/help`, `/details`, `/context`, `/usage`, `/tasks`, `/agents` |
| Error diagnostics / run metadata | `/details` after a failed or completed run |
| Scroll/close viewer | Arrows/Page keys; Esc |
| Approve/deny | Close details with Esc, then `/approve` or `/deny` |
| Follow-up | Type normally while running |
| Explicit steering | `/steer <message>` |
| Cancel active / exit idle | Ctrl+C |
| Exit empty editor | Ctrl+D |

`NO_COLOR` is honored by its presence, including an empty value. Status meaning
uses words as well as color. The blue-gray status accents use ANSI 67 on light terminals
and 109 on dark terminals. Failures use muted rose (ANSI 131/167) and approvals
muted ochre (ANSI 137/179), always with text labels; no red/green-only meaning.
The terminal owns the background; Opera does not paint a separate app surface.
The composer uses a steady cursor so idle chat does not redraw. `--no-motion`
disables the activity spinner and spring animation.
Activity ticks run only while an execution is active; no-motion and idle states
schedule no activity timers. The activity trace is indeterminate, never a
completion percentage. There is no simulated typewriter delay. Errors commit a
short recovery message once per checkpoint. Full provider diagnostics and run
metadata remain available through `/details`; approvals show formatted arguments
and changes in a scrollable viewer.

For a screen reader use `opera chat --accessible --no-motion`. This path stays
append-only in the terminal's canonical input mode, without raw-mode cursor moves,
animations or periodic redraw. Input remains responsive during execution for
follow-ups, `/steer`, `/usage`, tasks/agents and cancel. It has the same durable
approvals and backend. Use `/multiline`, enter lines, then `/send`; `/cancel-input`
discards the buffer. `/model <provider> <model>` replaces the visual selector.
`/activity full|auto|hidden` selects the activity display, and `/activity`
reports the current choice. Auto prints comments and responses as they complete.
Hidden prints confirmed responses. A change during a run applies to the next
execution.
Use `/quit` or EOF to exit. Active Ctrl+C cancels and keeps the session open;
idle Ctrl+C exits. Non-TTY approval returns 3 without waiting for input.

Native tests exercise Unix PTY state restoration and Windows ConPTY startup,
resize, output, keyboard exit and console-mode restoration. Windows tests use
[Microsoft's documented ConPTY lifecycle](https://learn.microsoft.com/en-us/windows/console/creating-a-pseudoconsole-session).
A colorless or screen-reader-friendly path does not claim compatibility with every
terminal emulator or assistive product. See the platform verification state in
[F5 validation](f5-validation.md).

## Provider sign-in

`/login` opens the available-provider submenu (currently OpenAI). In normal chat,
arrows navigate, Enter confirms, `/` filters, and Esc returns. Esc or Ctrl+C during
OAuth cancels sign-in and preserves the session. After sign-in, the account model
selector opens automatically.

Append-only chat prints numbered provider and model menus. Enter a number or the
provider name; `/cancel` returns or cancels the current sign-in. Browser launch
failure leaves the full authorization URL in terminal output for manual use.

When the system keyring is unavailable, sign-in displays a numbered credential
storage menu. Choose retry, an explicitly unencrypted private local file, or
cancel. The active OAuth attempt waits for this choice; tokens are not printed
and completed authorization does not need to be repeated to retry storage.
