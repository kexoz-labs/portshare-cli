package cmd

import (
	"fmt"
	"strings"
)

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiGreen  = "\x1b[32m"
	ansiBGreen = "\x1b[1;32m"
	ansiCyan   = "\x1b[36m"
	ansiYellow = "\x1b[33m"
	ansiGray   = "\x1b[90m"
	ansiWhite  = "\x1b[97m"
	ansiRed    = "\x1b[31m"
	ansiDim    = "\x1b[2m"
)

func printBanner(publicURL, localPort, clientID, plan string) {
	width := 54
	border := strings.Repeat("-", width)

	fmt.Printf("\n%s+%s+%s\n", ansiGreen, border, ansiReset)
	fmt.Printf("%s|%s  %sv Tunnel Active%s%s%s|%s\n",
		ansiGreen, ansiReset,
		ansiBGreen, ansiReset,
		strings.Repeat(" ", width-16),
		ansiGreen, ansiReset,
	)
	fmt.Printf("%s|%s%s%s|%s\n", ansiGreen, strings.Repeat(" ", width), ansiReset, ansiGreen, ansiReset)

	urlLabel := "  Public URL  "
	urlVal := publicURL
	urlPad := width - len(urlLabel) - len(urlVal) - 2
	if urlPad < 0 {
		urlPad = 0
		urlVal = urlVal[:width-len(urlLabel)-5] + "..."
	}
	fmt.Printf("%s|%s%s%s%s%s%s%s%s|%s\n",
		ansiGreen, ansiReset,
		ansiGray, urlLabel, ansiReset,
		ansiBold+ansiCyan, urlVal, ansiReset,
		strings.Repeat(" ", urlPad),
		ansiGreen,
	)

	fwdLabel := "  Forwarding  "
	fwdVal := fmt.Sprintf("http://localhost:%s", localPort)
	fwdPad := width - len(fwdLabel) - len(fwdVal) - 2
	if fwdPad < 0 {
		fwdPad = 0
	}
	fmt.Printf("%s|%s%s%s%s%s%s%s%s|%s\n",
		ansiGreen, ansiReset,
		ansiGray, fwdLabel, ansiReset,
		ansiWhite, fwdVal, ansiReset,
		strings.Repeat(" ", fwdPad),
		ansiGreen,
	)

	if clientID != "" {
		idLabel := "  Client ID   "
		shortID := clientID
		if len(shortID) > 22 {
			shortID = shortID[:22] + "..."
		}
		idPad := width - len(idLabel) - len(shortID) - 2
		if idPad < 0 {
			idPad = 0
		}
		fmt.Printf("%s|%s%s%s%s%s%s%s%s|%s\n",
			ansiGreen, ansiReset,
			ansiGray, idLabel, ansiReset,
			ansiDim+ansiWhite, shortID, ansiReset,
			strings.Repeat(" ", idPad),
			ansiGreen,
		)
	}

	fmt.Printf("%s|%s%s%s|%s\n", ansiGreen, strings.Repeat(" ", width), ansiReset, ansiGreen, ansiReset)
	fmt.Printf("%s+%s+%s\n\n", ansiGreen, border, ansiReset)
	fmt.Printf("%s  Watching for requests... (Ctrl+C to stop)%s\n\n", ansiGray, ansiReset)
}

func methodColor(method string) string {
	switch strings.ToUpper(method) {
	case "GET":
		return ansiGreen
	case "POST":
		return ansiCyan
	case "PUT":
		return ansiYellow
	case "DELETE":
		return ansiRed
	case "PATCH":
		return "\x1b[35m"
	default:
		return ansiGray
	}
}

func statusColor(status int) string {
	switch {
	case status < 300:
		return ansiGreen
	case status < 400:
		return ansiCyan
	case status < 500:
		return ansiYellow
	default:
		return ansiRed
	}
}

func PrintRequest(method, path string, status int, durationMs int64) {
	statusStr := fmt.Sprintf("%d", status)
	if status == 0 {
		statusStr = "ERR"
	}
	pathTrunc := path
	if len(pathTrunc) > 50 {
		pathTrunc = pathTrunc[:47] + "..."
	}
	fmt.Printf("  %s%s%-7s%s  %s%-3s%s  %s%s%s  %s%dms%s\n",
		methodColor(method), ansiBold, method, ansiReset,
		statusColor(status), statusStr, ansiReset,
		ansiGray, pathTrunc, ansiReset,
		ansiDim, durationMs, ansiReset,
	)
}