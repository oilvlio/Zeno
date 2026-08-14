package api

import "time"

type latencyWindow struct {
	Name    string
	Samples int
	Step    time.Duration
}

func extendedHistoryWindow(window latencyWindow) bool {
	return window.Name == "7d" || window.Name == "30d"
}

var latencyWindowVariants = map[string][2]latencyWindow{
	"1h":  {{Name: "1h", Samples: 20, Step: 3 * time.Minute}, {Name: "1h", Samples: 20, Step: 3 * time.Minute}},
	"1d":  {{Name: "1d", Samples: 48, Step: 30 * time.Minute}, {Name: "1d", Samples: 1440, Step: time.Minute}},
	"7d":  {{Name: "7d", Samples: 56, Step: 3 * time.Hour}, {Name: "7d", Samples: 1440, Step: 7 * time.Minute}},
	"30d": {{Name: "30d", Samples: 60, Step: 12 * time.Hour}, {Name: "30d", Samples: 1440, Step: 30 * time.Minute}},
}

func resolveLatencyWindowVariant(rangeName string, variant int) (latencyWindow, bool) {
	if rangeName == "" {
		rangeName = "1h"
	}
	windows, ok := latencyWindowVariants[rangeName]
	if !ok {
		return latencyWindow{}, false
	}
	return windows[variant], true
}

func resolveLatencyWindow(rangeName string) (latencyWindow, bool) {
	return resolveLatencyWindowVariant(rangeName, 0)
}

func resolveLatencyGridWindow(rangeName string) (latencyWindow, bool) {
	return resolveLatencyWindowVariant(rangeName, 1)
}

func resolveStateWindow(rangeName string) (latencyWindow, bool) {
	switch rangeName {
	case "", "1h":
		return latencyWindow{Name: "1h", Samples: 30, Step: 2 * time.Minute}, true
	case "1d":
		return latencyWindow{Name: "1d", Samples: 2880, Step: 30 * time.Second}, true
	case "7d":
		return latencyWindow{Name: "7d", Samples: 336, Step: 30 * time.Minute}, true
	case "30d":
		return latencyWindow{Name: "30d", Samples: 360, Step: 2 * time.Hour}, true
	default:
		return latencyWindow{}, false
	}
}
