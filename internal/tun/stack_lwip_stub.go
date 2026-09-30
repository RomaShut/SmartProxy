//go:build !with_lwip || !cgo

package tun

import (
	"errors"

	singtun "github.com/sagernet/sing-tun"
)

// ErrLWIPNotIncluded is returned when the lwIP stack is requested but not compiled in.
var ErrLWIPNotIncluded = errors.New("lwip stack is not included in this build, rebuild with -tags with_lwip")

// NewLWIPStack returns ErrLWIPNotIncluded when built without with_lwip tag.
func NewLWIPStack(options singtun.StackOptions) (singtun.Stack, error) {
	return nil, ErrLWIPNotIncluded
}
