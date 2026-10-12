@AGENTS.md

## Claude Code Notes

- `just install` (or `just install-claude`) registers the plugin as
  `glen@skills-dir` by symlinking `~/.claude/skills/glen -> <repo>/plugin`.
  Start a new session to pick up manifest or hook changes
  (`plugin/.claude-plugin/plugin.json`, `plugin/hooks/hooks.json`); skill and
  rule edits (`plugin/skills/<skill-name>/SKILL.md`,
  `plugin/rules/pure-bare-worktrees.md`) are live through the symlink.
- Validate the manifest with `claude plugin validate plugin` (or test in a
  session with `claude --plugin-dir ./plugin`) after editing
  `plugin/.claude-plugin/plugin.json` or `plugin/hooks/hooks.json`.
