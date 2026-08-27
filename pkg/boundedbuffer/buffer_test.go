package boundedbuffer_test

import (
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/candacelabs/candace/pkg/boundedbuffer"
	boundedbufferv1 "github.com/candacelabs/candace/pkg/boundedbuffer/v1"
)

var _ = Describe("Buffer", func() {
	It("retains output within its bound", func() {
		buffer, err := boundedbuffer.New(&boundedbufferv1.Retention{MaxBytes: 8})
		Expect(err).NotTo(HaveOccurred())
		written, err := io.WriteString(buffer, "hello")

		Expect(err).NotTo(HaveOccurred())
		Expect(written).To(Equal(5))
		Expect(buffer.Bytes()).To(Equal([]byte("hello")))
		Expect(buffer.String()).To(Equal("hello"))
	})

	It("drains every write while marking discarded bytes", func() {
		buffer, err := boundedbuffer.New(&boundedbufferv1.Retention{MaxBytes: 5})
		Expect(err).NotTo(HaveOccurred())
		first, firstErr := io.WriteString(buffer, "hello")
		second, secondErr := io.WriteString(buffer, " world")

		Expect(firstErr).NotTo(HaveOccurred())
		Expect(secondErr).NotTo(HaveOccurred())
		Expect(first).To(Equal(5))
		Expect(second).To(Equal(6))
		Expect(buffer.Bytes()).To(Equal([]byte("hello")))
		Expect(buffer.String()).To(Equal("hello (truncated)"))
	})
})
