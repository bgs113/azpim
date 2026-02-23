# azpim

A CLI tool for managing Azure Privileged Identity Management (PIM) role assignments for Azure Resources (RBAC). List eligible and active assignments, activate roles with interactive prompts, and deactivate them — all from the terminal.

## Prerequisites

- [Azure CLI](https://learn.microsoft.com/en-us/cli/azure/install-azure-cli) (`az`) — used for authentication
- An Azure account with PIM-eligible role assignments

## Installation

Releases are distributed as ZIP files hosted on SharePoint. SharePoint requires authentication, so command-line tools like `curl` or `wget` won't work — **download the ZIP using your web browser**.

### macOS

Download the ZIP for your architecture from SharePoint:

| Architecture  | File                            |
| ------------- | ------------------------------- |
| Apple Silicon | `azpim-vX.Y.Z-darwin-arm64.zip` |
| Intel         | `azpim-vX.Y.Z-darwin-amd64.zip` |

Unzip and install (replace the filename with the version you downloaded):

```bash
# Apple Silicon
unzip azpim-vX.Y.Z-darwin-arm64.zip
mkdir -p ~/.local/bin
install -m 755 azpim-vX.Y.Z-darwin-arm64 ~/.local/bin/azpim

# Intel
unzip azpim-vX.Y.Z-darwin-amd64.zip
mkdir -p ~/.local/bin
install -m 755 azpim-vX.Y.Z-darwin-amd64 ~/.local/bin/azpim
```

Ensure `~/.local/bin` is on your PATH (add to `~/.zshrc` if needed):

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

> **Gatekeeper prompt**: Because the binary was downloaded via a browser, macOS stamps it with a quarantine attribute and will block it on first run. Go to **System Settings → Privacy & Security** and click **Allow Anyway**, or clear the attribute from the terminal:
>
> ```bash
> xattr -dr com.apple.quarantine ~/.local/bin/azpim
> ```

### Linux

Download `azpim-vX.Y.Z-linux-amd64.zip` from SharePoint. Unzip and install (replace the filename with the version you downloaded):

```bash
unzip azpim-vX.Y.Z-linux-amd64.zip
mkdir -p ~/.local/bin
install -m 755 azpim-vX.Y.Z-linux-amd64 ~/.local/bin/azpim
```

Ensure `~/.local/bin` is on your PATH (add to `~/.bashrc` if needed):

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

### Windows

Download `azpim-vX.Y.Z-windows-amd64.zip` from SharePoint.

Extract the ZIP — in File Explorer: right-click → **Extract All**, or in PowerShell (replace the filename with the version you downloaded):

```powershell
Expand-Archive -Path azpim-vX.Y.Z-windows-amd64.zip -DestinationPath .
Rename-Item .\azpim-vX.Y.Z-windows-amd64.exe azpim.exe
```

> **SmartScreen warning**: Windows may block the executable because it is not code-signed. If you see a "Windows protected your PC" dialog, click **More info → Run anyway**.

**Option A — Add to a directory already on your PATH:**

```powershell
Move-Item azpim.exe C:\Windows\System32\azpim.exe
```

**Option B — Add a new directory to your PATH (no admin required):**

```powershell
New-Item -ItemType Directory -Force -Path "$HOME\bin"
Move-Item azpim.exe "$HOME\bin\azpim.exe"
[Environment]::SetEnvironmentVariable("Path", "$HOME\bin;" + $env:Path, "User")
```

Restart your terminal after updating PATH.

### Verify installation

```bash
azpim --version
```

### Uninstallation

**macOS / Linux** (binary install or `make install` — both use the same location):

```bash
rm ~/.local/bin/azpim
```

Or if you built from source:

```bash
make uninstall
```

**Windows Option A** (System32, requires admin):

```powershell
Remove-Item C:\Windows\System32\azpim.exe
```

**Windows Option B** (`$HOME\bin`):

```powershell
Remove-Item "$HOME\bin\azpim.exe"
# Optionally remove $HOME\bin from PATH if nothing else uses it:
[Environment]::SetEnvironmentVariable("Path", ($env:Path -split ";" | Where-Object { $_ -ne "$HOME\bin" }) -join ";", "User")
```

---

## Building from source

Requires [Go 1.22+](https://go.dev/dl/).

```bash
git clone https://github.com/yourorg/azpim.git
cd azpim
```

**Build for the current platform:**

```bash
make build        # produces ./azpim
```

**Install to `~/.local/bin` (no sudo):**

```bash
make install
```

**Cross-compile release binaries for all platforms:**

```bash
make release
```

Produces versioned zipped binaries in `dist/`:

```text
dist/azpim-v0.2.0-darwin-arm64.zip
dist/azpim-v0.2.0-darwin-amd64.zip
dist/azpim-v0.2.0-linux-amd64.zip
dist/azpim-v0.2.0-windows-amd64.zip
```

The version is derived automatically from the current git tag (e.g. `v0.2.0`). Tag before releasing:

```bash
git tag -a v0.2.0 -m "v0.2.0"
make release
```

---

## Authentication

`azpim` uses [DefaultAzureCredential](https://learn.microsoft.com/en-us/azure/developer/go/azure-sdk-authentication), which automatically picks up credentials from the following sources (in order):

1. Environment variables (`AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, `AZURE_TENANT_ID`)
2. Workload identity / managed identity
3. **Azure CLI** (`az login`) — the most common for local use

For personal use, simply log in with the Azure CLI:

```bash
az login
```

---

## Usage

### Scope flags

All commands accept the same scope flags to target a specific part of your Azure hierarchy:

| Flag                                        | Description        | ARM Scope                                                     |
| ------------------------------------------- | ------------------ | ------------------------------------------------------------- |
| `--subscription <id>`                       | Subscription       | `/subscriptions/<id>`                                         |
| `--subscription <id> --resource-group <rg>` | Resource group     | `/subscriptions/<id>/resourceGroups/<rg>`                     |
| `--management-group <id>`                   | Management group   | `/providers/Microsoft.Management/managementGroups/<id>`       |
| `--management-group /`                      | Tenant root group  | `/providers/Microsoft.Management/managementGroups/<tenantId>` |
| `--scope <arm-scope>`                       | Explicit ARM scope | (as provided)                                                 |

If no scope is specified, `azpim` automatically discovers all accessible subscriptions and management groups.

The `--subscription` and `--management-group` flags also read from environment variables `AZURE_SUBSCRIPTION_ID` and `AZURE_MANAGEMENT_GROUP_ID` respectively.

### Output flags

| Flag                       | Description                                                    |
| -------------------------- | -------------------------------------------------------------- |
| `-o, --output table\|json` | Output format (default: `table`)                               |
| `--human`                  | Human-readable time format (`1h 32m 5s` instead of `01:32:05`) |

---

### `azpim eligible` — List eligible assignments

List all PIM-eligible role assignments for the current principal.

```bash
azpim eligible
azpim eligible --subscription 00000000-0000-0000-0000-000000000000
azpim eligible --management-group myMG
azpim eligible --management-group /
azpim eligible --subscription <id> --resource-group myRG
azpim eligible --output json
```

**Table output columns:** ROLE · SCOPE · RESOURCE TYPE · MEMBERSHIP · CONDITION · END TIME

---

### `azpim active` — List active assignments

List active (time-bound) PIM role assignments for the current principal. Permanent assignments are hidden by default.

```bash
azpim active
azpim active --include-permanent
azpim active --subscription <id>
azpim active --human
azpim active --output json
```

**Table output columns:** ROLE · RESOURCE · RESOURCE TYPE · MEMBERSHIP · CONDITION · STATE · END TIME · TIME REMAINING

State is color-coded: **green** = Active, **cyan** = Permanent, **yellow** = Pending, **red** = Failed/Expired.

---

### `azpim activate` — Activate an eligible role

Activate an eligible PIM role assignment. If `--role` or `--justification` are omitted, interactive prompts appear.

```bash
# Fully interactive
azpim activate

# Specify role and scope; prompts for duration and justification
azpim activate --subscription <id> --role "Contributor"

# Non-interactive
azpim activate \
  --subscription <id> \
  --role "Contributor" \
  --duration 4 \
  --justification "Incident response for ticket #1234" \
  --ticket-number 1234 \
  --ticket-system https://myorg.atlassian.net
```

**Flags:**

| Flag                         | Description                                                                                     |
| ---------------------------- | ----------------------------------------------------------------------------------------------- |
| `--role <name>`              | Role name to activate (interactive list if omitted)                                             |
| `-d, --duration <hours>`     | Activation duration in hours, e.g. `4` or `4h` (prompts if omitted; defaults to policy maximum) |
| `-j, --justification <text>` | Justification text (prompts if omitted)                                                         |
| `--ticket-number <num>`      | Ticket or incident number                                                                       |
| `--ticket-system <url>`      | Ticket system URL                                                                               |

The duration prompt defaults to the **maximum allowed by the role's management policy** and validates that the requested duration does not exceed it.

---

### `azpim deactivate` — Deactivate an active role

Deactivate an active (time-bound) PIM role assignment. If `--role` is omitted, an interactive list is presented.

```bash
# Interactive selection
azpim deactivate

# Specify role directly
azpim deactivate --subscription <id> --role "Contributor"

# Deactivate all active roles (shows list and prompts for confirmation)
azpim deactivate --all
```

**Flags:**

| Flag            | Description                                            |
| --------------- | ------------------------------------------------------ |
| `--role <name>` | Role name to deactivate (interactive list if omitted)  |
| `--all`         | Deactivate all active roles (prompts for confirmation) |

---

## Examples

```bash
# See what roles you can activate across all scopes
azpim eligible

# Activate Contributor on a subscription interactively
azpim activate --subscription 00000000-0000-0000-0000-000000000000

# Activate with all options specified (no prompts)
azpim activate \
  --subscription 00000000-0000-0000-0000-000000000000 \
  --role "Owner" \
  --duration 2 \
  --justification "Emergency access"

# Check what's currently active with time remaining
azpim active --human

# Deactivate a specific role
azpim deactivate --role "Contributor"

# Deactivate everything (with confirmation)
azpim deactivate --all

# Export active roles as JSON
azpim active --output json | jq '.[].role_name'
```
