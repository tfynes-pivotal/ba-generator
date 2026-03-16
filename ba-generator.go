// BA Generator - Hub API CLI
//
// Connects to a Tanzu Hub instance, scrapes all TAS spaces across attached
// foundations, and automatically generates business applications based on a
// configurable regex pattern (default: ad\d{8}).
//
// Spaces whose names contain a matching substring are grouped by that
// identifier and upserted into Hub as a business application.  An optional
// CSV mapping file enriches each AD identifier with a human-readable name.
//
// This is an unofficial project provided "as is." It is not supported by any
// organisation, and no warranty or guarantee of functionality is provided.
// Use at your own discretion.

package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Main entry points (CTRL+F to navigate):
//
//   main                          - flag parsing and dispatch
//   loadAuthConfig                - read hub-api-config.json
//   NewHubClient                  - build API client
//   GenerateAccessToken           - OAuth token
//   ExecuteQuery                  - single GraphQL request
//   ListAllSpaces                 - paginated fetch of all Tanzu.TAS.Space entities
//   UpsertBusinessApplication     - mutation to create/update a business application
//   loadADNameMap                 - read ad_Id -> ad_Name CSV
//   extractADID                   - pull the ad\d{8} substring from a space name

// Configuration

const configFileName = "hub-api-config.json"
const defaultRegex = `ad\d{8}`
const defaultPageSize = 1000

// AuthConfig holds the credentials needed to talk to Hub.
type AuthConfig struct {
	OAuthAppID      string `json:"oauthAppId"`
	OAuthAppSecret  string `json:"oauthAppSecret"`
	GraphQLEndpoint string `json:"graphqlEndpoint"`
}

// loadAuthConfig reads hub-api-config.json from the working directory.
// If the file does not exist, a template is written and the process exits with
// instructions for the user to fill it in.
func loadAuthConfig() (*AuthConfig, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cannot get current directory: %v", err)
	}
	configPath := filepath.Join(cwd, configFileName)

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		template := AuthConfig{}
		templateJSON, err := json.MarshalIndent(template, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("error creating template config: %v", err)
		}
		if err := os.WriteFile(configPath, templateJSON, 0600); err != nil {
			return nil, fmt.Errorf("error writing config file: %v", err)
		}
		fmt.Fprintf(os.Stderr, "Authentication configuration file not found.\n")
		fmt.Fprintf(os.Stderr, "Created template at: %s\n\n", configPath)
		fmt.Fprintf(os.Stderr, "Please fill in the following fields:\n")
		fmt.Fprintf(os.Stderr, "  - oauthAppId:      Your OAuth application ID\n")
		fmt.Fprintf(os.Stderr, "  - oauthAppSecret:  Your OAuth application secret\n")
		fmt.Fprintf(os.Stderr, "  - graphqlEndpoint: Hub GraphQL URL (e.g. https://hub.example.com/hub/graphql)\n\n")
		fmt.Fprintf(os.Stderr, "After filling it in, run the command again.\n\n")
		os.Exit(1)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	var config AuthConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %v", err)
	}

	if config.OAuthAppID == "" {
		return nil, fmt.Errorf("oauthAppId is required in %s", configPath)
	}
	if config.OAuthAppSecret == "" {
		return nil, fmt.Errorf("oauthAppSecret is required in %s", configPath)
	}
	if config.GraphQLEndpoint == "" {
		return nil, fmt.Errorf("graphqlEndpoint is required in %s", configPath)
	}

	return &config, nil
}

// GraphQL plumbing

type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type GraphQLResponse struct {
	Data   map[string]interface{} `json:"data"`
	Errors []GraphQLError         `json:"errors,omitempty"`
}

type GraphQLError struct {
	Message string `json:"message"`
}

// HubClient handles authenticated GraphQL requests to Hub.
type HubClient struct {
	endpoint   string
	authToken  string
	httpClient *http.Client
}

// NewHubClient constructs a HubClient for the given endpoint and bearer token.
func NewHubClient(endpoint, authToken string) *HubClient {
	return &HubClient{
		endpoint:   endpoint,
		authToken:  authToken,
		httpClient: &http.Client{},
	}
}

// GenerateAccessToken fetches a short-lived OAuth access token from Hub.
func GenerateAccessToken(endpoint, appID, appSecret string) (string, error) {
	query := fmt.Sprintf(`
		mutation oauth {
			authMutation {
				oAuthAppMutation {
					generateAccessTokenForOAuthApp(
						input: {oauthAppId: "%s", oauthAppSecret: "%s"}
					) {
						accessToken
					}
				}
			}
		}`, appID, appSecret)

	req := GraphQLRequest{Query: query}
	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("error marshalling request: %v", err)
	}

	resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %v", err)
	}

	var result GraphQLResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("error parsing response: %v", err)
	}
	if len(result.Errors) > 0 {
		return "", fmt.Errorf("GraphQL errors: %v", result.Errors)
	}

	authMutation, ok := result.Data["authMutation"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected response structure")
	}
	oAuthAppMutation, ok := authMutation["oAuthAppMutation"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected response structure")
	}
	generateAccessToken, ok := oAuthAppMutation["generateAccessTokenForOAuthApp"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected response structure")
	}
	accessToken, ok := generateAccessToken["accessToken"].(string)
	if !ok {
		return "", fmt.Errorf("access token not found in response")
	}
	return accessToken, nil
}

// ExecuteQuery fires a single GraphQL request and returns the parsed response.
func (c *HubClient) ExecuteQuery(query string, variables map[string]interface{}) (*GraphQLResponse, error) {
	req := GraphQLRequest{
		Query:     query,
		Variables: variables,
	}
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshalling request: %v", err)
	}

	httpReq, err := http.NewRequest("POST", c.endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.authToken)
	httpReq.Header.Set("x-id-token", c.authToken)
	httpReq.Header.Set("x-graphql-client-name", "BA Generator CLI")
	httpReq.Header.Set("x-graphql-client-version", "1.0.0")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %v", err)
	}

	var result GraphQLResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("error parsing response: %v", err)
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL errors: %v", result.Errors)
	}
	return &result, nil
}

// Domain types

// TASSpace represents a single TAS space entity returned from Hub, together
// with the PotentialBusinessApplication entity IDs linked to it.
type TASSpace struct {
	EntityID   string
	EntityName string
	PBAIDs     []string // PotentialBusinessApplication entity IDs
}

// BAUpsertResult carries the outcome of a single upsertBusinessApplications call.
type BAUpsertResult struct {
	EntityID string
	ErrorMsg string
}

// Space query

const listSpacesQuery = `
query listTASSpaces($first: Int!, $after: String) {
  entityQuery {
    queryEntities(
      entityType: "Tanzu.TAS.Space"
      first: $first
      after: $after
    ) {
      count
      totalCount
      pageInfo {
        hasNextPage
        endCursor
      }
      entities {
        entityId
        entityName
        entitiesIn(entityType: "Tanzu.Hub.PotentialBusinessApplication") {
          entities {
            entityId
            entityName
          }
        }
      }
    }
  }
}`

// ListAllSpaces fetches every TAS space from Hub, following pagination cursors.
// queryCount is incremented by one per API request if non-nil.
func (c *HubClient) ListAllSpaces(pageSize int, queryCount *int) ([]TASSpace, error) {
	vars := map[string]interface{}{
		"first": pageSize,
	}
	var out []TASSpace
	after := ""

	for {
		if after != "" {
			vars["after"] = after
		} else {
			delete(vars, "after")
		}

		resp, err := c.ExecuteQuery(listSpacesQuery, vars)
		if queryCount != nil {
			*queryCount++
		}
		if err != nil {
			return nil, err
		}

		eq, _ := resp.Data["entityQuery"].(map[string]interface{})
		qe, _ := eq["queryEntities"].(map[string]interface{})
		entities, _ := qe["entities"].([]interface{})

		for _, e := range entities {
			if space := parseTASSpace(e); space != nil {
				out = append(out, *space)
			}
		}

		pageInfo, _ := qe["pageInfo"].(map[string]interface{})
		hasNext, _ := pageInfo["hasNextPage"].(bool)
		if !hasNext {
			break
		}
		cursor, _ := pageInfo["endCursor"].(string)
		if cursor == "" {
			break
		}
		after = cursor
	}
	return out, nil
}

func parseTASSpace(e interface{}) *TASSpace {
	m, ok := e.(map[string]interface{})
	if !ok {
		return nil
	}
	space := &TASSpace{
		EntityID:   getStr(m, "entityId"),
		EntityName: getStr(m, "entityName"),
	}
	if entitiesIn, ok := m["entitiesIn"].(map[string]interface{}); ok {
		if entities, ok := entitiesIn["entities"].([]interface{}); ok {
			for _, pba := range entities {
				if pm, ok := pba.(map[string]interface{}); ok {
					if id := getStr(pm, "entityId"); id != "" {
						space.PBAIDs = append(space.PBAIDs, id)
					}
				}
			}
		}
	}
	return space
}

func getStr(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// Business application mutation

const upsertBAMutation = `
mutation UpsertBusinessApps($entityName: String!, $potentialBusinessApps: [EntityId!]!) {
  businessAppMutation {
    upsertBusinessApplications(
      input: [{
        entityName: $entityName
        potentialBusinessApps: $potentialBusinessApps
      }]
    ) {
      entities { entityId }
      errors { entityId entityName errorMsg errorType }
    }
  }
}`

// UpsertBusinessApplication creates or updates a business application in Hub.
// name is the display name for the BA; pbaIDs are the PotentialBusinessApplication
// entity IDs that should be linked to it.
func (c *HubClient) UpsertBusinessApplication(name string, pbaIDs []string) (*BAUpsertResult, error) {
	vars := map[string]interface{}{
		"entityName":            name,
		"potentialBusinessApps": pbaIDs,
	}
	resp, err := c.ExecuteQuery(upsertBAMutation, vars)
	if err != nil {
		return nil, err
	}

	result := &BAUpsertResult{}

	bm, _ := resp.Data["businessAppMutation"].(map[string]interface{})
	upsert, _ := bm["upsertBusinessApplications"].(map[string]interface{})

	if entities, ok := upsert["entities"].([]interface{}); ok && len(entities) > 0 {
		if e, ok := entities[0].(map[string]interface{}); ok {
			result.EntityID = getStr(e, "entityId")
		}
	}
	if errors, ok := upsert["errors"].([]interface{}); ok && len(errors) > 0 {
		if e, ok := errors[0].(map[string]interface{}); ok {
			result.ErrorMsg = getStr(e, "errorMsg")
		}
	}
	return result, nil
}

// CSV name mapping

// loadADNameMap reads a two-column CSV (header: ad_Id, ad_Name) and returns a
// map from each AD identifier to its human-readable business application name.
func loadADNameMap(csvPath string) (map[string]string, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}

	m := make(map[string]string)
	for i, row := range records {
		if i == 0 {
			continue // skip header
		}
		if len(row) >= 2 {
			m[strings.TrimSpace(row[0])] = strings.TrimSpace(row[1])
		}
	}
	return m, nil
}

// extractADID returns the first substring matching re from s, or "" if none.
func extractADID(s string, re *regexp.Regexp) string {
	return re.FindString(s)
}

// Main

func printUsage() {
	bin := os.Args[0]
	fmt.Fprintf(os.Stderr, "BA Generator — auto-create Hub business applications from TAS space names\n\n")
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  %s -list-spaces\n", bin)
	fmt.Fprintf(os.Stderr, "  %s -generate [-dry-run] [-csv-map <file>] [-regex <pattern>]\n", bin)
	fmt.Fprintf(os.Stderr, "  %s -generate-token\n", bin)
	fmt.Fprintf(os.Stderr, "\nFlags:\n")
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nConfig file: hub-api-config.json (auto-created on first run)\n")
	fmt.Fprintf(os.Stderr, "\nExamples:\n")
	fmt.Fprintf(os.Stderr, "  # List all spaces that contain an AD identifier:\n")
	fmt.Fprintf(os.Stderr, "  %s -list-spaces\n\n", bin)
	fmt.Fprintf(os.Stderr, "  # Dry-run generation (no mutations):\n")
	fmt.Fprintf(os.Stderr, "  %s -generate -dry-run\n\n", bin)
	fmt.Fprintf(os.Stderr, "  # Generate with a name-mapping CSV:\n")
	fmt.Fprintf(os.Stderr, "  %s -generate -csv-map ad-id2name.csv\n\n", bin)
	fmt.Fprintf(os.Stderr, "  # Custom regex pattern:\n")
	fmt.Fprintf(os.Stderr, "  %s -generate -regex 'app[0-9]{6}' -csv-map map.csv\n\n", bin)
}

func main() {
	var (
		generateMode  bool
		dryRun        bool
		listSpaces    bool
		regexPattern  string
		csvMapPath    string
		generateToken bool
		endpointFlag  string
		tokenFlag     string
		pageSize      int
	)

	flag.BoolVar(&generateMode, "generate", false, "Generate business applications from matching TAS spaces")
	flag.BoolVar(&dryRun, "dry-run", false, "Show what would be created without calling the mutation (use with -generate)")
	flag.BoolVar(&listSpaces, "list-spaces", false, "List all TAS spaces whose names match the regex pattern")
	flag.StringVar(&regexPattern, "regex", defaultRegex, "Regex pattern to match against space names")
	flag.StringVar(&csvMapPath, "csv-map", "", "CSV file mapping AD IDs to BA names (columns: ad_Id,ad_Name)")
	flag.BoolVar(&generateToken, "generate-token", false, "Generate and print an OAuth access token, then exit")
	flag.StringVar(&endpointFlag, "endpoint", "", "GraphQL endpoint URL (overrides hub-api-config.json)")
	flag.StringVar(&tokenFlag, "token", "", "OAuth access token (skips token generation if provided)")
	flag.IntVar(&pageSize, "page-size", defaultPageSize, "Spaces fetched per API request")
	flag.Parse()

	if !generateMode && !listSpaces && !generateToken {
		printUsage()
		os.Exit(1)
	}

	// Load auth config
	config, err := loadAuthConfig()
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}
	if endpointFlag != "" {
		config.GraphQLEndpoint = endpointFlag
	}

	// Resolve auth token
	authToken := tokenFlag
	if authToken == "" {
		authToken, err = GenerateAccessToken(config.GraphQLEndpoint, config.OAuthAppID, config.OAuthAppSecret)
		if err != nil {
			log.Fatalf("Token error: %v", err)
		}
	}

	if generateToken {
		fmt.Println(authToken)
		return
	}

	client := NewHubClient(config.GraphQLEndpoint, authToken)

	// Compile regex
	re, err := regexp.Compile(regexPattern)
	if err != nil {
		log.Fatalf("Invalid regex %q: %v", regexPattern, err)
	}

	// Load optional name-mapping CSV
	adNameMap := make(map[string]string)
	if csvMapPath != "" {
		adNameMap, err = loadADNameMap(csvMapPath)
		if err != nil {
			log.Fatalf("Error loading CSV map: %v", err)
		}
		fmt.Printf("Loaded %d AD ID→name mappings from %s\n", len(adNameMap), csvMapPath)
	}

	// Fetch all TAS spaces (paginated)
	var queryCount int
	fmt.Printf("Fetching TAS spaces from Hub (%s)...\n", config.GraphQLEndpoint)
	spaces, err := client.ListAllSpaces(pageSize, &queryCount)
	if err != nil {
		log.Fatalf("Error fetching spaces: %v", err)
	}
	fmt.Printf("Fetched %d spaces in %d API request(s).\n\n", len(spaces), queryCount)

	// Group matching spaces by extracted AD identifier
	type BAGroup struct {
		ADID   string
		BAName string
		Spaces []string
		PBAIDs []string
	}

	groups := make(map[string]*BAGroup)
	matchCount := 0

	for _, space := range spaces {
		adID := extractADID(space.EntityName, re)
		if adID == "" {
			continue
		}
		matchCount++

		g, exists := groups[adID]
		if !exists {
			baName := adID
			if mapped, ok := adNameMap[adID]; ok {
				baName = mapped
			}
			g = &BAGroup{ADID: adID, BAName: baName}
			groups[adID] = g
		}

		g.Spaces = append(g.Spaces, space.EntityName)

		// Collect PBA IDs, deduplicating
		for _, pbaID := range space.PBAIDs {
			duplicate := false
			for _, existing := range g.PBAIDs {
				if existing == pbaID {
					duplicate = true
					break
				}
			}
			if !duplicate {
				g.PBAIDs = append(g.PBAIDs, pbaID)
			}
		}
	}

	// Sort groups for deterministic output
	adIDs := make([]string, 0, len(groups))
	for adID := range groups {
		adIDs = append(adIDs, adID)
	}
	sort.Strings(adIDs)

	fmt.Printf("Matched %d space(s) across %d unique AD identifier(s) using pattern %q.\n\n",
		matchCount, len(groups), regexPattern)

	if listSpaces || generateMode {
		for _, adID := range adIDs {
			g := groups[adID]
			fmt.Printf("AD ID: %-14s  BA Name: %-40s  Spaces: %d  PBA IDs: %d\n",
				g.ADID, g.BAName, len(g.Spaces), len(g.PBAIDs))
			for _, s := range g.Spaces {
				fmt.Printf("    - %s\n", s)
			}
		}
		fmt.Println()
	}

	if !generateMode {
		return
	}

	// Execute upsert mutations (or dry-run)
	if dryRun {
		fmt.Println("--- DRY RUN: no mutations will be executed ---")
		for _, adID := range adIDs {
			g := groups[adID]
			fmt.Printf("[DRY RUN] upsertBusinessApplications  name=%q  pbaIDs=%v\n",
				g.BAName, g.PBAIDs)
		}
		return
	}

	successCount := 0
	skipCount := 0
	failCount := 0

	for _, adID := range adIDs {
		g := groups[adID]
		if len(g.PBAIDs) == 0 {
			fmt.Printf("[SKIP]  %s (%s): no PotentialBusinessApplication IDs found for this group\n",
				adID, g.BAName)
			skipCount++
			continue
		}

		result, err := client.UpsertBusinessApplication(g.BAName, g.PBAIDs)
		if err != nil {
			fmt.Printf("[ERROR] %s (%s): %v\n", adID, g.BAName, err)
			failCount++
			continue
		}
		if result.ErrorMsg != "" {
			fmt.Printf("[ERROR] %s (%s): %s\n", adID, g.BAName, result.ErrorMsg)
			failCount++
		} else {
			fmt.Printf("[OK]    %s (%s): entityId=%s\n", adID, g.BAName, result.EntityID)
			successCount++
		}
	}

	fmt.Printf("\nDone. %d created/updated, %d skipped (no PBA IDs), %d failed.\n",
		successCount, skipCount, failCount)
}
