package cobrashell

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// ANSI color/style escape codes. Pass these to [Colorize] to build colored
// prompt strings, or use them directly in any output that is not part of a
// readline prompt string.
const (
	ColorReset   = "\033[0m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorBold    = "\033[1m"
)

// Colorize wraps text with the given ANSI code for safe use inside a readline
// prompt string. The escape sequences are enclosed in \x01/\x02
// (RL_PROMPT_START_IGNORE / RL_PROMPT_END_IGNORE) markers so readline
// measures only the visible characters when calculating cursor position.
//
// If code is empty, text is returned unchanged.
//
// Example — green text in a prompt:
//
//	cfg.DynamicPrompt = func(code int) string {
//	    c := cobrashell.ColorGreen
//	    if code != 0 {
//	        c = cobrashell.ColorRed
//	    }
//	    return cobrashell.Colorize("› ", c)
//	}
func Colorize(text, code string) string {
	if code == "" {
		return text
	}
	return "\x01" + code + "\x02" + text + "\x01" + ColorReset + "\x02"
}

// writeErr prints a cobra-shell internal error message to stderr. When stderr
// is a terminal the message is colored red; otherwise it is printed verbatim.
// The format and args follow [fmt.Sprintf] conventions.
func writeErr(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if term.IsTerminal(int(os.Stderr.Fd())) {
		fmt.Fprint(os.Stderr, ColorRed+msg+ColorReset)
	} else {
		fmt.Fprint(os.Stderr, msg)
	}
}
