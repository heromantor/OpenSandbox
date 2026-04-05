//go:build tools

package firecracker

// Import dependencies that are used in Plan 02 but declared in go.mod now
// to ensure they are pinned. This file is excluded from normal builds.
import (
	_ "github.com/firecracker-microvm/firecracker-go-sdk"
	_ "github.com/sirupsen/logrus"
)
