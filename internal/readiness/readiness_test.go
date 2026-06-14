package readiness

import "testing"

func TestIsStatusReady(t *testing.T) {
	for _, status := range []int{200, 204, 301, 404} {
		if !IsStatusReady(status) {
			t.Fatalf("status %d should be ready", status)
		}
	}
	for _, status := range []int{0, 500, 525, 526, 527, 530} {
		if IsStatusReady(status) {
			t.Fatalf("status %d should not be ready", status)
		}
	}
}
