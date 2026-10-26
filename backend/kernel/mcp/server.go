package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server speaks MCP for one API key. It calls the API over HTTP rather than
// dispatching in-process - that is the whole point: the agent is a CLIENT,
// and goes through the same middleware, the same `aud: api_key` claim, the
// same tenant scope and the same permission checks as any other machine
// caller. An in-process version would hand an agent more access than the
// credential it was given.
type Server struct {
	Tools   []Tool
	BaseURL string // https://api.example.com
	Key     string // the API key, sent as a bearer token
	Version string
	Client  *http.Client
}

// Build returns the SDK server with every tool installed. Run it over
// stdio with `Run`, or over an in-memory transport in a test.
func (s Server) Build() *sdk.Server {
	srv := sdk.NewServer(&sdk.Implementation{Name: "open-enu", Version: s.Version}, nil)
	for _, t := range s.Tools {
		tool := t
		srv.AddTool(&sdk.Tool{Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema},
			func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
				return s.call(ctx, tool, req.Params.Arguments)
			})
	}
	return srv
}

// Run serves one client over stdin and stdout until it disconnects.
func (s Server) Run(ctx context.Context) error {
	return s.Build().Run(ctx, &sdk.StdioTransport{})
}

// call makes the HTTP request a tool stands for. A 4xx comes back as tool
// content with IsError, not as a protocol error: the call itself worked, and
// the model needs to READ the refusal - "you lack example.manage" is the most
// useful sentence it can get.
func (s Server) call(ctx context.Context, tool Tool, raw json.RawMessage) (*sdk.CallToolResult, error) {
	args := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("arguments: %w", err)
		}
	}
	path := tool.Path
	for k, v := range args {
		switch v.(type) {
		case string, float64, bool:
			path = strings.ReplaceAll(path, "{"+k+"}", url.PathEscape(fmt.Sprint(v)))
		}
	}
	var body io.Reader
	if b, ok := args["body"].(map[string]any); ok {
		data, err := json.Marshal(b)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}
	target := strings.TrimRight(s.BaseURL, "/") + path
	if q, ok := args["query"].(map[string]any); ok && len(q) > 0 {
		values := url.Values{}
		for k, v := range q {
			values.Set(k, fmt.Sprint(v))
		}
		target += "?" + values.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, tool.Method, target, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.Key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	text, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return &sdk.CallToolResult{
		Content: []sdk.Content{&sdk.TextContent{Text: string(text)}},
		IsError: resp.StatusCode >= 400,
	}, nil
}
