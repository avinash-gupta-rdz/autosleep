// Package fake is an in-memory Heroku Platform API. Tests use it to
// exercise sleep/wake logic, and demo mode uses it so AutoSleep can be tried
// without a Heroku account.
package fake

import (
	"autosleep/herokuclient"
	"context"
	"errors"
	"fmt"
	"sync"
)

// Heroku is an in-memory Heroku API, used by tests and demo mode.
type Heroku struct {
	mu         sync.Mutex
	Formations map[string][]herokuclient.Formation // app -> formation
	Drains     map[string][]herokuclient.Drain
	ConfigVars map[string]map[string]string
	Apps       []herokuclient.App
	Tokens     []string // tokens clients were created with
	Email      string   // account email returned by AccountInfo
	Name       string
	Calls      []string

	// FailNext makes the next call to the named method fail.
	FailNext map[string]error
	// FailAfter makes the next call to the named method take effect but
	// still return an error, like a request that timed out after Heroku
	// processed it.
	FailAfter map[string]error
}

// Install replaces herokuclient.New with the fake until t's cleanup runs.
func (f *Heroku) Install(t interface{ Cleanup(func()) }) *Heroku {
	t.Cleanup(f.Activate())
	return f
}

// Activate replaces herokuclient.New with the fake and returns a function
// that restores the real client.
func (f *Heroku) Activate() (restore func()) {
	if f.Formations == nil {
		f.Formations = map[string][]herokuclient.Formation{}
	}
	if f.Drains == nil {
		f.Drains = map[string][]herokuclient.Drain{}
	}
	if f.ConfigVars == nil {
		f.ConfigVars = map[string]map[string]string{}
	}
	if f.FailNext == nil {
		f.FailNext = map[string]error{}
	}
	if f.FailAfter == nil {
		f.FailAfter = map[string]error{}
	}
	prev := herokuclient.New
	herokuclient.New = func(token string) herokuclient.Client {
		f.mu.Lock()
		f.Tokens = append(f.Tokens, token)
		f.mu.Unlock()
		return &fakeClient{f: f}
	}
	return func() { herokuclient.New = prev }
}

// Running returns the current quantity of each process type for app.
func (f *Heroku) Running(app string) map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]int{}
	for _, fm := range f.Formations[app] {
		out[fm.Type] = fm.Quantity
	}
	return out
}

func (f *Heroku) call(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, name)
	if err, ok := f.FailNext[name]; ok {
		delete(f.FailNext, name)
		return err
	}
	return nil
}

var ErrNotFound = errors.New("not found")

type fakeClient struct{ f *Heroku }

func (c *fakeClient) ListFormations(ctx context.Context, app string) ([]herokuclient.Formation, error) {
	if err := c.f.call("ListFormations"); err != nil {
		return nil, err
	}
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	return append([]herokuclient.Formation(nil), c.f.Formations[app]...), nil
}

func (c *fakeClient) UpdateFormations(ctx context.Context, app string, updates []herokuclient.Formation) (retErr error) {
	if err := c.f.call("UpdateFormations"); err != nil {
		return err
	}
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	if err, ok := c.f.FailAfter["UpdateFormations"]; ok {
		delete(c.f.FailAfter, "UpdateFormations")
		defer func() { retErr = err }()
	}
	for _, u := range updates {
		found := false
		for i, fm := range c.f.Formations[app] {
			if fm.Type == u.Type {
				c.f.Formations[app][i].Quantity = u.Quantity
				if u.Size != "" {
					c.f.Formations[app][i].Size = u.Size
				}
				found = true
			}
		}
		if !found {
			c.f.Formations[app] = append(c.f.Formations[app], u)
		}
	}
	return nil
}

func (c *fakeClient) CreateDrain(ctx context.Context, app, url string) (herokuclient.Drain, error) {
	if err := c.f.call("CreateDrain"); err != nil {
		return herokuclient.Drain{}, err
	}
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	for _, d := range c.f.Drains[app] {
		if d.URL == url {
			return herokuclient.Drain{}, errors.New("url has already been taken")
		}
	}
	n := len(c.f.Drains[app]) + 1
	d := herokuclient.Drain{ID: fmt.Sprintf("drain-%s-%d", app, n), Token: fmt.Sprintf("d.token-%s-%d", app, n), URL: url}
	c.f.Drains[app] = append(c.f.Drains[app], d)
	return d, nil
}

func (c *fakeClient) DrainInfo(ctx context.Context, app, id string) (herokuclient.Drain, error) {
	if err := c.f.call("DrainInfo"); err != nil {
		return herokuclient.Drain{}, err
	}
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	for _, d := range c.f.Drains[app] {
		if d.ID == id {
			return d, nil
		}
	}
	return herokuclient.Drain{}, ErrNotFound
}

func (c *fakeClient) ListDrains(ctx context.Context, app string) ([]herokuclient.Drain, error) {
	if err := c.f.call("ListDrains"); err != nil {
		return nil, err
	}
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	return append([]herokuclient.Drain(nil), c.f.Drains[app]...), nil
}

func (c *fakeClient) DeleteDrain(ctx context.Context, app, id string) error {
	if err := c.f.call("DeleteDrain"); err != nil {
		return err
	}
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	var kept []herokuclient.Drain
	for _, d := range c.f.Drains[app] {
		if d.ID != id {
			kept = append(kept, d)
		}
	}
	c.f.Drains[app] = kept
	return nil
}

func (c *fakeClient) ListApps(ctx context.Context) ([]herokuclient.App, error) {
	if err := c.f.call("ListApps"); err != nil {
		return nil, err
	}
	return c.f.Apps, nil
}

func (c *fakeClient) GetConfigVar(ctx context.Context, app, key string) (string, error) {
	if err := c.f.call("GetConfigVar"); err != nil {
		return "", err
	}
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	return c.f.ConfigVars[app][key], nil
}

func (c *fakeClient) SetConfigVar(ctx context.Context, app, key string, value *string) error {
	if err := c.f.call("SetConfigVar"); err != nil {
		return err
	}
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	if c.f.ConfigVars[app] == nil {
		c.f.ConfigVars[app] = map[string]string{}
	}
	if value == nil {
		delete(c.f.ConfigVars[app], key)
	} else {
		c.f.ConfigVars[app][key] = *value
	}
	return nil
}

func (c *fakeClient) AccountInfo(ctx context.Context) (string, string, error) {
	if err := c.f.call("AccountInfo"); err != nil {
		return "", "", err
	}
	if c.f.Email != "" {
		return c.f.Email, c.f.Name, nil
	}
	return "dev@example.com", "Dev", nil
}
