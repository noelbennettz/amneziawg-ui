package httpapi

import (
	"fmt"
	"net"
	"testing"

	"github.com/gofiber/fiber/v3"
)

// The panel has to answer on both stacks: Fiber's default listener is "tcp4",
// which binds 0.0.0.0 and makes every IPv6 client's connection attempt fail no
// matter what the host or the container publishes. This test listens with the
// very network the app runs with and connects from both loopbacks, so a
// regression in FiberConfig fails here rather than on a v6-only network.
func TestFiberConfigListensOnBothStacks(t *testing.T) {
	network := ListenConfig().ListenerNetwork
	if network != fiber.NetworkTCP {
		t.Fatalf("ListenerNetwork = %q, want %q (the dual-stack listener)", network, fiber.NetworkTCP)
	}

	listener, err := net.Listen(network, ":0")
	if err != nil {
		t.Skipf("cannot listen on %q here: %v", network, err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	for _, address := range []string{
		fmt.Sprintf("127.0.0.1:%d", port),
		fmt.Sprintf("[::1]:%d", port),
	} {
		conn, err := net.Dial("tcp", address)
		if err != nil {
			t.Fatalf("the listener on %q does not answer at %s: %v", listener.Addr(), address, err)
		}
		conn.Close()
	}
}
