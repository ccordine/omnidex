package main

import "testing"

func TestPrivateGatewayListener(t *testing.T) {
	for _, address := range []string{"127.0.0.1:8097", "[::1]:8097", "172.17.0.1:8097", "10.66.0.1:8097"} {
		if err := validateListener(address); err != nil {
			t.Errorf("%s: %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:8097", ":8097", "[::]:8097", "8.8.8.8:8097", "172.17.0.1:0", "172.17.0.1:99999"} {
		if validateListener(address) == nil {
			t.Errorf("unsafe listener accepted: %s", address)
		}
	}
}
