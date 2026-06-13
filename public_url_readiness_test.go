package main

import "testing"

func TestPublicURLStatusReady(t *testing.T) {
	for _, status := range []int{200, 204, 301, 404} {
		if !publicURLStatusReady(status) {
			t.Fatalf("status %d should be ready", status)
		}
	}
	for _, status := range []int{0, 500, 525, 526, 527, 530} {
		if publicURLStatusReady(status) {
			t.Fatalf("status %d should not be ready", status)
		}
	}
}
