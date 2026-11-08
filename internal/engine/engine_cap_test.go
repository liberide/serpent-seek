package engine

import (
	"testing"
	"time"
)

func TestAsyncRunCap(t *testing.T) {
	cases := []struct {
		name string
		secs int
		want time.Duration
	}{
		{"disabled keeps the floor", 0, 10 * time.Minute},
		{"short timeout keeps the floor", 60, 10 * time.Minute},
		{"longer chain timeout wins", 1200, 20 * time.Minute},
		{"negative is treated as unset", -5, 10 * time.Minute},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := asyncRunCap(c.secs); got != c.want {
				t.Fatalf("asyncRunCap(%d) = %v, want %v", c.secs, got, c.want)
			}
		})
	}
}
