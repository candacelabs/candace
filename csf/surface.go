package csf

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/candacelabs/candace/pkg/liquidproto"
	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const maxAPIBytes = 2 * 1024 * 1024

// ErrInvalidRequest marks caller input rejected before an operation can succeed.
// Backend and encoding failures remain server errors at the HTTP boundary.
var ErrInvalidRequest = errors.New("invalid request")

// IHTTPDoer is the only network behavior the generated client consumes.
type IHTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

// Client uses generated method signatures with protobuf's JSON codec.
type Client struct {
	endpoint string
	http     IHTTPDoer
}

func NewClient(endpoint string, transport IHTTPDoer) (*Client, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || transport == nil {
		return nil, fmt.Errorf("an HTTP(S) endpoint and transport are required")
	}
	return &Client{endpoint: strings.TrimRight(endpoint, "/"), http: transport}, nil
}

func (client *Client) call(ctx context.Context, method string, path string, input proto.Message, output proto.Message) error {
	encoded, err := protojson.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, method, client.endpoint+path, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxAPIBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxAPIBytes {
		return fmt.Errorf("response exceeds limit")
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("adapter returned HTTP %d: %s", response.StatusCode, string(body))
	}
	return protojson.Unmarshal(body, output)
}

// Surface mounts into an existing Gin engine and exposes the same capabilities
// through an SDK-owned MCP server. It opens no listener and owns no process.
type Surface struct {
	MCP    *mcp.Server
	routes []route
}
type route struct {
	method  string
	path    string
	handler gin.HandlerFunc
}

func newSurface() *Surface {
	return &Surface{MCP: mcp.NewServer(&mcp.Implementation{Name: "candace-brain-spine", Version: "0.1.0"}, nil)}
}

func (surface *Surface) Register(router gin.IRouter) {
	for _, route := range surface.routes {
		router.Handle(route.method, route.path, route.handler)
	}
}

// Erasure is contained in this transport adapter. Capability authors consume
// generated protobuf types; MCP's SDK owns the heterogeneous tool collection.
func registerOperation[Q proto.Message, R proto.Message](surface *Surface, name string, method string, path string, humanDescription string, schema json.RawMessage, construct func() Q, call func(ctx context.Context, request Q) (R, error)) {
	invoke := func(ctx context.Context, raw []byte) ([]byte, error) {
		if len(raw) > maxAPIBytes {
			return nil, fmt.Errorf("%w: request exceeds limit", ErrInvalidRequest)
		}
		request := construct()
		if len(raw) == 0 {
			raw = []byte("{}")
		}
		if err := protojson.Unmarshal(raw, request); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalidRequest, err)
		}
		result, err := call(ctx, request)
		if err != nil {
			return nil, err
		}
		return protojson.Marshal(result)
	}
	surface.routes = append(surface.routes, route{method: method, path: path, handler: func(ctx *gin.Context) {
		raw, err := io.ReadAll(http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxAPIBytes))
		if err != nil {
			ctx.String(http.StatusBadRequest, "request exceeds limit")
			return
		}
		result, err := invoke(ctx.Request.Context(), raw)
		if err != nil {
			status := http.StatusInternalServerError
			var validation *liquidproto.Error
			if errors.Is(err, ErrInvalidRequest) || errors.As(err, &validation) {
				status = http.StatusBadRequest
			}
			ctx.String(status, "%s", err.Error())
			return
		}
		ctx.Data(http.StatusOK, "application/json", result)
	}})
	surface.MCP.AddTool(&mcp.Tool{Name: name, Title: humanDescription, Description: humanDescription + " Technical operation: " + name + ".", InputSchema: schema}, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := invoke(ctx, request.Params.Arguments)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(result)}}, StructuredContent: json.RawMessage(result)}, nil
	})
}

// MCPHandler supports legacy clients and per-request protocol metadata. Durable
// knowledge belongs to the service stores, so the HTTP transport is stateless.
func (surface *Surface) MCPHandler() *mcp.StreamableHTTPHandler {
	return mcp.NewStreamableHTTPHandler(func(request *http.Request) *mcp.Server { return surface.MCP }, &mcp.StreamableHTTPOptions{Stateless: true})
}
