# Codex, Cursor, Gemini CLI and other agents

The Claude Code plugin ([plugin.md](plugin.md)) has three parts. Two of them work in any agent:

| Part | Claude Code | Other agents |
|------|-------------|--------------|
| MCP server `cpictl mcp` (the tools) | part of the plugin | any MCP client: configure it as below |
| Skills `cpi-discover`, `cpi-plan`, `cpi-build`, `cpi-test`, `cpi-review` | part of the plugin | `SKILL.md` folders: copy them with `scripts/install-skills.sh` |
| Read-only reviewer agent `cpi-reviewer` | part of the plugin | not installed; `cpi-review` then does the review itself with read tools only |

The tenant conventions (`.cpi/` in the content repository) are the same for every agent, so a
team can mix agents in one repository.

## 1. cpictl and profiles

Install `cpictl` and create one profile per tenant, as for the CLI
([README](../README.md#configuration-and-profiles)):

```bash
go install github.com/cpars-innovation/cpicli/cmd/cpictl@latest   # or a release binary
mkdir -p ~/.cpictl && cp docs/examples/profiles/dev.yaml ~/.cpictl/dev.yaml   # then edit it
chmod 600 ~/.cpictl/dev.yaml
cpictl --profile dev packages list        # check the connection
```

The MCP configurations below name only the profile, so they contain no credentials and can be
committed.

## 2. MCP server

Each server keeps its profile while it runs. Use `--mode` or `--read-only` to limit what the
agent can do ([mcp.md](mcp.md#modes)). `--root` confines local paths of tool calls to the content
repository: use the repository path, or `.` when the agent starts servers in the repository.

### Codex

`.codex/config.toml` in the content repository (Codex reads it for trusted projects) or
`~/.codex/config.toml` ([example](examples/agents/codex-config.toml)):

```toml
[mcp_servers.cpi-dev]
command = "cpictl"
args = ["mcp", "--root", ".", "--profile", "dev", "--mode", "develop"]
tool_timeout_sec = 600

[mcp_servers.cpi-qa]
command = "cpictl"
args = ["mcp", "--root", ".", "--profile", "qa", "--read-only"]
```

or from the shell: `codex mcp add cpi-dev -- cpictl mcp --root . --profile dev --mode develop`.
Codex stops a tool call after 60 seconds by default; `deploy` and `send_test_message` wait
for the tenant longer than that, hence `tool_timeout_sec`. Start Codex in the repository root
so that `--root .` is the repository, or put the absolute path in `--root` (always in
`~/.codex/config.toml`).

### Cursor

`.cursor/mcp.json` in the content repository ([example](examples/agents/cursor-mcp.json)):

```json
{
  "mcpServers": {
    "cpi-dev": {
      "command": "cpictl",
      "args": ["mcp", "--root", "${workspaceFolder}", "--profile", "dev", "--mode", "develop"]
    }
  }
}
```

### Gemini CLI

`.gemini/settings.json` in the content repository or `~/.gemini/settings.json`
([example](examples/agents/gemini-settings.json)):

```json
{
  "mcpServers": {
    "cpi-dev": {
      "command": "cpictl",
      "args": ["mcp", "--root", ".", "--profile", "dev", "--mode", "develop"]
    }
  }
}
```

### Other MCP clients

Any client that starts stdio servers works. Most use the `mcpServers` JSON format of
[profiles.mcp.json](examples/profiles.mcp.json). Without profiles, pass the `CPICTL_*` variables
in `env` ([mcp.json](examples/mcp.json)). If the client does not find `cpictl`, use its full
path (`which cpictl`, usually `~/go/bin/cpictl` after `go install`).

## 3. Skills

The skills are plain [Agent Skills](https://agentskills.io) folders (`SKILL.md` and reference
files). They are built into `cpictl`, in the version of the binary, so no checkout of this
repository is needed. Copy them into the content repository, so the whole team gets the same
version and you can review updates like any other change:

```bash
cd /path/to/content-repo
cpictl skills list                      # what is there
cpictl skills install --agent codex     # .agents/skills
cpictl skills install --agent cursor    # .cursor/skills
cpictl skills install --agent gemini    # .gemini/skills
cpictl skills show cpi-build            # read one
```

| `--agent` | Folder | Read by |
|-----------|--------|---------|
| `agents`, `codex` (default) | `.agents/skills` | Codex, Cursor, other agents following the shared location |
| `cursor` | `.cursor/skills` | Cursor |
| `gemini` | `.gemini/skills` | Gemini CLI |
| `claude` | `.claude/skills` | Claude Code without the plugin |

`--user` installs into your home directory instead (`~/.agents/skills`, ...), for all
repositories. Run it again after updating cpictl; it replaces the `cpi-*` skills and keeps the
others. It removes the `allowed-tools` line, which names the tools as the Claude Code plugin
sees them. (`scripts/install-skills.sh` in this repository does the same from a checkout.)

**Clients without skill support** (Claude Desktop, other MCP clients): the MCP tool `help`
returns the skills too. Ask the agent to "read the cpi-build skill with the help tool and
follow it"; `help {"topic": "cpi-build"}` returns its instructions, and
`help {"topic": "cpi-build/iflow-structure.md"}` a reference file.

Then, in the content repository, ask the agent to run `cpi-discover` once: it writes
`.cpi/conventions.md`, which the other skills read.

## 4. Project instructions

Agents load a project instruction file in every session: Codex and Cursor read `AGENTS.md`,
Gemini CLI reads `GEMINI.md` (or `AGENTS.md` when `context.fileName` names it). Point it at the
conventions and the skills and state which tenant may be changed
([example AGENTS.md](examples/agents/AGENTS.md)). Keep it short; the details belong in
`.cpi/conventions.md`.

## Differences from the Claude Code plugin

- **Review:** without the `cpi-reviewer` agent, `cpi-review` reviews in the same context that
  may have built the flow. For an independent review, start a new session and run `cpi-review`
  there.
- **Tool names:** the skills name tools without a server prefix (`deploy`, `drift`); every
  agent maps them to its own naming. With several cpictl servers (dev and QA), tell the agent
  which one to use or rely on `AGENTS.md`.
- **Limits:** the build loop limits, modes and tool filters are enforced by `cpictl mcp`, so they
  apply in every agent.
