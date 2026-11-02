// BA Deleter - Hub API CLI
//
// Companion to ba-generator. Deletes Tanzu Hub business applications selected
// either by explicit name or by a regular expression matched against the
// business application name.
//
// Safety:
//   - -dry-run lists what would be deleted without calling any mutation.
//   - Without -yes, a real run asks for interactive confirmation.
//
// This is an unofficial project provided "as is." It is not supported by any
// organisation, and no warranty or guarantee of functionality is provided.
// Use at your own discretion.

package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
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

// Configuration

const configFileName = "hub-api-config.json"
const defaultPageSize = 1000
const defaultBatchSize = 50

// AuthConfig holds the credentials needed to talk to Hub (same file as ba-generator).
type AuthConfig struct {
	OAuthAppID      string `json:"oauthAppId"`
	OAuthAppSecret  string `json:"oauthAppSecret"`
	GraphQLEndpoint string `json:"graphqlEndpoint"`
}

// loadAuthConfig reads hub-api-config.json from the working directory.
func loadAuthConfig() (*AuthConfig, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cannot get current directory: %v", err)
	}
	configPath := filepath.Join(cwd, configFileName)

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%s not found in %s (run ba-generator once to create a template, or create it with oauthAppId, oauthAppSecret, graphqlEndpoint)", configFileName, cwd)
		}
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	var config AuthConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %v", err)
	}
	if config.OAuthAppID == "" || config.OAuthAppSecret == "" || config.GraphQLEndpoint == "" {
		return nil, fmt.Errorf("oauthAppId, oauthAppSecret and graphqlEndpoint are all required in %s", configPath)
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
	Message    string                 `json:"message"`
	ErrorMsg   string                 `json:"errorMsg"`
	Extensions map[string]interface{} `json:"extensions"`
}

// Error returns the message plus any non-empty debugInfo Hub attached, since
// bare codes like BAD_REQUEST are otherwise hard to diagnose.
func (e GraphQLError) Error() string {
	msg := e.Message
	if e.ErrorMsg != "" {
		msg = e.ErrorMsg
	}
	if dbg, ok := e.Extensions["debugInfo"].(map[string]interface{}); ok && len(dbg) > 0 {
		if b, err := json.Marshal(dbg); err == nil {
			msg += " " + string(b)
		}
	}
	return msg
}

func newHTTPClient(skipTLS bool) *http.Client {
	if !skipTLS {
		return &http.Client{}
	}
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}
}

// HubClient handles authenticated GraphQL requests to Hub.
type HubClient struct {
	endpoint   string
	authToken  string
	httpClient *http.Client
	debug      bool
}

func NewHubClient(endpoint, authToken string, skipTLS, debug bool) *HubClient {
	return &HubClient{
		endpoint:   endpoint,
		authToken:  authToken,
		httpClient: newHTTPClient(skipTLS),
		debug:      debug,
	}
}

// GenerateAccessToken fetches a short-lived OAuth access token from Hub.
func GenerateAccessToken(httpClient *http.Client, endpoint, appID, appSecret string) (string, error) {
	query := fmt.Sprintf(`
		mutation oauth {
			authMutation {
				oAuthAppMutation {
					generateAccessTokenForOAuthApp(
						input: {oauthAppId: %q, oauthAppSecret: %q}
					) {
						accessToken
					}
				}
			}
		}`, appID, appSecret)

	reqBody, err := json.Marshal(GraphQLRequest{Query: query})
	if err != nil {
		return "", fmt.Errorf("error marshalling request: %v", err)
	}
	resp, err := httpClient.Post(endpoint, "application/json", bytes.NewBuffer(reqBody))
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

	am, _ := result.Data["authMutation"].(map[string]interface{})
	om, _ := am["oAuthAppMutation"].(map[string]interface{})
	gt, _ := om["generateAccessTokenForOAuthApp"].(map[string]interface{})
	token, ok := gt["accessToken"].(string)
	if !ok || token == "" {
		return "", fmt.Errorf("access token not found in response")
	}
	return token, nil
}

// ExecuteQuery fires a single GraphQL request and returns the parsed response.
func (c *HubClient) ExecuteQuery(query string, variables map[string]interface{}) (*GraphQLResponse, error) {
	reqBody, err := json.Marshal(GraphQLRequest{Query: query, Variables: variables})
	if err != nil {
		return nil, fmt.Errorf("error marshalling request: %v", err)
	}
	if c.debug {
		log.Printf("[DEBUG] GraphQL request to %s\n%s", c.endpoint, reqBody)
	}

	httpReq, err := http.NewRequest("POST", c.endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.authToken)
	httpReq.Header.Set("x-id-token", c.authToken)
	httpReq.Header.Set("x-graphql-client-name", "BA Deleter CLI")
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
	if c.debug {
		log.Printf("[DEBUG] GraphQL response (HTTP %d): %s", resp.StatusCode, body)
	}

	var result GraphQLResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("error parsing response (HTTP %d): %v", resp.StatusCode, err)
	}
	if len(result.Errors) > 0 {
		msgs := make([]string, len(result.Errors))
		for i, e := range result.Errors {
			msgs[i] = e.Error()
		}
		return nil, fmt.Errorf("GraphQL errors: %s", strings.Join(msgs, "; "))
	}
	return &result, nil
}

// Business applications

// BusinessApp is an existing Tanzu.Hub.BusinessApplication entity.
type BusinessApp struct {
	EntityID   string
	EntityName string
}

const listBAsQuery = `
query listBusinessApplications($first: Int!, $after: String) {
  entityQuery {
    queryEntities(
      entityType: "Tanzu.Hub.BusinessApplication"
      first: $first
      after: $after
    ) {
      pageInfo {
        hasNextPage
        endCursor
      }
      entities {
        entityId
        entityName
      }
    }
  }
}`

// ListAllBusinessApplications returns every BusinessApplication in Hub,
// following pagination cursors. Duplicate names are preserved.
func (c *HubClient) ListAllBusinessApplications(pageSize int) ([]BusinessApp, error) {
	vars := map[string]interface{}{"first": pageSize}
	var out []BusinessApp
	after := ""

	for {
		if after != "" {
			vars["after"] = after
		} else {
			delete(vars, "after")
		}

		resp, err := c.ExecuteQuery(listBAsQuery, vars)
		if err != nil {
			return nil, err
		}

		eq, _ := resp.Data["entityQuery"].(map[string]interface{})
		qe, _ := eq["queryEntities"].(map[string]interface{})
		entities, _ := qe["entities"].([]interface{})
		for _, e := range entities {
			if m, ok := e.(map[string]interface{}); ok {
				id, _ := m["entityId"].(string)
				name, _ := m["entityName"].(string)
				if id != "" {
					out = append(out, BusinessApp{EntityID: id, EntityName: name})
				}
			}
		}

		pageInfo, _ := qe["pageInfo"].(map[string]interface{})
		hasNext, _ := pageInfo["hasNextPage"].(bool)
		cursor, _ := pageInfo["endCursor"].(string)
		if !hasNext || cursor == "" {
			break
		}
		after = cursor
	}
	return out, nil
}

// DeleteBusinessApplications deletes the given BA entity IDs in a single
// mutation and returns one success flag per ID (in input order).
//
// IDs are embedded inline in the query document (not as variables) because
// Hub's EntityId custom scalar does not coerce correctly via variables.
func (c *HubClient) DeleteBusinessApplications(ids []string) ([]bool, error) {
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = fmt.Sprintf("%q", id)
	}
	mutation := fmt.Sprintf(`
mutation DeleteBusinessApps {
  businessAppMutation {
    deleteBusinessApplications(entityId: [%s])
  }
}`, strings.Join(quoted, ", "))

	resp, err := c.ExecuteQuery(mutation, nil)
	if err != nil {
		return nil, err
	}

	bm, _ := resp.Data["businessAppMutation"].(map[string]interface{})
	raw, _ := bm["deleteBusinessApplications"].([]interface{})
	results := make([]bool, len(ids))
	for i := range ids {
		if i < len(raw) {
			results[i], _ = raw[i].(bool)
		}
	}
	return results, nil
}

// Selection

// selectTargets picks the BAs to delete. Exactly one of names or re should be
// set. For names, matching is exact and case-sensitive; any requested name with
// no matching BA is returned in missing. For re, a BA is selected when the
// regex matches anywhere in its name (anchor with ^...$ for a full match).
// Results are sorted by name then ID.
func selectTargets(all []BusinessApp, names []string, re *regexp.Regexp) (targets []BusinessApp, missing []string) {
	if re != nil {
		for _, ba := range all {
			if re.MatchString(ba.EntityName) {
				targets = append(targets, ba)
			}
		}
	} else {
		wanted := make(map[string]bool, len(names))
		for _, n := range names {
			wanted[n] = true
		}
		found := make(map[string]bool)
		for _, ba := range all {
			if wanted[ba.EntityName] {
				targets = append(targets, ba)
				found[ba.EntityName] = true
			}
		}
		seen := make(map[string]bool)
		for _, n := range names {
			if !found[n] && !seen[n] {
				missing = append(missing, n)
				seen[n] = true
			}
		}
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].EntityName != targets[j].EntityName {
			return targets[i].EntityName < targets[j].EntityName
		}
		return targets[i].EntityID < targets[j].EntityID
	})
	return targets, missing
}

// parseNames splits comma-separated values and positional args into a clean,
// de-duplicated list of names (order preserved).
func parseNames(csv string, args []string) []string {
	var raw []string
	if csv != "" {
		raw = append(raw, strings.Split(csv, ",")...)
	}
	raw = append(raw, args...)

	var out []string
	seen := make(map[string]bool)
	for _, n := range raw {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// loadNamesFile reads one BA name per line; blank lines and # comments are ignored.
func loadNamesFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var names []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		names = append(names, line)
	}
	return names, sc.Err()
}

func confirm(n int) bool {
	fmt.Printf("About to permanently delete %d business application(s). Type 'yes' to continue: ", n)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line) == "yes"
}

// Main

func printUsage() {
	bin := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, "BA Deleter — delete Hub business applications by name or regex\n\n")
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  %s -list [-regex <pattern>]\n", bin)
	fmt.Fprintf(os.Stderr, "  %s [-dry-run] [-yes] -names \"BA One,BA Two\" [more names...]\n", bin)
	fmt.Fprintf(os.Stderr, "  %s [-dry-run] [-yes] -names-file <file>\n", bin)
	fmt.Fprintf(os.Stderr, "  %s [-dry-run] [-yes] -regex <pattern>\n", bin)
	fmt.Fprintf(os.Stderr, "\nFlags:\n")
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nConfig file: hub-api-config.json in the working directory (shared with ba-generator)\n")
	fmt.Fprintf(os.Stderr, "\nExamples:\n")
	fmt.Fprintf(os.Stderr, "  # Preview deleting every BA whose name is a raw AD identifier:\n")
	fmt.Fprintf(os.Stderr, "  %s -regex '^ad\\d{8}$' -dry-run\n\n", bin)
	fmt.Fprintf(os.Stderr, "  # Delete two specific BAs, without the confirmation prompt:\n")
	fmt.Fprintf(os.Stderr, "  %s -names \"Payments Platform,Identity Service\" -yes\n\n", bin)
}

func main() {
	var (
		namesCSV  string
		namesFile string
		regexStr  string
		listMode  bool
		dryRun    bool
		assumeYes bool
		endpoint  string
		tokenFlag string
		pageSize  int
		batchSize int
		insecure  bool
		debug     bool
	)

	flag.StringVar(&namesCSV, "names", "", "Comma-separated BA names to delete (exact, case-sensitive). Extra positional args are also treated as names")
	flag.StringVar(&namesFile, "names-file", "", "File of BA names to delete, one per line (# comments allowed)")
	flag.StringVar(&regexStr, "regex", "", "Delete every BA whose name matches this regex (unanchored; use ^...$ for full match)")
	flag.BoolVar(&listMode, "list", false, "List existing business applications (filtered by -regex if given) and exit")
	flag.BoolVar(&dryRun, "dry-run", false, "Show what would be deleted without calling the delete mutation")
	flag.BoolVar(&assumeYes, "yes", false, "Skip the interactive confirmation prompt")
	flag.StringVar(&endpoint, "endpoint", "", "GraphQL endpoint URL (overrides hub-api-config.json)")
	flag.StringVar(&tokenFlag, "token", "", "OAuth access token (skips token generation if provided)")
	flag.IntVar(&pageSize, "page-size", defaultPageSize, "Business applications fetched per API request")
	flag.IntVar(&batchSize, "batch-size", defaultBatchSize, "Business applications deleted per mutation call")
	flag.BoolVar(&insecure, "insecure", false, "Skip TLS certificate verification (useful for self-signed certs)")
	flag.BoolVar(&debug, "debug", false, "Emit detailed debug logs for each GraphQL request and response")
	flag.Usage = printUsage
	flag.Parse()

	names := parseNames(namesCSV, flag.Args())
	if namesFile != "" {
		fileNames, err := loadNamesFile(namesFile)
		if err != nil {
			log.Fatalf("Error reading names file: %v", err)
		}
		names = parseNames("", append(names, fileNames...))
	}

	var re *regexp.Regexp
	if regexStr != "" {
		var err error
		re, err = regexp.Compile(regexStr)
		if err != nil {
			log.Fatalf("Invalid regex %q: %v", regexStr, err)
		}
	}

	if !listMode {
		if len(names) == 0 && re == nil {
			printUsage()
			os.Exit(1)
		}
		if len(names) > 0 && re != nil {
			log.Fatalf("Use either explicit names (-names / -names-file / args) or -regex, not both")
		}
	}
	if batchSize < 1 {
		batchSize = 1
	}

	config, err := loadAuthConfig()
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}
	if endpoint != "" {
		config.GraphQLEndpoint = endpoint
	}

	authToken := tokenFlag
	if authToken == "" {
		authToken, err = GenerateAccessToken(newHTTPClient(insecure), config.GraphQLEndpoint, config.OAuthAppID, config.OAuthAppSecret)
		if err != nil {
			log.Fatalf("Token error: %v", err)
		}
	}
	client := NewHubClient(config.GraphQLEndpoint, authToken, insecure, debug)

	fmt.Printf("Fetching business applications from Hub (%s)...\n", config.GraphQLEndpoint)
	all, err := client.ListAllBusinessApplications(pageSize)
	if err != nil {
		log.Fatalf("Error fetching business applications: %v", err)
	}
	fmt.Printf("Found %d business application(s).\n\n", len(all))

	if listMode {
		filter := re
		if filter == nil {
			filter = regexp.MustCompile(``) // matches everything
		}
		shown, _ := selectTargets(all, nil, filter)
		for _, ba := range shown {
			fmt.Printf("%-50s  %s\n", ba.EntityName, ba.EntityID)
		}
		fmt.Printf("\n%d business application(s) listed.\n", len(shown))
		return
	}

	targets, missing := selectTargets(all, names, re)
	for _, n := range missing {
		fmt.Printf("[NOT FOUND] %s\n", n)
	}
	if len(missing) > 0 {
		fmt.Println()
	}

	if len(targets) == 0 {
		fmt.Println("No business applications matched — nothing to delete.")
		return
	}

	if re != nil {
		fmt.Printf("%d business application(s) match pattern %q:\n", len(targets), regexStr)
	} else {
		fmt.Printf("%d business application(s) match the supplied name(s):\n", len(targets))
	}
	for _, ba := range targets {
		fmt.Printf("    - %-50s  %s\n", ba.EntityName, ba.EntityID)
	}
	fmt.Println()

	if dryRun {
		fmt.Println("--- DRY RUN: no business applications were deleted ---")
		return
	}

	if !assumeYes && !confirm(len(targets)) {
		fmt.Println("Aborted — nothing deleted.")
		os.Exit(1)
	}

	deleted, failed := 0, 0
	report := func(ba BusinessApp, ok bool, err error) {
		switch {
		case err != nil:
			fmt.Printf("[ERROR]   %s (%s): %v\n", ba.EntityName, ba.EntityID, err)
			failed++
		case ok:
			fmt.Printf("[DELETED] %s (%s)\n", ba.EntityName, ba.EntityID)
			deleted++
		default:
			fmt.Printf("[FAILED]  %s (%s): Hub returned false\n", ba.EntityName, ba.EntityID)
			failed++
		}
	}

	for start := 0; start < len(targets); start += batchSize {
		end := start + batchSize
		if end > len(targets) {
			end = len(targets)
		}
		batch := targets[start:end]
		ids := make([]string, len(batch))
		for i, ba := range batch {
			ids[i] = ba.EntityID
		}

		deletedBefore := deleted
		results, err := client.DeleteBusinessApplications(ids)
		switch {
		case err == nil:
			for i, ba := range batch {
				report(ba, results[i], nil)
			}
		case len(batch) == 1:
			report(batch[0], false, err)
		default:
			// One bad ID fails the whole call, so retry one at a time to isolate it.
			fmt.Printf("[WARN]    batch of %d failed (%v); retrying individually\n", len(batch), err)
			for _, ba := range batch {
				r, err := client.DeleteBusinessApplications([]string{ba.EntityID})
				report(ba, err == nil && r[0], err)
			}
		}

		// If nothing at all has succeeded after the first batch, the problem is
		// systemic (permissions, API change) — stop instead of hammering Hub.
		if start == 0 && deleted == deletedBefore && len(targets) > len(batch) {
			fmt.Printf("\nFirst batch failed completely — aborting remaining %d. Re-run with -debug for full request/response detail.\n",
				len(targets)-len(batch))
			break
		}
	}

	fmt.Printf("\nDone. %d deleted, %d failed, %d name(s) not found.\n", deleted, failed, len(missing))
	if failed > 0 {
		os.Exit(2)
	}
}
