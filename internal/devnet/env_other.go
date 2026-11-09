//go:build !darwin

package devnet

import "context"

func osDetect(context.Context) ([]Route, []Resolver) { return nil, nil }
