package main

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	copilot "github.com/github/copilot-sdk/go"

	copilotv1 "github.com/candacelabs/candace/services/copilot-adapter/proto/candace/copilot/v1"
)

const (
	workbenchCSFServer     = "csf"
	workbenchTraceServer   = "brain-traces"
	workbenchMCPPath       = "/mcp"
	workbenchTraceMCPPath  = "/api/public/mcp"
	workbenchAllTools      = "*"
	workbenchAuthorization = "Authorization"
	workbenchBasicAuth     = "Basic "
	workbenchSchemeHTTP    = "http"
	workbenchSchemeHTTPS   = "https"
)

// The same project credentials configure native Langfuse access and trace export.
// The upstream Copilot SDK owns MCP transport and permission requests.
func workbenchMCPServers(origin string, config *copilotv1.TraceExportConfig) (map[string]copilot.MCPServerConfig, error) {
	servers := map[string]copilot.MCPServerConfig{
		workbenchCSFServer: copilot.MCPHTTPServerConfig{
			URL: strings.TrimRight(origin, "/") + workbenchMCPPath, Tools: []string{workbenchAllTools},
		},
	}
	if config == nil {
		return servers, nil
	}
	if err := copilotv1.ValidateTraceExportConfig(config); err != nil {
		return nil, fmt.Errorf("invalid Workbench trace configuration")
	}
	endpoint, err := url.Parse(config.GetEndpointUrl())
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != workbenchSchemeHTTP && endpoint.Scheme != workbenchSchemeHTTPS) || endpoint.User != nil {
		return nil, fmt.Errorf("Workbench trace endpoint must be an HTTP URL without embedded credentials")
	}
	endpoint.Path, endpoint.RawPath, endpoint.RawQuery, endpoint.Fragment, endpoint.RawFragment = workbenchTraceMCPPath, "", "", "", ""
	endpoint.ForceQuery = false
	servers[workbenchTraceServer] = copilot.MCPHTTPServerConfig{
		URL: endpoint.String(), Tools: []string{workbenchAllTools},
		Headers: map[string]string{
			workbenchAuthorization: workbenchBasicAuth + base64.StdEncoding.EncodeToString([]byte(config.GetPublicKey()+":"+config.GetSecretKey())),
		},
	}
	return servers, nil
}
