# azpim

A CLI tool for managing Azure Privileged Identity Management (PIM) role assignments for Azure Resources (RBAC). List eligible and active assignments, activate roles with interactive prompts, and deactivate them — all from the terminal.

## Prerequisites

- [Azure CLI](https://learn.microsoft.com/en-us/cli/azure/install-azure-cli) (`az`) — used for authentication
- An Azure account with PIM-eligible role assignments

## Installation

Pre-built binaries are available from [GitHub Releases](https://github.com/bgs113/azpim/releases/latest). Each release also includes a `checksums.txt` file with SHA256 hashes and an `.sbom.json` SPDX software bill of materials for each archive.

### macOS (Homebrew)

```bash
brew install --cask bgs113/tap/azpim
```

Update with:

```bash
brew upgrade --cask azpim
```

> **Gatekeeper prompt**: If macOS blocks the binary on first run, go to **System Settings → Privacy & Security** and click **Allow Anyway**, or clear the quarantine attribute:
>
> ```bash
> xattr -dr com.apple.quarantine "$(which azpim)"
> ```

### macOS (ZIP download)

Download the ZIP for your architecture:

| Architecture  | File                            |
| ------------- | ------------------------------- |
| Apple Silicon | `azpim-vX.Y.Z-darwin-arm64.zip` |
| Intel         | `azpim-vX.Y.Z-darwin-amd64.zip` |

Unzip and install (replace `X.Y.Z` with the version you downloaded):

```bash
# Apple Silicon
unzip azpim-vX.Y.Z-darwin-arm64.zip
mkdir -p ~/.local/bin
install -m 755 azpim ~/.local/bin/azpim

# Intel
unzip azpim-vX.Y.Z-darwin-amd64.zip
mkdir -p ~/.local/bin
install -m 755 azpim ~/.local/bin/azpim
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

Download the ZIP for your architecture from [GitHub Releases](https://github.com/bgs113/azpim/releases/latest):

| Architecture | File                            |
| ------------ | ------------------------------- |
| x86-64       | `azpim-vX.Y.Z-linux-amd64.zip`  |
| ARM64        | `azpim-vX.Y.Z-linux-arm64.zip`  |

Unzip and install (replace `X.Y.Z` with the version you downloaded):

```bash
unzip azpim-vX.Y.Z-linux-amd64.zip
mkdir -p ~/.local/bin
install -m 755 azpim ~/.local/bin/azpim
```

Ensure `~/.local/bin` is on your PATH (add to `~/.bashrc` if needed):

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

### Windows

Download `azpim-vX.Y.Z-windows-amd64.zip` from [GitHub Releases](https://github.com/bgs113/azpim/releases/latest).

Extract the ZIP — in File Explorer: right-click → **Extract All**, or in PowerShell
(replace `X.Y.Z` with the version you downloaded):

```powershell
Expand-Archive -Path azpim-vX.Y.Z-windows-amd64.zip -DestinationPath .
```

> **SmartScreen warning**: Windows may block the executable because it is not code-signed. If you see a "Windows protected your PC" dialog, click **More info → Run anyway**.

#### Install to a per-user bin directory (recommended)

This installs azpim for your user only and does not require admin rights.

```powershell
# Create a per-user bin directory if it doesn't exist
New-Item -ItemType Directory -Force -Path "$HOME\bin"

# Move the binary into place (overwrite-in-place on updates)
Move-Item -Force azpim.exe "$HOME\bin\azpim.exe"

# Add the directory to your user PATH (one-time setup)
[Environment]::SetEnvironmentVariable(
  "Path",
  "$HOME\bin;" + [Environment]::GetEnvironmentVariable("Path", "User"),
  "User"
)
```

Restart your terminal after updating PATH.

After this, you can update azpim by simply replacing `$HOME\bin\azpim.exe` with a newer version.

### Verify the download

After downloading, verify the checksum against `checksums.txt` from the same release:

```bash
# macOS
shasum -a 256 --check checksums.txt --ignore-missing

# Linux
sha256sum --check checksums.txt --ignore-missing
```

To also verify the signature on `checksums.txt` itself (requires [Cosign](https://docs.sigstore.dev/cosign/system_config/installation/)):

```bash
cosign verify-blob \
  --certificate-identity-regexp='https://github.com/bgs113/azpim/.github/workflows/release.yml' \
  --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
  --bundle checksums.txt.bundle \
  checksums.txt
```

```powershell
# Windows (replace filename with the version you downloaded)
$file = "azpim-vX.Y.Z-windows-amd64.zip"
$hash = (Get-FileHash $file -Algorithm SHA256).Hash.ToLower()
$expected = (Select-String $file checksums.txt).Line.Split(" ")[0]
if ($hash -eq $expected) { "OK" } else { "MISMATCH" }
```

### Verify installation

```bash
azpim --version
```

### Uninstallation

**macOS / Linux** (binary install, `gh release download`, or `make install` — all use the same location):

```bash
rm ~/.local/bin/azpim
```

Or if you built from source:

```bash
make uninstall
```

**Windows** (`$HOME\bin`):

```powershell
Remove-Item "$HOME\bin\azpim.exe"

# Optionally remove $HOME\bin from PATH if nothing else uses it
[Environment]::SetEnvironmentVariable(
  "Path",
  ([Environment]::GetEnvironmentVariable("Path", "User") -split ";" |
    Where-Object { $_ -ne "$HOME\bin" }) -join ";",
  "User"
)
```

### Container image

`azpim` is published to the [GitHub Container Registry](https://ghcr.io/bgs113/azpim) as a hardened image built with [Ko](https://ko.build) on a [Chainguard](https://cgr.dev) distroless base (nonroot, near-zero CVEs). A new image is pushed automatically on every release. Each image has an SPDX SBOM attached to its manifest in the registry.

**Pull the latest image:**

```bash
docker pull ghcr.io/bgs113/azpim:latest
```

**Verify the image signature** (requires [Cosign](https://docs.sigstore.dev/cosign/system_config/installation/)):

```bash
cosign verify \
  --certificate-identity-regexp='https://github.com/bgs113/azpim/.github/workflows/release.yml' \
  --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
  ghcr.io/bgs113/azpim:latest
```

Signatures are keyless — the certificate proves the image was built by the Release workflow in this repository and is recorded in [Rekor's](https://rekor.sigstore.dev) public transparency log.

**Run using your existing `az login` session** (mounts Azure CLI credentials and cache from the host):

```bash
docker run --rm -it \
  -v ~/.azure:/home/nonroot/.azure:ro \
  -v ~/.cache/azpim:/home/nonroot/.cache/azpim \
  ghcr.io/bgs113/azpim:latest eligible

docker run --rm -it \
  -v ~/.azure:/home/nonroot/.azure:ro \
  -v ~/.cache/azpim:/home/nonroot/.cache/azpim \
  ghcr.io/bgs113/azpim:latest activate \
    --subscription 00000000-0000-0000-0000-000000000000 \
    --role Contributor --duration 4 --justification "Incident response"
```

**Run using a service principal** (environment variable auth — no volume mount needed):

```bash
docker run --rm -it \
  -e AZURE_TENANT_ID=<tenant-id> \
  -e AZURE_CLIENT_ID=<client-id> \
  -e AZURE_CLIENT_SECRET=<client-secret> \
  ghcr.io/bgs113/azpim:latest eligible
```

> **Interactive prompts:** Commands that show interactive selection menus (e.g. `azpim activate` without `--role`) require a TTY, which `docker run -it` provides. Fully flag-specified commands work without `-t`.

---

## Building from source

Requires [Go 1.26+](https://go.dev/dl/).

**Build for the current platform:**

```bash
make build        # produces ./azpim
```

**Install to `~/.local/bin` (no sudo):**

```bash
make install
```

**Test a local release build without publishing:**

```bash
make snapshot   # produces dist/ artifacts via GoReleaser, no tag required
```

**Cut a release** (requires [GoReleaser](https://goreleaser.com) — installed via `mise install`):

```bash
git tag -a v0.4.0 -m "v0.4.0"
git push --tags   # triggers the release CI workflow automatically
```

The CI workflow cross-compiles for all platforms, creates the GitHub Release with ZIP artifacts, per-archive SPDX SBOMs, and `checksums.txt`, builds and pushes the container image to GHCR via Ko, and signs the image with keyless Cosign.

---

## Authentication

`azpim` uses [DefaultAzureCredential](https://learn.microsoft.com/en-us/azure/developer/go/azure-sdk-authentication), which automatically picks up credentials from the following sources (in order):

1. Environment variables (`AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, `AZURE_TENANT_ID`)
2. Workload identity / managed identity
3. **Azure CLI** (`az login`) — the most common for local use
4. **Azure Developer CLI** (`azd auth login`)

For personal use, log in with either the Azure CLI or the Azure Developer CLI:

```bash
az login
# or
azd auth login
```

> **Note:** Azure PowerShell (`Connect-AzAccount`) is not supported — it is not included in the Go SDK's `DefaultAzureCredential` chain.

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

| Flag                         | Description                                                                                                           |
| ---------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `--role <name>`              | Role name to activate (interactive list if omitted; prompts for scope if the name matches multiple entries)           |
| `-d, --duration <value>`     | Duration as integer hours (`4`), or duration string (`4h30m`, `90m`) — prompts if omitted; defaults to policy maximum |
| `-j, --justification <text>` | Justification text (prompts if omitted)                                                                               |
| `--ticket-number <num>`      | Ticket or incident number                                                                                             |
| `--ticket-system <url>`      | Ticket system URL                                                                                                     |
| `-o, --output table\|json`   | Output format (default: `table`)                                                                                      |

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

| Flag                       | Description                                                                   |
| -------------------------- | ----------------------------------------------------------------------------- |
| `--role <name>`            | Role name to deactivate (interactive list if omitted; disambiguates by scope) |
| `--all`                    | Deactivate all active roles (prompts for confirmation)                        |
| `-y, --yes`                | Skip confirmation prompt when used with `--all`                               |
| `-o, --output table\|json` | Output format (default: `table`)                                              |

---

### `azpim extend` — Extend an active role

Request an extension of an active (time-bound) PIM role assignment. The extension sets a new duration from the current time. Whether it is auto-approved or requires admin approval depends on the role's management policy.

If `--role` or `--justification` are omitted, interactive prompts appear.

```bash
# Interactive selection
azpim extend

# Specify role and new duration
azpim extend --subscription <id> --role "Contributor" --duration 4h
```

**Flags:**

| Flag                         | Description                                                                                  |
| ---------------------------- | -------------------------------------------------------------------------------------------- |
| `--role <name>`              | Role name to extend (interactive list if omitted; disambiguates by scope)                    |
| `-d, --duration <value>`     | New duration from now, e.g. `4h` or `4h30m` — prompts if omitted; defaults to policy maximum |
| `-j, --justification <text>` | Justification text (prompts if omitted)                                                      |
| `--ticket-number <num>`      | Ticket or incident number                                                                    |
| `--ticket-system <url>`      | Ticket system URL                                                                            |
| `-o, --output table\|json`   | Output format (default: `table`)                                                             |

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

# Same workflows via Docker (published image)
docker run --rm -it -v ~/.azure:/home/nonroot/.azure:ro ghcr.io/bgs113/azpim:latest eligible
docker run --rm -it -v ~/.azure:/home/nonroot/.azure:ro ghcr.io/bgs113/azpim:latest active --human
docker run --rm -it -v ~/.azure:/home/nonroot/.azure:ro ghcr.io/bgs113/azpim:latest active --output json
```
