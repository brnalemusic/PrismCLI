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

## License

This project is licensed under the GNU General Public License - see the [LICENSE](LICENSE) file for details.
