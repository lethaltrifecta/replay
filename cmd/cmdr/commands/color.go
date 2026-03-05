package commands

import (
	"fmt"
	"os"

	"github.com/lethaltrifecta/replay/pkg/storage"
)

var noColor = os.Getenv("NO_COLOR") != ""

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

func colorize(color, text string) string {
	if noColor {
		return text
	}
	return color + text + colorReset
}

func coloredVerdictDisplay(verdict string) string {
	switch verdict {
	case storage.DriftVerdictPass:
		return colorize(colorBold+colorGreen, "PASS")
	case storage.DriftVerdictWarn:
		return colorize(colorBold+colorYellow, "WARN")
	case storage.DriftVerdictFail:
		return colorize(colorBold+colorRed, "FAIL")
	default:
		return verdict
	}
}

func coloredScore(score, threshold float64) string {
	text := fmt.Sprintf("%.4f", score)
	if score >= threshold {
		return colorize(colorGreen, text)
	}
	return colorize(colorRed, text)
}
