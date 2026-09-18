package main

import (
	"encoding/base64"
	"os"
	"testing"

	copilot "github.com/github/copilot-sdk/go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	copilotv1 "github.com/candacelabs/candace/services/copilot-adapter/proto/candace/copilot/v1"
)

func TestBrainSpineCommand(test *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(test, "Brain Spine command composition")
}

var _ = Describe("Workbench native MCP configuration", func() {
	var config *copilotv1.TraceExportConfig
	BeforeEach(func() {
		config = &copilotv1.TraceExportConfig{}
		document, err := os.ReadFile("../../../services/copilot-adapter/config/traces.defaults.json")
		Expect(err).NotTo(HaveOccurred())
		Expect(protojson.Unmarshal(document, config)).To(Succeed())
		config.EndpointUrl = "http://collector.invalid:3000/api/public/otel/v1/traces"
		config.PublicKey, config.SecretKey = "public-fixture", "secret-fixture"
	})

	It("retains only the unauthenticated CSF connection without trace configuration", func() {
		servers, err := workbenchMCPServers("http://workbench.invalid/", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(servers).To(HaveLen(1))
		Expect(servers["csf"]).To(Equal(copilot.MCPHTTPServerConfig{URL: "http://workbench.invalid/mcp", Tools: []string{"*"}}))
	})

	DescribeTable("uses the trace origin and project credentials only for native Langfuse",
		func(endpoint string, expected string) {
			config.EndpointUrl = endpoint
			original := proto.Clone(config)
			servers, err := workbenchMCPServers("http://workbench.invalid", config)
			Expect(err).NotTo(HaveOccurred())
			Expect(servers).To(HaveLen(2))
			Expect(servers["brain-traces"]).To(Equal(copilot.MCPHTTPServerConfig{
				URL: expected, Tools: []string{"*"},
				Headers: map[string]string{"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("public-fixture:secret-fixture"))},
			}))
			Expect(servers["csf"]).To(Equal(copilot.MCPHTTPServerConfig{URL: "http://workbench.invalid/mcp", Tools: []string{"*"}}))
			Expect(proto.Equal(config, original)).To(BeTrue())
		},
		Entry("internal HTTP origin", "http://collector.invalid:3000/api/public/otel/v1/traces", "http://collector.invalid:3000/api/public/mcp"),
		Entry("TLS origin without exporter path or query", "https://collector.invalid:8443/custom%2Fotel?token=private-fixture#fragment", "https://collector.invalid:8443/api/public/mcp"),
	)

	DescribeTable("rejects unusable endpoints without exposing private input",
		func(endpoint string) {
			config.EndpointUrl = endpoint
			servers, err := workbenchMCPServers("http://workbench.invalid", config)
			Expect(servers).To(BeNil())
			Expect(err).To(MatchError("Workbench trace endpoint must be an HTTP URL without embedded credentials"))
		},
		Entry("missing host", "/private-fixture"),
		Entry("unsupported transport", "file:///private-fixture"),
		Entry("embedded credentials", "https://public-fixture:secret-fixture@collector.invalid/traces"),
		Entry("invalid URL", "http://collector.invalid/%private-fixture"),
	)

	It("rejects incomplete trace configuration before passing it to the SDK", func() {
		config.SecretKey = ""
		servers, err := workbenchMCPServers("http://workbench.invalid", config)
		Expect(servers).To(BeNil())
		Expect(err).To(MatchError("invalid Workbench trace configuration"))
	})
})
