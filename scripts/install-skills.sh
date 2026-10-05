#!/usr/bin/env bash
# Installs the cpi skills (plugin/skills) for agents that read SKILL.md
# folders: Codex, Cursor, Gemini CLI and others following the Agent Skills
# format. Claude Code users install the plugin instead (docs/plugin.md).
# Without a checkout, 'cpictl skills install' does the same from the binary.
#
#   scripts/install-skills.sh [--agent agents|codex|cursor|gemini] [--user] [REPO]
#
#   --agent  where the agent looks for skills (default: agents)
#              agents, codex  .agents/skills   (Codex; Cursor reads it too)
#              cursor         .cursor/skills
#              gemini         .gemini/skills
#   --user   install for the current user (~/.agents/skills, ...) instead of
#            into the content repository REPO (default: current directory)
#
# Existing cpi-* skills in the target are replaced; other skills are kept.
set -euo pipefail

usage() { sed -n '2,16p' "$0" | sed 's/^# \{0,1\}//' >&2; exit 2; }

agent=agents
user=false
repo=.
while [ $# -gt 0 ]; do
  case "$1" in
    --agent) [ $# -ge 2 ] || usage; agent="$2"; shift 2 ;;
    --user) user=true; shift ;;
    -h|--help) usage ;;
    -*) echo "unknown option $1" >&2; usage ;;
    *) repo="$1"; shift ;;
  esac
done

case "$agent" in
  agents|codex) dir=.agents/skills ;;
  cursor) dir=.cursor/skills ;;
  gemini) dir=.gemini/skills ;;
  *) echo "unknown agent $agent (agents, codex, cursor, gemini)" >&2; exit 2 ;;
esac

if $user; then
  target="$HOME/$dir"
else
  [ -d "$repo" ] || { echo "$repo is not a directory" >&2; exit 2; }
  target="$(cd "$repo" && pwd)/$dir"
fi

src="$(cd "$(dirname "$0")/../plugin/skills" && pwd)"
mkdir -p "$target"
for skill in "$src"/*/; do
  name="$(basename "$skill")"
  rm -rf "${target:?}/$name"
  cp -R "$skill" "$target/$name"
  # allowed-tools names the tools as the Claude Code plugin sees them
  # (mcp__plugin_cpi_cpi__*); other agents name MCP tools differently.
  sed -i.bak '/^allowed-tools:/d' "$target/$name/SKILL.md" && rm -f "$target/$name/SKILL.md.bak"
  echo "installed $name -> $target/$name"
done
