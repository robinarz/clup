# `clup` - A CLI for ClickUp

clup is a command-line interface and terminal UI (TUI) for interacting with the ClickUp API.  
I was bored of the slow web ui and wanted to have a faster, more efficient way to manage my tasks.

## Features

- Interactive TUI: Browse and manage tasks in a fast, keyboard-driven interface.

- CLI Commands: Quickly create new tasks or find existing ones with fzf.

- Task Management:

    - View task details and comments.

    - Create, delete, and edit tasks.

    - Update task status, assignees, and priority.

- Vim-style Editing: An intuitive, modal editing experience for power users.

- Secure: Your API token and team ID are stored locally in a .env file.

## Installation

### From Source

Clone the repository:

```bash
git clone <your-repo-url>
cd clup
```

Build and install the binary:

```bash
make build && make install
```

This will place the clup binary in `~/.local/bin`. Make sure this directory is in your `$PATH`.

## From GitHub Releases

You can download the latest pre-compiled binary for your operating system.

```bash
# macOS (Apple Silicon / arm64)

curl -L -o clup https://github.com/robinarz/clup/releases/latest/download/clup-darwin-arm64
chmod +x clup
sudo mv clup $HOME/.local/bin/
```

```bash
# macOS (Intel / amd64)

curl -L -o clup https://github.com/robinarz/clup/releases/latest/download/clup-darwin-amd64
chmod +x clup
sudo mv clup $HOME/.local/bin/
```

```bash
# Linux (amd64)

curl -L -o clup https://github.com/robinarz/clup/releases/latest/download/clup-linux-amd64
chmod +x clup
sudo mv clup $HOME/.local/bin/
```

## Configuration

Before you can use clup, you need to create a `.clup.env` file in your home directory and source it in your shell.

For example, if you're using zsh, you can add the following line to your `.zshrc` file:

```bash
source ~/.clup.env
```

The `.clup.env` file should contain the following environment variables:

```env
CLICKUP_API_TOKEN="pk_12345678_..."
CLICKUP_TEAM_ID="12345678"
```

`CLICKUP_API_TOKEN`: You can find your personal API token in your ClickUp settings under "My Settings" > "Apps".

`CLICKUP_TEAM_ID`: This is your Workspace ID. You can find it in the URL of your ClickUp workspace (e.g., https://app.clickup.com/12345678/...).

## Usage

clup provides several commands to interact with your ClickUp tasks.

```bash
clup
```
Launches the main TUI, which will first prompt you to select a Space and then display all the tasks within that space.

```bash
clup list
```
Instantly search for any task in your workspace using fzf. After selecting a task, you'll be prompted to either view or edit it.

```bash
clup task
```
Launches a step-by-step TUI to create a new task. You'll be guided through selecting a Space and List, and then prompted to enter the task's details.

## Keybindings

### Main List View

| Key | Action               |
|-----|----------------------|
| `v` | View task details    |
| `e` | Edit selected task   |
| `d` | Delete selected task |
| `/` | Filter/Search tasks  |
| `q` | Quit                 |

### Edit View (Normal Mode)

| Key | Action                                  |
|-----|-----------------------------------------|
| `i` | Enter Insert Mode to edit the description |
| `a` | Enter Insert Mode to add a comment      |
| `s` | Change the task's status                |
| `q` | Return to the task list without saving  |
| `:` | Enter Command Mode                      |

### Edit View (Command Mode)

| Command | Action                                     |
|---------|--------------------------------------------|
| `:w`    | Write (save) all changes and return to the list |
| `:q`    | Quit the application                       |
| `:q!`   | Quit without saving and return to the list |

## License

This project is licensed under the MIT License.
