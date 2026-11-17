package cli_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/diasYuri/opera/internal/adapters/credentials"
	"github.com/diasYuri/opera/internal/modules/configuration"
	"github.com/diasYuri/opera/internal/modules/execution"
	operaext "github.com/diasYuri/opera/internal/modules/extensions"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func capabilitiesFixture(t *testing.T) *fixture {
	f := newFixture(t)
	workspace := filepath.Join(f.root, "workspace")
	if err := os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	f.app.Getwd = func() (string, error) { return workspace, nil }
	f.app.OpenStore = func(string, string) (credentials.Store, error) {
		t.Fatal("memory management read credentials")
		return nil, nil
	}
	return f
}
func executionResult(t *testing.T, text string) execution.Result {
	t.Helper()
	var result execution.Result
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		var event struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal("invalid JSONL", err, line)
		}
		if event.Type == "execution.result" {
			if err := json.Unmarshal(event.Data, &result); err != nil {
				t.Fatal(err)
			}
		}
	}
	return result
}
func approve(t *testing.T, f *fixture, result execution.Result) execution.Result {
	t.Helper()
	if result.Approval == nil {
		t.Fatal("approval missing", result)
	}
	f.out.Reset()
	f.err.Reset()
	if code := f.app.Run([]string{"resume", result.ID, "--approve", result.Approval.ID, "--format=jsonl"}); code != 0 {
		t.Fatalf("resume %d: %s stdout %s", code, f.err.String(), f.out.String())
	}
	return executionResult(t, f.out.String())
}
func TestMemoryManagementApprovalPersistenceAndForget(t *testing.T) {
	f := capabilitiesFixture(t)
	if code := f.app.Run([]string{"memory", "enable", "--format=jsonl"}); code != 3 {
		t.Fatalf("enable %d: %s stdout %s", code, f.err.String(), f.out.String())
	}
	approve(t, f, executionResult(t, f.out.String()))
	f.out.Reset()
	f.err.Reset()
	value := `{"type":"project_fact","scope":"workspace","summary":"test memory","content":"durable fact"}`
	if code := f.app.Run([]string{"memory", "save", value, "--format=jsonl"}); code != 3 {
		t.Fatalf("save %d: %s stdout %s", code, f.err.String(), f.out.String())
	}
	saved := approve(t, f, executionResult(t, f.out.String()))
	var body struct {
		ID string `json:"id"`
	}
	if json.Unmarshal([]byte(saved.Text), &body) != nil || body.ID == "" {
		t.Fatal("memory ID missing", saved.Text)
	}
	f.out.Reset()
	f.err.Reset()
	if code := f.app.Run([]string{"memory", "read", body.ID, "--format=jsonl"}); code != 0 || !strings.Contains(f.out.String(), "durable fact") {
		t.Fatalf("read %d: %s %s", code, f.err.String(), f.out.String())
	}
	f.out.Reset()
	f.err.Reset()
	if code := f.app.Run([]string{"memory", "forget", body.ID, "--format=jsonl"}); code != 3 {
		t.Fatalf("forget %d: %s stdout %s", code, f.err.String(), f.out.String())
	}
	approve(t, f, executionResult(t, f.out.String()))
	f.out.Reset()
	f.err.Reset()
	if code := f.app.Run([]string{"memory", "read", body.ID, "--format=jsonl"}); code == 0 {
		t.Fatal("forgotten memory still readable")
	}
}

type extensionSecrets struct{ value string }

func (s *extensionSecrets) Set(_ string, value string) error { s.value = value; return nil }
func (s *extensionSecrets) Get(string) (string, error)       { return s.value, nil }
func (s *extensionSecrets) Delete(string) error              { s.value = ""; return nil }
func TestMCPHTTPActivationToolsResourcesPromptsAndNoSecretLeak(t *testing.T) {
	var effects, requests atomic.Int32
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "fixture", Version: "1"}, nil)
	server.AddTool(&mcpsdk.Tool{Name: "mutate", InputSchema: json.RawMessage(`{"type":"object"}`)}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		effects.Add(1)
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "remote proof"}}}, nil
	})
	server.AddResource(&mcpsdk.Resource{Name: "data", URI: "test://fixture/data"}, func(_ context.Context, req *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
		return &mcpsdk.ReadResourceResult{Contents: []*mcpsdk.ResourceContents{{URI: req.Params.URI, Text: "resource proof"}}}, nil
	})
	server.AddPrompt(&mcpsdk.Prompt{Name: "greet"}, func(context.Context, *mcpsdk.GetPromptRequest) (*mcpsdk.GetPromptResult, error) {
		return &mcpsdk.GetPromptResult{Messages: []*mcpsdk.PromptMessage{{Role: "user", Content: &mcpsdk.TextContent{Text: "prompt proof"}}}}, nil
	})
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, nil)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		if req.Header.Get("Authorization") != "Bearer confidential-fixture-token" {
			t.Error("missing MCP reference credential")
		}
		handler.ServeHTTP(w, req)
	}))
	defer httpServer.Close()
	f := capabilitiesFixture(t)
	secrets := &extensionSecrets{value: "confidential-fixture-token"}
	f.app.OpenStore = func(string, string) (credentials.Store, error) { return secrets, nil }
	if code := f.app.Run([]string{"mcp", "add", "fixture", "streamable_http", httpServer.URL, "mcp-ref", "--format=jsonl"}); code != 0 {
		t.Fatal(code, f.err.String())
	}
	if requests.Load() != 0 {
		t.Fatal("adding disabled config started a server")
	}
	f.out.Reset()
	f.err.Reset()
	if code := f.app.Run([]string{"mcp", "enable", "fixture", "--format=jsonl"}); code != 3 {
		t.Fatal(code, f.err.String(), f.out.String())
	}
	if requests.Load() != 0 {
		t.Fatal("unapproved activation connected to MCP")
	}
	approve(t, f, executionResult(t, f.out.String()))
	if requests.Load() == 0 {
		t.Fatal("activation did not connect")
	}
	f.out.Reset()
	f.err.Reset()
	if code := f.app.Run([]string{"mcp", "call", "mcp.fixture.mutate", `{}`, "--format=jsonl"}); code != 3 {
		t.Fatal(code, f.err.String(), f.out.String())
	}
	if effects.Load() != 0 {
		t.Fatal("MCP mutated before operation approval")
	}
	result := approve(t, f, executionResult(t, f.out.String()))
	if effects.Load() != 1 || !strings.Contains(result.Text, "remote proof") {
		t.Fatal("MCP result missing", effects.Load(), result.Text)
	}
	if strings.Contains(f.out.String(), secrets.value) {
		t.Fatal("secret leaked to JSONL")
	}
	for tool, want := range map[string]string{"mcp.fixture.resources.read": "resource proof", "mcp.fixture.prompts.get": "prompt proof"} {
		args := `{"uri":"test://fixture/data"}`
		if strings.Contains(tool, "prompts") {
			args = `{"name":"greet"}`
		}
		f.out.Reset()
		f.err.Reset()
		if code := f.app.Run([]string{"mcp", "call", tool, args, "--policy=trusted", "--format=jsonl"}); code != 0 || !strings.Contains(f.out.String(), want) {
			t.Fatal(tool, code, f.err.String(), f.out.String())
		}
	}
}

func TestMCPStdioServerProcess(t *testing.T) {
	if len(os.Args) < 3 || os.Args[len(os.Args)-2] != "opera-mcp-server" {
		return
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		os.Exit(17)
	}
	marker := os.Args[len(os.Args)-1]
	file, err := os.OpenFile(marker, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(18)
	}
	_, _ = fmt.Fprintln(file, os.Getpid())
	_ = file.Close()
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "stdio", Version: "1"}, nil)
	server.AddTool(&mcpsdk.Tool{Name: "echo", InputSchema: json.RawMessage(`{"type":"object"}`)}, func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "stdio proof"}}}, nil
	})
	if err := server.Run(context.Background(), &mcpsdk.StdioTransport{}); err != nil {
		os.Exit(19)
	}
	os.Exit(0)
}
func TestMCPStdioConsentAndCleanup(t *testing.T) {
	f := capabilitiesFixture(t)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "pids")
	args := []string{"mcp", "add", "local", "stdio", binary, "--", "-test.run=^TestMCPStdioServerProcess$", "--", "opera-mcp-server", marker}
	if code := f.app.Run(args); code != 0 {
		t.Fatal(code, f.err.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("disabled stdio server started")
	}
	f.out.Reset()
	f.err.Reset()
	if code := f.app.Run([]string{"mcp", "enable", "local", "--policy=trusted", "--format=jsonl"}); code != 0 {
		t.Fatal(code, f.err.String(), f.out.String())
	}
	f.out.Reset()
	f.err.Reset()
	if code := f.app.Run([]string{"tools", "call", "mcp.local.echo", `{}`, "--policy=trusted", "--format=jsonl"}); code != 0 || !strings.Contains(f.out.String(), "stdio proof") {
		t.Fatal(code, f.err.String(), f.out.String())
	}
	// A second startup failure must close the already connected first process.
	workspace, err := f.app.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := f.app.Paths(workspace)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(config.Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := operaext.Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	broken := operaext.Server{Name: "broken", Transport: "stdio", Command: filepath.Join(f.root, "does-not-exist"), Enabled: true}
	broken.Consent = operaext.ServerDigest(broken)
	settings.MCP = append(settings.MCP, broken)
	if err := operaext.Save(cfg, settings); err != nil {
		t.Fatal(err)
	}
	f.out.Reset()
	f.err.Reset()
	if code := f.app.Run([]string{"tools", "call", "mcp.local.echo", `{}`, "--policy=trusted", "--format=jsonl"}); code == 0 {
		t.Fatal("partial startup succeeded")
	}
	assertMCPProcessesExited(t, marker)
}

func TestMCPActivationTimeoutLeavesDisabledConfiguration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(300 * time.Millisecond):
			http.Error(w, "fixture unavailable", http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	f := capabilitiesFixture(t)
	if code := f.app.Run([]string{"mcp", "add", "slow", "streamable_http", server.URL}); code != 0 {
		t.Fatal(code, f.err.String())
	}
	f.out.Reset()
	f.err.Reset()
	start := time.Now()
	code := f.app.Run([]string{"mcp", "enable", "slow", "--policy=trusted", "--set=limits.process_timeout=100ms", "--format=jsonl"})
	if code == 0 || time.Since(start) > 10*time.Second {
		t.Fatal("activation timeout failed", code, f.err.String())
	}
	f.out.Reset()
	f.err.Reset()
	if code := f.app.Run([]string{"mcp", "list", "--format=jsonl"}); code != 0 || strings.Contains(f.out.String(), `"Enabled":true`) {
		t.Fatal("failed activation committed", code, f.out.String())
	}
}
