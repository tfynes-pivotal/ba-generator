# BA Generator - Hub API CLI

Connects to a Tanzu Hub instance, scrapes all TAS spaces across every attached
foundation, and automatically generates **business applications** in Hub based on
a configurable regex pattern.

The default pattern is `ad\d{8}` (the literal string `ad` followed by exactly
eight digits).  Every space whose name contains such a substring is grouped by
that identifier and upserted as a business application.  An optional CSV
mapping file enriches each identifier with a human-readable name.

> [!WARNING]
> **This repository is an unofficial project provided "as is."**
>
> It is not supported by any organisation, and no warranty or guarantee of
> functionality is provided. Use at your own discretion.

## How it works

1. Authenticates to Hub via OAuth.
2. Paginates through every `Tanzu.TAS.Space` entity across all attached foundations.
3. For each space whose name matches the regex, extracts the identifier (e.g. `ad12345678`).
4. Groups spaces by identifier, collecting their `PotentialBusinessApplication`
   (PBA) entity IDs.
5. Optionally looks up a human-readable name for each identifier in a CSV file.
6. Calls `upsertBusinessApplications` for each group, creating or updating the
   corresponding business application in Hub.

## Installation

```bash
go build -o ba-generator .
```

## Authentication

On first run the tool creates a `hub-api-config.json` template in the working
directory and exits with instructions:

```
Authentication configuration file not found.
Created template at: /path/to/hub-api-config.json

Please fill in the following fields:
  - oauthAppId:      Your OAuth application ID
  - oauthAppSecret:  Your OAuth application secret
  - graphqlEndpoint: Hub GraphQL URL (e.g. https://hub.example.com/hub/graphql)
```

Edit the file and fill in all three fields, then run the tool again.

```json
{
  "oauthAppId": "your-oauth-app-id",
  "oauthAppSecret": "your-oauth-app-secret",
  "graphqlEndpoint": "https://hub.example.com/hub/graphql"
}
```

`hub-api-config.json` is listed in `.gitignore` — never commit it.

## Usage

```
BA Generator — auto-create Hub business applications from TAS space names

Usage:
  ba-generator -list-spaces
  ba-generator -generate [-dry-run] [-csv-map <file>] [-regex <pattern>]
  ba-generator -generate-token
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `-generate` | — | Generate business applications from matching TAS spaces |
| `-dry-run` | — | Print what would be created without executing any mutations (use with `-generate`) |
| `-list-spaces` | — | List all TAS spaces whose names match the regex |
| `-regex` | `ad\d{8}` | Regex pattern matched against space names |
| `-csv-map` | — | CSV file mapping identifiers to BA names (`ad_Id`,`ad_Name`) |
| `-generate-token` | — | Generate and print an OAuth token, then exit |
| `-endpoint` | — | Override the GraphQL endpoint from config |
| `-token` | — | Provide a pre-generated OAuth token (skips generation) |
| `-page-size` | `1000` | Spaces fetched per API request |

## Workflow

### 1 — Preview matching spaces

```bash
./ba-generator -list-spaces
```

Fetches all spaces and prints those whose names contain an `ad\d{8}` identifier,
grouped by identifier with their linked PBA entity IDs.

### 2 — Dry run

```bash
./ba-generator -generate -dry-run
```

Shows exactly which `upsertBusinessApplications` calls would be made — no
mutations are executed.

### 3 — Generate with name enrichment

Prepare a CSV file (e.g. `ad-id2name.csv`):

```csv
ad_Id,ad_Name
ad12345678,Payments Platform
ad87654321,Identity Service
```

Then run:

```bash
./ba-generator -generate -csv-map ad-id2name.csv
```

Each business application will be created with the mapped name.  If a mapping
is not found for a particular identifier, the raw identifier is used as the
name.

### 4 — Custom regex

```bash
./ba-generator -generate -regex 'app[0-9]{6}' -csv-map map.csv
```

### 5 — Token generation

```bash
./ba-generator -generate-token
```

Prints the raw OAuth bearer token and exits — useful for scripting or manual
API calls.

## Output

```
Fetching TAS spaces from Hub (https://hub.example.com/hub/graphql)...
Fetched 3420 spaces in 4 API request(s).

Matched 87 space(s) across 23 unique AD identifier(s) using pattern "ad\d{8}".

AD ID: ad12345678     BA Name: Payments Platform               Spaces: 4  PBA IDs: 4
    - org-ad12345678-dev
    - org-ad12345678-staging
    - org-ad12345678-prod
    - shared-ad12345678-sandbox
AD ID: ad87654321     BA Name: Identity Service                Spaces: 2  PBA IDs: 2
    ...

[OK]    ad12345678 (Payments Platform): entityId=ba-uuid-...
[OK]    ad87654321 (Identity Service): entityId=ba-uuid-...

Done. 23 created/updated, 0 skipped (no PBA IDs), 0 failed.
```

## Testing

```bash
go build -o ba-generator .
go test -v -count=1
```

Integration tests (those prefixed `Test*` that call Hub) require
`hub-api-config.json` with valid credentials and network access.  They are
skipped automatically when the config is absent.

Unit tests (`TestExtractADID`, `TestLoadADNameMap`) run without any external
connectivity.
