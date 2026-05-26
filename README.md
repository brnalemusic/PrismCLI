# PrismCLI

**One interface. Total control.**

PrismCLI is the command-line counterpart of [Prism](https://github.com/brnalemusic/Prism) — a local AI agent that runs natively on Windows and has real access to your file system, terminal, applications, and the web. No browser required. No cloud sync. Just intelligence at your command line.

[![License](https://img.shields.io/badge/License-GPL%20v3.0-6C63FF?style=flat-square)](./LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Windows-C084FC?style=flat-square)](#installation)
[![Version](https://img.shields.io/badge/Version-0.1.1-F0F0F5?style=flat-square)](https://github.com/brnalemusic/PrismCLI/releases)

---

## What it does

PrismCLI gives you a conversational AI session directly in your terminal. Unlike web-based assistants that only talk, Prism acts — reading files, running shell commands, searching the web, launching apps, and even spawning parallel AI agents to tackle complex tasks.

---

## Installation

**One-line install (PowerShell):**

```powershell
iwr bit.ly/prismcli | iex
```

This will:
1. Create a `.prism` folder in your user directory
2. Download the latest `prism.exe` from GitHub Releases
3. Add it to your `PATH` automatically

**Manual install:**
1. Download `prism.exe` from the [Releases](https://github.com/brnalemusic/PrismCLI/releases) page
2. Place it in a folder of your choice (e.g. `C:\tools\prism`)
3. Add that folder to your system `PATH`

After installing, restart your terminal and run:

```
prism --help
```

---

## Setup

On first run, Prism will launch a setup wizard to configure your Gemini API key. You can re-run it anytime:

```
prism --config
```

Or from inside a session:

```
/config
```

---

## Usage

```
prism                  Start a chat session
prism --search         Start with Active Search enabled
prism --deep           Start with Deep Research enabled
prism --version        Show current version
prism --config         Open the API key setup wizard
prism list             List saved chat sessions
```

---

## In-session commands

Once inside a chat session, the following slash commands are available:

| Command | Description |
|---|---|
| `/search` | Toggle active web search on/off |
| `/deep` or `/research` | Toggle deep research mode (extended web investigation) |
| `/think` | Toggle Think Mode (extended reasoning) |
| `/model` | View available models |
| `/model <n>` | Switch to model by number or name (e.g. `/model 1` or `/model Prism 5`) |
| `/youtube <term>` | Open a YouTube search directly in your browser |
| `/swarm <goal>` | Spawn a parallel agent swarm to solve a complex task |
| `/config` | Reconfigure your API key |
| `/clear` | Clear the screen and reset chat history |
| `/exit` or `/quit` | Exit PrismCLI |
| `/help` | Show all available commands |

**Keyboard shortcuts:**

| Shortcut | Action |
|---|---|
| `Ctrl+T` | Toggle Think Mode |
| `Ctrl+M` | Open interactive model selector (arrow keys + Enter) |

---

## Models

PrismCLI shares the same model family as Prism Desktop:

| Model | Best for |
|---|---|
| **Prism 5** | Flagship — complex tasks and fast execution |
| **Prism 4.3** | Deep reasoning and careful planning |
| **Prism 4.2** | Balanced multi-step automation |
| **Prism 4.1** | Responsive everyday assistance |
| **Prism 4** | Lightweight tasks and quick answers |

All models support **Think Mode**, which can be toggled on demand for higher reasoning quality.

---

## System capabilities

Prism can interact with your computer through the following built-in tools:

| Capability | What it does |
|---|---|
| **Terminal** | Execute shell commands (PowerShell, cmd) |
| **File System** | Create, read, edit, move, copy, and delete files and directories |
| **Web Search** | Search via DuckDuckGo for real-time results |
| **Web Reader** | Fetch and extract content from any URL |
| **Applications** | List installed apps and launch them |
| **Browser** | Open URLs in your default browser |
| **Agent Swarm** | Spawn parallel AI subagents to coordinate complex tasks |
| **Mini-Apps** | Generate interactive HTML modules, saved locally and opened in the browser |
| **Chat History Search** | Semantic search across past sessions |

---

## Privacy

Your API key is stored locally in your `.prism` config folder and never leaves your machine. PrismCLI only makes outbound requests to the Gemini API. No telemetry, no cloud sync.

---

## Building from source

```bash
git clone https://github.com/brnalemusic/PrismCLI.git
cd PrismCLI
go build -o prism.exe .
```

Requires Go 1.21+.

---

## Related

- [Prism Desktop](https://github.com/brnalemusic/Prism) — the full native desktop experience with GUI

---

## License

GPL-3.0 — see [LICENSE](./LICENSE) for details.