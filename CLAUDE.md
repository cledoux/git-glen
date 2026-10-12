@AGENTS.md

## Claude Code Notes

- `just install` (or `just install-claude`) registers the plugin as
  `glen@skills-dir` by linking `~/.claude/skills/glen/`. Start a new session to
  pick up manifest or hook changes; skill text edits are live through the
  symlinks.
- Validate the manifest with `claude plugin validate .` after editing
  `.claude-plugin/plugin.json` or `hooks/hooks.json`.
