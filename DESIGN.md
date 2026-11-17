---
name: Opera
description: A minimal local coding conversation in the terminal.
typography:
  label:
    fontWeight: 700
spacing:
  content-inset: "2ch"
components:
  composer:
    width: "100%"
  author-label:
    typography: "{typography.label}"
  authored-message:
    width: "100ch"
---

# Design System: Opera

## Overview

**Creative North Star: "Quiet terminal conversation"**

Opera is a quiet terminal conversation: authored text carries the work, a thin ruled composer marks the next action, and session context stays close to the input. Codex and pi are the confirmed visual references; minimalism is the chosen direction.

The terminal owns the font, foreground and background. Hierarchy comes from whitespace, author labels, restrained foreground accents and explicit state words. Completed messages remain in native scrollback; temporary viewers borrow focus and return it to the composer.

**Key Characteristics:**
- Native terminal surface and scrollback
- One column, thin rules and a growing composer
- Keyboard-first controls and textual states
- Motion only during active execution

## Colors

Color is a sparse foreground aid. `internal/ui/theme.go` is authoritative for ANSI palette indexes; these are terminal colors, not CSS colors, so the frontmatter deliberately has no invented hex palette.

- **Primary — muted blue:** accent foreground, Opera author labels, composer rules, shortcuts and selection. ANSI 67 for light terminals; ANSI 110 for dark terminals.
- **Model — soft gold:** the active provider/model in session context. ANSI 137 for light terminals; ANSI 222 for dark terminals.
- **Workspace — green:** the active workspace in session context, using the success token.
- **Mode — secondary accent:** the active mode uses the info token and follows the selected theme.
- **Neutral — secondary gray:** context separators, shortcut descriptions and placeholders. ANSI 240 for light terminals; ANSI 248 for dark terminals. Body text inherits the terminal foreground.
- **Warning — warm amber:** approval state foregrounds. ANSI 137 for light terminals; ANSI 180 for dark terminals. States also have explicit text.

**The Terminal Ownership Rule.** Use inherited terminal background and foreground; do not paint an application background.

**The Text Before Color Rule.** Every meaningful state has a readable text label when color is disabled.

`NO_COLOR` disables styling by presence, including an empty value. Light/dark detection changes the foreground palette without changing the layout.

## Typography

The terminal supplies the monospace font, size, line height and glyph rendering. Do not prescribe a web font or simulate a separate display scale. Bold author labels and temporary-view titles establish hierarchy; normal text carries messages, and secondary text carries context. The frontmatter records the bold label role only.

Authored messages wrap at at most 100 terminal columns, reduced to fit the current width. The `ch` units in portable tokens describe terminal column measures, not browser font specifications.

## Layout

Use one column with a two-column content inset where space permits. The live content width is terminal width minus four columns, clamped to at least one. Composer rules span the available composer width; completed message bodies use the narrower reading measure.

Print the compact Opera banner once per chat session, with provider/model, workspace and command hints. Keep finished authored blocks in terminal scrollback. Bound the streaming preview to the available height and at most eight lines. Chat never enters alternate screen.

The composer grows from one to five rows, further constrained by terminal height. Put provider/model and workspace immediately below it, followed by shortcuts for the current state. At widths below 76 and 48 columns, shorten hints rather than forcing horizontal overflow. Clip long context and constrained views to the available dimensions.

## Elevation & Depth

Everything shares the native terminal plane. No shadows, painted surfaces or overlay depth. Temporary lists and viewers replace the live composer region while open; titles, whitespace and keyboard hints establish their context.

## Shapes

The composer has thin horizontal rules above and below, without side borders or rounded corners. A `›` prompt identifies input. Selection uses a left marker and bold accent text. Avoid decorative enclosing boxes around the conversation.

## Components

- **Session banner:** compact, once per session; identifies Opera and current context without a persistent hero.
- **Composer:** Bubbles textarea with the `›` prompt, one-to-five-row growth and a steady virtual cursor. Enter sends; Alt+Enter or Ctrl+J inserts a newline. Alt+Up/Down recalls input. Bracketed paste inserts without submitting.
- **Authored message:** bold `You` or accented `Opera` label, indented body and blank-line separation. Commit completed content once per checkpoint to scrollback.
- **Context footer:** current provider/model and workspace, then state-specific keyboard hints. Truncate context when needed.
- **Activity status:** words identify running/tool/approval/failure states. During execution only, a spinner and Harmonica spring drive an indeterminate trace at 20 frames per second. `--no-motion` removes both; idle chat has no activity timer or blinking cursor. Never add simulated typing delays.
- **Model selector and detail viewer:** Bubbles list/viewport, keyboard focus, bounded height of at most 12 rows. Arrows navigate or scroll; Esc restores the composer. Review approval arguments and changes before closing details and using `/approve` or `/deny`.
- **Error summary:** short recovery text in the conversation; `/details` retains full diagnostics and available run/state/checkpoint metadata. Prefer useful provider messages over transport noise.
- **Accessible chat:** append-only canonical input without raw-mode cursor movement or periodic redraw. Use `--accessible --no-motion`; preserve the documented command alternatives in [accessibility.md](docs/accessibility.md).

Bubble Tea v2 owns presentation updates, Bubbles provides interaction primitives, Lip Gloss renders terminal styling, and Harmonica supplies active spring motion. The visual source is `internal/ui/{layout,theme,chat,errors}.go`.

## Do's and Don'ts

### Do:
- **Do** preserve the terminal background and native scrollback.
- **Do** label authors and states in words, including without color.
- **Do** keep full diagnostics and run metadata available through `/details`.
- **Do** return focus to the composer when a temporary viewer closes.

### Don't:
- **Don't** add side borders, filled cards or a permanent dashboard around chat.
- **Don't** animate idle chat or imply a completion percentage.
- **Don't** require color, animation or raw-mode interaction for the accessible path.
