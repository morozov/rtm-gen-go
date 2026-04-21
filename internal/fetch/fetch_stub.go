//go:build !livefetch

package fetch

import (
	"context"
	"errors"

	"github.com/morozov/rtm-gen-go/internal/apispec"
)

// ErrLiveFetchDisabled is returned when Fetch is called on a build
// that did not include the `livefetch` tag. Rebuild with
// `-tags=livefetch` to enable.
var ErrLiveFetchDisabled = errors.New("live fetch requires the livefetch build tag")

// Fetch is the default build's stub. Call with `-tags=livefetch` for
// the real implementation.
func Fetch(_ context.Context, _, _, _ string) (apispec.Spec, error) {
	return nil, ErrLiveFetchDisabled
}
