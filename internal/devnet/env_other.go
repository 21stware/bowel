//go:build !darwin && !linux && !windows

package devnet

import "context"

func osDetect(context.Context) ([]Route, []Resolver, []string) { return nil, nil, nil }
