package main

import (
	"fmt"
	"net"
	"strconv"
)

// A private listener may be published behind an authenticated TLS gateway.
// Wildcard and public-address listeners are rejected; the API always requires
// the integration token regardless of which listener is selected.
func validateListener(listen string) error {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return fmt.Errorf("OMNIDEX_INFERENCE_LISTEN requires a private or loopback IP and port")
	}
	ip := net.ParseIP(host)
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 || ip == nil || (!ip.IsLoopback() && !ip.IsPrivate()) {
		return fmt.Errorf("OMNIDEX_INFERENCE_LISTEN requires a private or loopback IP and port")
	}
	return nil
}
