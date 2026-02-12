# CVE Lookup Tool - Hub API CLI

A comprehensive command-line tool for querying and analyzing vulnerability data from the Hub API. This tool provides functionality to look up CVEs, analyze component vulnerabilities, compare versions, and generate inventory reports.

> [!WARNING]
> **This repository is an unofficial project provided "as is."**
>
> It is not supported by any organization, and no warranty or guarantee of functionality is provided. Use at your own discretion.

## Table of Contents

- [Installation](#installation)
- [Authentication](#authentication)
- [Use Cases](#use-cases)
  - [CVE Lookup](#1-cve-lookup)
  - [Component Vulnerability Lookup](#2-component-vulnerability-lookup)
  - [Component Version Management](#3-component-version-management)
  - [CVE Comparison](#4-cve-comparison)
  - [Tree Navigation](#5-tree-navigation)
  - [Inventory Reports](#6-inventory-reports)
- [Command Reference](#command-reference)
- [Examples](#examples)

## Installation

Build the tool from source (macOS, Linux, Windows):

```bash
go build -o cve-lookup .
```

## Authentication Setup

The tool requires authentication configuration stored in a local file that is not tracked by git.

### First-Time Setup

On first run, if the configuration file doesn't exist, the tool will create a template file `hub-api-config.json` in the same directory as the executable. You'll see instructions like:

```
Authentication configuration file not found.
Created template file: /path/to/hub-api-config.json

Please fill in the following fields in the config file:
  - oauthAppId: Your OAuth application ID
  - oauthAppSecret: Your OAuth application secret
  - graphqlEndpoint: GraphQL endpoint URL (required)

The config file has been created with empty values.
After filling it in, run the command again.
```

### Configuration File Format

Edit `hub-api-config.json` and fill in your credentials:

```json
{
  "oauthAppId": "your-oauth-app-id-here",
  "oauthAppSecret": "your-oauth-app-secret-here",
  "graphqlEndpoint": "https://your-hub-host/hub/graphql"
}
```

**Note**: All three fields are required.

### Security

- The configuration file (`hub-api-config.json`) is automatically excluded from git via `.gitignore`
- The file is created with permissions `0600` (read/write for owner only)
- Never commit this file to version control

### Token Generation

Once configured, the tool automatically generates an OAuth access token when needed. You can also generate a token manually:

```bash
./cve-lookup -generate-token
```

Or provide your own token:

```bash
./cve-lookup -cve CVE-2025-15467 -token <your-token>
```

## Use Cases

### 1. CVE Lookup

**Purpose**: Find all components affected by a specific CVE and their vulnerability status.

**Use Case**: When you need to know which components in your infrastructure are affected by a particular CVE and what their triage status is (e.g., "affected", "false positive", "not affected").

**Command**:
```bash
./cve-lookup -cve CVE-2025-15467
```

**Output**: 
- Lists all components affected by the CVE
- Shows component details (name, type, foundation, version)
- Displays vulnerability status (severity, triage status, patch status)

**Example Output**:
```
Looking up CVE: CVE-2025-15467

Found 3 component(s) affected by CVE-2025-15467:

--- Component 1/3 ---
Component Name: bosh-vsphere-esxi-ubuntu-jammy-go_agent
Component Type: Stemcells
Foundation: opsman.elasticsky.cloud
Current Version: 1.915
Vulnerability Status:
  - CVE ID: CVE-2025-15467
  - Severity: HIGH
  - Triage Status: AFFECTED
  - Patch Status: PATCHED
```

### 2. Component Vulnerability Lookup

**Purpose**: Get all vulnerabilities affecting a specific component.

**Use Case**: When you need to see all CVEs affecting a particular component, optionally filtered by severity or triage status.

**Basic Command**:
```bash
./cve-lookup -component "vrn/provider:TAS/instance:p-bosh-d882816bed6e9c00f303/Stemcell:bosh-vsphere-esxi-ubuntu-jammy-go_agent-1.915"
```

**With Filters**:
```bash
./cve-lookup -component "vrn/provider:TAS/instance:p-bosh-d882816bed6e9c00f303/Stemcell:bosh-vsphere-esxi-ubuntu-jammy-go_agent-1.915" -severity HIGH,CRITICAL -triage AFFECTED,IN_TRIAGE,NOT_AFFECTED,FALSE_POSITIVE
```
(Triage values: `IN_TRIAGE`, `NOT_AFFECTED`, `FALSE_POSITIVE`, `AFFECTED`.)

**Output**:
- Lists all vulnerabilities for the component
- Shows vulnerability details (ID, severity, triage status, patch status, URL)
- Extracts CVE IDs from URLs when available

### 3. Component Version Management

**Purpose**: List all available versions of a component and compare CVEs across multiple versions.

#### 3.1 List All Versions

**Use Case**: When you need to see all available versions of a component, identify which version is currently deployed, and see CVE counts for each version.

**Command**:
```bash
./cve-lookup -list-versions -component "vrn/provider:TAS/instance:p-bosh-d882816bed6e9c00f303/Stemcell:bosh-vsphere-esxi-ubuntu-jammy-go_agent-1.915"
```

**Output**:
- Lists all available versions sorted by version number
- Tags the current version with `[CURRENT]`
- Shows CVE counts (Critical and High) for each version
- Displays version IDs (for reference)
- Provides example command for comparing versions

**Example Output**:
```
Available Versions (3):

  1. 1.910 [CURRENT]
     CVEs: 5 Critical, 12 High
     ID: abc123-def456-ghi789

  2. 1.915
     CVEs: 3 Critical, 8 High
     ID: xyz789-abc123-def456

  3. 1.920
     CVEs: 2 Critical, 6 High
     ID: mno456-pqr789-stu123

To compare versions, use: -compare-versions -component <component-id> -versions <version1>,<version2>,<version3>
Example: -compare-versions -component vrn/provider:... -versions 1.910,1.915,1.920
```

### 4. CVE Comparison

**Purpose**: Compare CVEs between different versions of a component to understand what vulnerabilities are resolved, new, or remain across versions.

#### 4.1 Compare Current vs Latest Patch Version

**Use Case**: Quickly see what CVEs are fixed in the latest patch version compared to the current deployed version.

**Command**:
```bash
./cve-lookup -compare-cves -component "vrn/provider:TAS/instance:p-bosh-d882816bed6e9c00f303/Stemcell:bosh-vsphere-esxi-ubuntu-jammy-go_agent-1.915"
```

**Output**:
- Comparison table showing:
  - CVE ID
  - Severity (doesn't change between versions)
  - Current Status (triage status in current version)
  - Patch Status (triage status in latest patch version)
  - Delta (RESOLVED, NEW, or REMAINS)
- Summary statistics:
  - Number of CVEs resolved in patch
  - Number of new CVEs in patch
  - Number of CVEs that remain
  - Total CVEs in each version

**Example Output**:
```
CVE Comparison Table
====================

CVE ID               | Severity        | Current Status      | Patch Status        | Delta     
--------------------------------------------------------------------------------------------
CVE-2025-15467      | HIGH            | AFFECTED            | N/A                 | RESOLVED  
CVE-2025-15468      | CRITICAL        | IN_TRIAGE           | IN_TRIAGE           | REMAINS   
CVE-2025-15469      | HIGH            | N/A                 | AFFECTED            | NEW       

Summary:
  Resolved in patch: 1
  New in patch: 1
  Remains: 1
  Total CVEs in current version: 2
  Total CVEs in patch version: 2
```

#### 4.2 Compare Multiple Versions (Three-Way or More)

**Use Case**: Compare CVEs across multiple versions (e.g., current, intermediate, and latest) to understand the vulnerability evolution over time. Supports comparing any number of versions.

**Command** (with explicit versions):
```bash
./cve-lookup -compare-versions -component "vrn/provider:TAS/instance:p-bosh-d882816bed6e9c00f303/Stemcell:bosh-vsphere-esxi-ubuntu-jammy-go_agent-1.915" -versions "1.910,1.915,1.920"
```

**Command** (default: current vs latest):
```bash
./cve-lookup -compare-versions -component "vrn/provider:TAS/instance:p-bosh-d882816bed6e9c00f303/Stemcell:bosh-vsphere-esxi-ubuntu-jammy-go_agent-1.915"
```
If `-versions` is omitted, the tool compares the component's **current** version with the **latest** version.

**Output**:
- Comparison table with columns for each version
- Current version is tagged with `[CURRENT]` in the header
- Shows CVE status (triage) for each version
- Displays "N/A" for CVEs not present in a version
- Summary showing total CVEs per version

**Example Output**:
```
CVE Comparison Table
====================

CVE ID               | Severity        | 1.910 [CURRENT]     | 1.915               | 1.920               
--------------------------------------------------------------------------------------------------------
CVE-2025-15467      | HIGH            | AFFECTED            | AFFECTED            | N/A                 
CVE-2025-15468      | CRITICAL        | IN_TRIAGE           | IN_TRIAGE           | IN_TRIAGE            
CVE-2025-15469      | HIGH            | N/A                 | AFFECTED            | AFFECTED             

Summary:
  1.910 [CURRENT]: 2 CVEs
  1.915: 2 CVEs
  1.920: 2 CVEs
```

**Note**: Version numbers (e.g., "1.910") are used, not version IDs. Get version numbers from the `-list-versions` command.

### 5. Tree Navigation

**Purpose**: Explore the structure of foundations, components, and component types in your infrastructure.

#### 5.1 List All Foundations

**Use Case**: Get an overview of all foundations available in the system.

**Command**:
```bash
./cve-lookup -list-foundations
```

**Output**:
- Lists all unique foundations
- Shows foundation group names
- Sorted alphabetically

#### 5.2 List Component Types

**Use Case**: See what types of components are available and their counts.

**Command** (all foundations):
```bash
./cve-lookup -list-component-types
```

**Command** (filtered by foundation):
```bash
./cve-lookup -list-component-types -foundation "opsman.elasticsky.cloud"
```

**Output**:
- Lists all component types with counts:
  - Buildpack
  - Stemcells
  - Tiles/Services
  - Tiles/Foundation Management

#### 5.3 List Components

**Use Case**: Browse all components, optionally filtered by foundation and/or component type.

**Command** (all components):
```bash
./cve-lookup -list-components
```

**Command** (filtered by foundation):
```bash
./cve-lookup -list-components -foundation "opsman.elasticsky.cloud"
```

**Command** (filtered by component type):
```bash
./cve-lookup -list-components -component-type "Stemcells,Buildpack"
```

**Command** (filtered by both):
```bash
./cve-lookup -list-components -foundation "opsman.elasticsky.cloud" -component-type "Stemcells"
```

**Output**:
- Groups components by foundation
- Shows component details:
  - Name
  - Type
  - Current version
  - Latest patch version
  - Critical/High CVE counts
  - CVEs fixed by patch and percent fixed
  - Component ID

**Use case — list all Buildpacks in a foundation**:
```bash
./cve-lookup -list-components -foundation "your-foundation-host" -component-type Buildpack
```
Use `-csv` for CSV output.

### 6. Inventory Reports

**Purpose**: Generate comprehensive inventory reports of all components with vulnerability metrics.

**Use Case**: Get a complete overview of your infrastructure's security posture, including which components have patches available and how many CVEs would be fixed by upgrading.

**Command** (pretty format):
```bash
./cve-lookup -inventory
```

**Command** (CSV format for export):
```bash
./cve-lookup -inventory --csv
```

**Command** (filtered by foundation):
```bash
./cve-lookup -inventory -foundation "opsman.elasticsky.cloud"
```

**Output Fields**:
- Foundation name
- Component name
- Foundation group
- Current version
- Critical CVE count
- High CVE count
- Latest patch version
- CVEs fixed by patch (calculated delta)
- Percent of vulnerabilities fixed
- Component type
- Component ID

**CSV Output**: Suitable for importing into spreadsheets or other analysis tools.

**Pretty Output**: Human-readable format grouped by foundation.

## Command Reference

### Global Flags

- `-endpoint <url>`: GraphQL endpoint URL (default: configured endpoint)
- `-token <token>`: OAuth access token (if not provided, will generate one)
- `-generate-token`: Generate and print OAuth token, then exit
- **`-csv`**: Output in CSV format. Supported by all commands that produce tabular output (inventory, list-foundations, list-components, list-component-types, list-frameworks, applications, component vulnerabilities, CVE lookup, compare-cves, compare-versions, list-versions).

### CVE Lookup

- `-cve <cve-id>`: CVE ID to lookup (e.g., CVE-2025-15467). CSV includes CVEs fixed in latest version and percent fixed.

### Component Operations

- `-component <component-id>`: Component ID for various operations
- `-severity <severities>`: Comma-separated severity filter (values: HIGH, CRITICAL)
- `-triage <statuses>`: Comma-separated triage filter. Allowed values: **IN_TRIAGE**, **NOT_AFFECTED**, **FALSE_POSITIVE**, **AFFECTED**.

### Version Management

- `-list-versions`: List all available versions of a component (requires `-component`)
- `-compare-cves`: Compare CVEs between current and latest patch version (requires `-component`)
- `-compare-versions`: Compare CVEs across versions (requires `-component`). With `-versions <list>` uses those versions; **without `-versions`**, compares the component's **current** version with the **latest** version.
- `-versions <versions>`: Comma-separated list of version numbers (e.g., "1.910,1.915,1.920")

### List commands (all support `-foundation` where applicable and `-csv`)

- `-list-foundations`: List all available foundations
- `-list-component-types`: List all available component types (optionally `-foundation <name>`)
- `-list-components`: List all components (optionally `-foundation <name>`, `-component-type <types>`)
- `-list-frameworks`: List all application frameworks (for use with `-applications`)

### VM operations (BOSH VMs)

- `-vm-list`: List BOSH VMs (optionally `-vm-foundation <bosh-director-id>` to filter)
- `-vm-lookup <ip>`: Look up BOSH VM by IP address (shows stemcell, component ID, and suggested `-component` command)

### Inventory and Applications

- `-inventory`: Generate component inventory report (optionally `-foundation <name>`, `-csv`)
- `-applications`: Query applications (use with `-framework <name>` and/or `-buildpack <component-id>`). Output includes Foundation, Current Version, Critical/High CVEs, Latest Patch Version, CVEs fixed by patch, Percent fixed.
- `-foundation <name>`: Filter by foundation name (inventory, list-components, list-component-types)

## Examples

### Complete Workflow: Analyzing a Component

1. **Find components in a foundation**:
   ```bash
   ./cve-lookup -list-components -foundation "opsman.elasticsky.cloud"
   ```

2. **List all versions of a component**:
   ```bash
   ./cve-lookup -list-versions -component "vrn/provider:TAS/instance:p-bosh-d882816bed6e9c00f303/Stemcell:bosh-vsphere-esxi-ubuntu-jammy-go_agent-1.915"
   ```

3. **Compare current vs latest patch**:
   ```bash
   ./cve-lookup -compare-cves -component "vrn/provider:TAS/instance:p-bosh-d882816bed6e9c00f303/Stemcell:bosh-vsphere-esxi-ubuntu-jammy-go_agent-1.915"
   ```

4. **Compare multiple versions**:
   ```bash
   ./cve-lookup -compare-versions -component "vrn/provider:TAS/instance:p-bosh-d882816bed6e9c00f303/Stemcell:bosh-vsphere-esxi-ubuntu-jammy-go_agent-1.915" -versions "1.910,1.915,1.920"
   ```

### Security Assessment Workflow

1. **Check for a specific CVE across infrastructure**:
   ```bash
   ./cve-lookup -cve CVE-2025-15467
   ```

2. **Get all vulnerabilities for a component**:
   ```bash
   ./cve-lookup -component "vrn/provider:TAS/instance:p-bosh-d882816bed6e9c00f303/Stemcell:bosh-vsphere-esxi-ubuntu-jammy-go_agent-1.915" -severity HIGH,CRITICAL
   ```

3. **Generate inventory report**:
   ```bash
   ./cve-lookup -inventory --csv > inventory.csv
   ```

## Piping CSV output into shell commands

All commands that support `-csv` write a header row and comma-separated columns. You can pipe that output into standard shell tools.

### Extract CVE IDs from component vulnerabilities

Component CSV has columns: Vulnerability ID, CVE ID, Severity, Triage Status, Patch Status, Artifact, URL. To get CVE IDs (one per line, skip header), use a CSV-aware tool so quoted fields are handled correctly. Example with a stemcell component that may have vulnerabilities:

```bash
./cve-lookup --component "vrn/provider:TAS/instance:p-bosh-d882816bed6e9c00f303/Stemcell:bosh-vsphere-esxi-ubuntu-jammy-go_agent-1.915" --csv | tail -n +2 | cut -d',' -f2
```

To get unique CVE IDs (again use a component ID from your environment, e.g. from `-list-components --csv`):

```bash
./cve-lookup --component "vrn/provider:TAS/instance:p-bosh-d882816bed6e9c00f303/Stemcell:bosh-vsphere-esxi-ubuntu-jammy-go_agent-1.915" --csv | tail -n +2 | cut -d',' -f2 | sort -u
```

### Extract application names and foundation from applications CSV

Applications CSV columns: Application Name (1), Entity ID (2), Provider (3), Framework (4), Related Entities Count (5), Foundation (6), then version/CVE columns. To list application names for a framework:

```bash
./cve-lookup --applications --framework Ruby --csv | tail -n +2 | cut -d',' -f1
```

To list **application name and foundation** (columns 1 and 6):

```bash
./cve-lookup --applications --framework Ruby --csv | tail -n +2 | cut -d',' -f1,7
```

### Other pipe use-cases

- **Foundations with most components**:  
  `./cve-lookup -list-components --csv | tail -n +2 | cut -d',' -f1 | sort | uniq -c | sort -rn`
- **Components with Critical CVEs**:  
  `./cve-lookup -list-components --csv | awk -F',' 'NR>1 && $6+0>0 {print $2","$6}'`
- **Inventory filtered by foundation, then open in Excel**:  
  `./cve-lookup -inventory --foundation "opsman.example.com" --csv > inv.csv && open inv.csv`

## Technical Details

### Query Counting

Most operations display the number of queries executed. This helps understand API usage and performance.

### Pagination

The tool automatically handles pagination for all queries, ensuring complete results regardless of result set size.

### Data Structures

- **Component ID**: Unique identifier for a component (e.g., `vrn/provider:TAS/instance:...`)
- **Version Number**: Semantic version string (e.g., `1.915`)
- **Version ID**: Internal database ID for a specific component version instance

## Testing

### Go integration tests

From the `scripts/Hub API` directory, with `hub-api-config.json` in place and network access to the Hub API:

```bash
go build -o cve-lookup .
go test -v -count=1
```

Tests require valid credentials and will fail if the config is missing or the API is unreachable. Use `-v` to see each command run and output size.

### Quick smoke test (manual)

Run these from the same directory to verify main flows. Replace foundation/component IDs with values from your environment (e.g. from `-list-foundations --csv` and `-list-components --csv`).

```bash
# Usage
./cve-lookup

# List commands (CSV)
./cve-lookup -list-foundations --csv | head -5
./cve-lookup -list-frameworks --csv | head -5
./cve-lookup -list-component-types --csv | head -5
./cve-lookup -list-components -foundation "opsman.elasticsky.cloud" --csv | head -5

# Applications (pretty and CSV; columns use N/A when no component data)
./cve-lookup -applications -framework Ruby | head -40
./cve-lookup -applications -framework Ruby --csv | head -5

# CVE lookup (replace CVE ID if needed)
./cve-lookup -cve CVE-2025-61723 2>&1 | head -60

# Component vulnerabilities (use a component ID from -list-components; must resolve to entity ID)
./cve-lookup -component "vrn/provider:TAS/instance:p-bosh-d882816bed6e9c00f303/Stemcell:bosh-vsphere-esxi-ubuntu-jammy-go_agent-1.915" 2>&1 | head -50

# VM list (and filter by BOSH director)
./cve-lookup -vm-list | head -20
./cve-lookup -vm-list -vm-foundation "p-bosh-d882816bed6e9c00f303" | head -20

# Inventory
./cve-lookup -inventory --csv | head -5
```

If `-vm-lookup <ip>` or token generation fails with "Connection refused", check `hub-api-config.json` and network connectivity to the GraphQL endpoint.
