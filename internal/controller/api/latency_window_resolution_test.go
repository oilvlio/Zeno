package api

import (
	"testing"
	"time"
)

func TestLatencyWindowVariants(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		resolve func(string) (latencyWindow, bool)
		want    map[string]latencyWindow
	}{
		{
			name:    "series",
			resolve: resolveLatencyWindow,
			want: map[string]latencyWindow{
				"":    {Name: "1h", Samples: 60, Step: time.Minute},
				"1h":  {Name: "1h", Samples: 60, Step: time.Minute},
				"1d":  {Name: "1d", Samples: 48, Step: 30 * time.Minute},
				"3d":  {Name: "3d", Samples: 24, Step: 3 * time.Hour},
				"7d":  {Name: "7d", Samples: 56, Step: 3 * time.Hour},
				"30d": {Name: "30d", Samples: 60, Step: 12 * time.Hour},
			},
		},
		{
			name:    "grid",
			resolve: resolveLatencyGridWindow,
			want: map[string]latencyWindow{
				"":    {Name: "1h", Samples: 60, Step: time.Minute},
				"1h":  {Name: "1h", Samples: 60, Step: time.Minute},
				"1d":  {Name: "1d", Samples: 1440, Step: time.Minute},
				"3d":  {Name: "3d", Samples: 1440, Step: 3 * time.Minute},
				"7d":  {Name: "7d", Samples: 1440, Step: 7 * time.Minute},
				"30d": {Name: "30d", Samples: 1440, Step: 30 * time.Minute},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for input, want := range test.want {
				got, ok := test.resolve(input)
				if !ok || got != want {
					t.Fatalf("resolve(%q) = (%+v, %v), want (%+v, true)", input, got, ok, want)
				}
			}
			if got, ok := test.resolve("invalid"); ok || got != (latencyWindow{}) {
				t.Fatalf("resolve(invalid) = (%+v, %v), want zero, false", got, ok)
			}
		})
	}
}
