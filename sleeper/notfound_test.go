package sleeper

import (
	"net/http"

	heroku "github.com/heroku/heroku-go/v5"
)

// heroku404 converts to the error the real client returns for a deleted app.
type heroku404 struct{}

func (heroku404) Error() string { return "Couldn't find that app." }
func (heroku404) As(target interface{}) bool {
	if e, ok := target.(*heroku.Error); ok {
		*e = heroku.Error{StatusCode: http.StatusNotFound}
		return true
	}
	return false
}
