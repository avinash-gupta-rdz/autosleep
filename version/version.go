// Package version knows which AutoSleep release is running and, unless
// UPDATE_CHECK=false, whether a newer one has been published on GitHub, so
// the dashboard can point self-hosters at an upgrade.
package version

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Current is this build's version. Bump it (and CHANGELOG.md) when tagging
// a release.
const Current = "0.2.0"

// Repo is where releases are published.
const Repo = "avinash-gupta-rdz/autosleep"

// ReleasesURL is the page users are sent to for upgrade notes.
const ReleasesURL = "https://github.com/" + Repo + "/releases"

var (
	mu     sync.RWMutex
	latest string
)

// Latest returns the newest published release tag seen, or "".
func Latest() string {
	mu.RLock()
	defer mu.RUnlock()
	return latest
}

// UpdateAvailable reports whether Latest is newer than Current.
func UpdateAvailable() bool { return Newer(Latest(), Current) }

// Newer reports whether version a is greater than b ("v1.2.3" or "1.2.3").
func Newer(a, b string) bool {
	pa, pb := parse(a), parse(b)
	if pa == nil || pb == nil {
		return false
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parse(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v, _, _ = strings.Cut(v, "-")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nil
	}
	out := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		out[i] = n
	}
	return out
}

// CheckEnabled reports whether the daily GitHub check runs. It sends one
// anonymous request a day to api.github.com and nothing else.
func CheckEnabled() bool {
	return !strings.EqualFold(os.Getenv("UPDATE_CHECK"), "false")
}

// fetch asks GitHub for the latest release tag. A variable for tests.
var fetch = func(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+Repo+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "autosleep/"+Current)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil // no releases yet, or rate limited: try tomorrow
	}
	var r struct {
		TagName string `json:"tag_name"`
	}
	err = json.NewDecoder(resp.Body).Decode(&r)
	return r.TagName, err
}

// Refresh updates Latest once.
func Refresh(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if tag, err := fetch(ctx); err == nil && tag != "" {
		mu.Lock()
		latest = tag
		mu.Unlock()
	}
}

// Watch refreshes Latest now and then daily, until ctx is done.
func Watch(ctx context.Context) {
	if !CheckEnabled() {
		return
	}
	for {
		Refresh(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(24 * time.Hour):
		}
	}
}
