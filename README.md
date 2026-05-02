# ⛵ GitFleet

GitFleet is a blazingly fast, highly concurrent Terminal UI (TUI) for managing massive local Git workspaces. Designed mobile-first for environments like Termux, but gorgeous on macOS, Linux, and Windows.

Instead of managing the cloud, GitFleet manages your local file system. It uses a strictly bounded worker pool to execute mass Git operations ("swarms") across dozens or hundreds of repositories simultaneously without dropping frames or exhausting OS resources.

## 🚀 Features

* **Morning Routine**: Concurrently check the `git status` of all local repos.
* **Sync Swarm**: Securely runs `git pull --rebase --autostash` to sync your code without losing uncommitted work.
* **Cleanup Crew**: Prunes local tracking branches that have been deleted on the remote.
* **Hardware Aware**: Automatically scales worker threads based on your physical CPU cores.
* **Cross-Platform**: Compiled statically for Linux, macOS, Windows, and ARM architectures (Android/Termux).

## 📦 Installation

Download the latest optimized binary for your OS from the [Releases](https://github.com/TangoSplicer/gitfleet/releases) tab.

Alternatively, build from source:

    go install github.com/TangoSplicer/gitfleet@latest

## 🛠️ Usage & Workspace Selection

By default, GitFleet looks for repositories in `~/clones`. You can dynamically point it to any folder:

    # Use the current directory
    gitfleet .

    # Provide a specific path
    gitfleet ~/projects

    # Use the explicit flag
    gitfleet -dir /var/www/html

### ⚙️ Persistent Configuration

Upon first run, GitFleet generates a configuration file. You can edit this file to permanently change your default workspace, set maximum workers, or ignore heavy directories.

* **Linux/Termux:** `~/.config/gitfleet/config.yaml`
* **macOS:** `~/Library/Application Support/gitfleet/config.yaml`
* **Windows:** `%AppData%\gitfleet\config.yaml`

Example config.yaml:

    default_workspace: /home/user/my-custom-folder
    ignore_directories:
        - node_modules
        - target
        - vendor
    max_workers: 0

