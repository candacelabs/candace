package csf_test

import (
	"context"
	"net/http/httptest"

	"github.com/candacelabs/candace/csf"
	"github.com/candacelabs/candace/pkg/httpserver"
	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The same consumer-owned Gin host and real transports can exercise any CSF
// capability selected with options; no production interface is mocked here.
type csfConsumer struct {
	context context.Context
	client  *csf.Client
	agent   *mcp.ClientSession
}

func newCSFConsumer(options ...csf.Option) *csfConsumer {
	ctx, cancel := context.WithCancel(context.Background())
	DeferCleanup(cancel)
	service, err := csf.New(options...)
	Expect(err).NotTo(HaveOccurred())
	surface := csf.NewSurface(service)
	router := httpserver.NewEngine("csf-consumer")
	surface.Register(router)
	router.Any("/mcp", gin.WrapH(surface.MCPHandler()))
	server := httptest.NewServer(router)
	DeferCleanup(server.Close)
	client, err := csf.NewClient(server.URL, server.Client())
	Expect(err).NotTo(HaveOccurred())
	return &csfConsumer{context: ctx, client: client, agent: connectCSFConsumer(ctx, server.URL)}
}

func connectCSFConsumer(ctx context.Context, endpoint string) *mcp.ClientSession {
	agent := mcp.NewClient(&mcp.Implementation{Name: "csf-consumer", Version: "1"}, nil)
	session, err := agent.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint + "/mcp"}, nil)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(session.Close)
	return session
}
