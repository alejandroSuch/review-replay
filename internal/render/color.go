package render

import (
	"os"
	"regexp"
)

// useColor is set once at startup. Toggles all color helpers off when stdout
// is not a TTY (piping, redirection) or when NO_COLOR is set.
var useColor = detectColor()

func detectColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func wrap(s, prefix string) string {
	if !useColor {
		return s
	}
	return prefix + s + "\x1b[0m"
}

func bold(s string) string   { return wrap(s, "\x1b[1m") }
func dim(s string) string    { return wrap(s, "\x1b[2m") }
func red(s string) string    { return wrap(s, "\x1b[31m") }
func green(s string) string  { return wrap(s, "\x1b[32m") }
func yellow(s string) string { return wrap(s, "\x1b[33m") }
func cyan(s string) string   { return wrap(s, "\x1b[36m") }

var ansiCodes = regexp.MustCompile("\x1b\\[[0-9;]*m")

// stripANSI returns s without ANSI escape sequences. Used by column padding so
// colored text still aligns to the intended visible width.
func stripANSI(s string) string {
	return ansiCodes.ReplaceAllString(s, "")
}
