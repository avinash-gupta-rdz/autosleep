// Package activity decides whether a batch of Heroku log-drain messages
// shows real HTTP traffic to an app.
package activity

import (
	"bytes"
	"strconv"
	"strings"
)

// Message is one syslog line from a Heroku log drain:
// "<pri>1 timestamp host app proc - message".
type Message struct {
	App  string // "heroku" for platform logs, "app" for the app's own
	Proc string // e.g. "router", "web.1"
	Text string
}

// Parse splits an application/logplex-1 body into messages. Logplex frames
// are octet-counted ("<len> <msg>"); if framing is broken it falls back to
// splitting on newlines so a malformed batch still gets inspected.
func Parse(body []byte) []Message {
	var out []Message
	rest := body
	for len(bytes.TrimSpace(rest)) > 0 {
		rest = bytes.TrimLeft(rest, " \r\n")
		sp := bytes.IndexByte(rest, ' ')
		if sp <= 0 {
			return append(out, parseLines(rest)...)
		}
		n, err := strconv.Atoi(string(rest[:sp]))
		if err != nil || n <= 0 || sp+1+n > len(rest) {
			return append(out, parseLines(rest)...)
		}
		out = append(out, parseLine(string(rest[sp+1:sp+1+n])))
		rest = rest[sp+1+n:]
	}
	return out
}

func parseLines(b []byte) []Message {
	var out []Message
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, parseLine(line))
		}
	}
	return out
}

func parseLine(line string) Message {
	line = strings.TrimSpace(line)
	// Fields: <pri>version, timestamp, hostname, app, procid, msgid, text
	f := strings.SplitN(line, " ", 7)
	if len(f) < 6 {
		return Message{Text: line}
	}
	m := Message{App: f[3], Proc: f[4]}
	if len(f) == 7 {
		m.Text = f[6]
	}
	return m
}

// IsRouterRequest reports whether m is a Heroku router log for an incoming
// request. Router errors (e.g. H14 "No web processes running" while the app
// sleeps) count too: they are exactly the requests that must wake it.
// Requests for /.well-known/ (ACME certificate checks) are ignored.
func IsRouterRequest(m Message) bool {
	if m.Proc != "router" && m.App != "router" {
		return false
	}
	return !strings.Contains(m.Text, "/.well-known/")
}

// Summary describes what a drain batch says about an app.
type Summary struct {
	Traffic bool // at least one real HTTP request reached the router
	NoDynos bool // the router reported H14 "No web processes running"
}

// Analyze inspects a drain batch.
func Analyze(body []byte) Summary {
	var s Summary
	for _, m := range Parse(body) {
		if IsRouterRequest(m) {
			s.Traffic = true
			if strings.Contains(m.Text, "code=H14") {
				s.NoDynos = true
			}
		}
	}
	return s
}

// HasTraffic reports whether any message in body is a router request.
func HasTraffic(body []byte) bool { return Analyze(body).Traffic }
