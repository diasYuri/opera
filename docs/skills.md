# Agent Skills

Opera supports the Agent Skills `SKILL.md` format. A skill is a directory with
YAML frontmatter and Markdown instructions, with optional references, scripts,
assets, and other bundled files.

## Locations and precedence

Opera discovers immediate child directories in:

- `<workspace>/.agents/skills/<skill-name>/SKILL.md`
- `~/.agents/skills/<skill-name>/SKILL.md`

Workspace skills take precedence over user skills with the same name. A
workspace skill with invalid metadata still shadows a same-name user skill so
that a broken project copy cannot silently change behavior to another skill.
Opera does not search `.claude/skills`, `.codex/skills`, or parent workspaces.

## Format

`SKILL.md` must begin with YAML frontmatter delimited by `---`. Required fields
are `name` and `description`; the name must match the directory, use lowercase
letters, digits, and single hyphens, and be at most 64 characters. Description
is limited to 1024 characters. Optional supported fields are `license`,
`compatibility`, `metadata` (string values), and `allowed-tools`.
Unknown frontmatter fields are retained as inert extension metadata.

```markdown
---
name: release-notes
description: Summarize changes into release notes. Use when preparing a release.
---

Read the changes, group them by user impact, and include compatibility notes.
```

## Using skills

`opera skills list` shows discovered skills, `opera skills show <name>` displays
one skill, and `opera skills check` reports invalid skills. In chat, use
`/skills` to inspect the catalog, `/skills <name>` to inspect instructions, or
type `$` to autocomplete a name. The model sees names and descriptions and can
load the full instructions with `skill_load`; it can read a bundled file with
`skill_resource` when needed. Skills are rediscovered for each execution, and
editing the catalog during a run takes effect on the next run.

Resources are limited to regular files inside the skill directory. Symlinks,
absolute paths, traversal, oversized files, and binary files other than common
image formats are rejected. `SKILL.md` is capped at 128 KiB, each resource at
1 MiB, the package at 8 MiB and 1024 files, and discovery at 256 skills.
The total discovered package bytes are capped at 16 MiB per workspace catalog.
`scripts/` files can be read as resources; Opera
does not execute scripts automatically. The `allowed-tools` metadata is exposed
as metadata only and never bypasses Opera's operation policy or approvals.

Skill text is untrusted guidance. It cannot override the user, workspace
instructions, or tool policy. Changing a skill's contents after a checkpoint
was created prevents that checkpoint from resuming until the original skill
catalog is restored.
