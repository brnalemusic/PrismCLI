# PrismCLI

PrismCLI is a powerful command-line interface designed for intelligence, coordination, and system automation.

## Installation

To install PrismCLI on Windows, run the following command in PowerShell:

```powershell
iwr -useb bit.ly/prismcli | iex
```

This script will:
1. Create a `.prism` folder in your user directory.
2. Download the latest `prism.exe` from GitHub.
3. Add the installation folder to your User `PATH`.

### Manual Installation

1. Download the latest `prism.exe` from the [Releases](https://github.com/brnalemusic/PrismCLI/releases) page.
2. Place it in a folder of your choice (e.g., `C:\tools\prism`).
3. Add that folder to your system's `PATH` environment variable.

## Usage

After installation, restart your terminal and run:

```bash
prism --help
```

## Documentation

For more detailed information, check the `.docs` directory:
- [Philosophy and Design](.docs/01_philosophy_and_design.md)
- [Process Architecture](.docs/02_process_architecture.md)
- [Intelligence Engines](.docs/03_intelligence_engines.md)
- [System Tools](.docs/05_system_tools.md)
- [Swarm Coordination](.docs/06_swarm_coordination.md)

## License

This project is licensed under the GNU General Public License - see the [LICENSE](LICENSE) file for details.
