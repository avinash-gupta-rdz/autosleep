// Package herokuclient wraps the Heroku Platform API with the few calls
// AutoSleep needs. Every Client carries its own token and transport, so
// concurrent jobs for different accounts never share credentials.
package herokuclient

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	heroku "github.com/heroku/heroku-go/v5"
)

// Formation is a single process type of an app, e.g. web=2:standard-1X.
type Formation struct {
	Type     string
	Size     string
	Quantity int
}

// Drain is a log drain registered on an app.
type Drain struct {
	ID    string
	Token string
	URL   string
}

// App is the subset of Heroku app metadata AutoSleep shows to users.
type App struct {
	Name   string
	WebURL string
}

// Client is the Heroku API surface used by AutoSleep. It is an interface so
// the sleep/wake logic can be tested without talking to Heroku.
type Client interface {
	ListFormations(ctx context.Context, app string) ([]Formation, error)
	UpdateFormations(ctx context.Context, app string, updates []Formation) error
	CreateDrain(ctx context.Context, app, url string) (Drain, error)
	DrainInfo(ctx context.Context, app, id string) (Drain, error)
	ListDrains(ctx context.Context, app string) ([]Drain, error)
	DeleteDrain(ctx context.Context, app, id string) error
	ListApps(ctx context.Context) ([]App, error)
	GetConfigVar(ctx context.Context, app, key string) (string, error)
	SetConfigVar(ctx context.Context, app, key string, value *string) error
	AccountInfo(ctx context.Context) (email, name string, err error)
}

// New returns a Client authenticated with token. It is a variable so tests
// can substitute a fake.
var New = func(token string) Client {
	return &client{svc: heroku.NewService(&http.Client{
		Timeout: 30 * time.Second,
		Transport: &heroku.Transport{
			BearerToken: token,
			UserAgent:   "autosleep",
			Transport:   http.DefaultTransport,
		},
	})}
}

type client struct {
	svc *heroku.Service
}

// IsNotFound reports whether err is a Heroku 404 (app or drain is gone).
func IsNotFound(err error) bool {
	var e heroku.Error
	return errors.As(err, &e) && e.StatusCode == http.StatusNotFound
}

// Ignored process types are one-off dynos, never part of a formation we
// should scale.
func ignoredType(t string) bool {
	return t == "console" || t == "rake"
}

func (c *client) ListFormations(ctx context.Context, app string) ([]Formation, error) {
	list, err := c.svc.FormationList(ctx, app, &heroku.ListRange{Field: "id", Max: 1000})
	if err != nil {
		return nil, err
	}
	out := make([]Formation, 0, len(list))
	for _, f := range list {
		if ignoredType(f.Type) {
			continue
		}
		out = append(out, Formation{Type: f.Type, Size: f.Size, Quantity: f.Quantity})
	}
	return out, nil
}

func (c *client) UpdateFormations(ctx context.Context, app string, updates []Formation) error {
	if len(updates) == 0 {
		return nil
	}
	opts := heroku.FormationBatchUpdateOpts{}
	for _, u := range updates {
		qty := u.Quantity
		upd := struct {
			Quantity *int    `json:"quantity,omitempty" url:"quantity,omitempty,key"`
			Size     *string `json:"size,omitempty" url:"size,omitempty,key"`
			Type     string  `json:"type" url:"type,key"`
		}{Quantity: &qty, Type: u.Type}
		if u.Size != "" {
			size := u.Size
			upd.Size = &size
		}
		opts.Updates = append(opts.Updates, upd)
	}
	_, err := c.svc.FormationBatchUpdate(ctx, app, opts)
	return err
}

func (c *client) CreateDrain(ctx context.Context, app, url string) (Drain, error) {
	d, err := c.svc.LogDrainCreate(ctx, app, heroku.LogDrainCreateOpts{URL: url})
	if err != nil {
		return Drain{}, err
	}
	return Drain{ID: d.ID, Token: d.Token, URL: d.URL}, nil
}

func (c *client) DrainInfo(ctx context.Context, app, id string) (Drain, error) {
	d, err := c.svc.LogDrainInfo(ctx, app, id)
	if err != nil {
		return Drain{}, err
	}
	return Drain{ID: d.ID, Token: d.Token, URL: d.URL}, nil
}

func (c *client) ListDrains(ctx context.Context, app string) ([]Drain, error) {
	list, err := c.svc.LogDrainList(ctx, app, &heroku.ListRange{Field: "id", Max: 1000})
	if err != nil {
		return nil, err
	}
	out := make([]Drain, 0, len(list))
	for _, d := range list {
		out = append(out, Drain{ID: d.ID, Token: d.Token, URL: d.URL})
	}
	return out, nil
}

func (c *client) DeleteDrain(ctx context.Context, app, id string) error {
	_, err := c.svc.LogDrainDelete(ctx, app, id)
	return err
}

func (c *client) ListApps(ctx context.Context) ([]App, error) {
	list, err := c.svc.AppList(ctx, &heroku.ListRange{Field: "name", Max: 1000})
	if err != nil {
		return nil, err
	}
	out := make([]App, 0, len(list))
	for _, a := range list {
		out = append(out, App{Name: a.Name, WebURL: a.WebURL})
	}
	return out, nil
}

func (c *client) GetConfigVar(ctx context.Context, app, key string) (string, error) {
	vars, err := c.svc.ConfigVarInfoForApp(ctx, app)
	if err != nil {
		return "", err
	}
	if v, ok := vars[key]; ok && v != nil {
		return *v, nil
	}
	return "", nil
}

// SetConfigVar sets key to value, or unsets it when value is nil.
func (c *client) SetConfigVar(ctx context.Context, app, key string, value *string) error {
	_, err := c.svc.ConfigVarUpdate(ctx, app, map[string]*string{key: value})
	return err
}

func (c *client) AccountInfo(ctx context.Context) (string, string, error) {
	a, err := c.svc.AccountInfo(ctx)
	if err != nil {
		return "", "", err
	}
	name := ""
	if a.Name != nil {
		name = *a.Name
	}
	if name == "" {
		name = strings.Split(a.Email, "@")[0]
	}
	return a.Email, name, nil
}
