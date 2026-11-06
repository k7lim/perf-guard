//go:build !darwin

package process

import (
	"context"
	"errors"
	"github.com/k7lim/perf-guard/internal/review"
)

type platformSnapshot struct{}
type platformCWD struct{}

func ReadIdentity(context.Context, int) (review.ProcessIdentity, error) {
	return review.ProcessIdentity{}, errors.New("process identity collection requires macOS")
}
func (platformSnapshot) Snapshot(context.Context) ([]SnapshotProcess, []review.Warning, error) {
	return nil, nil, errors.New("process collection requires macOS")
}
func (platformCWD) Lookup(context.Context, review.ProcessIdentity, int) (string, error) {
	return "", errors.New("cwd collection requires macOS")
}
