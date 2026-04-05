//go:build linux

package firecracker

import (
	sdk "github.com/firecracker-microvm/firecracker-go-sdk"
)

// toSDKConfig converts the resolved jailer configuration to a firecracker-go-sdk
// JailerConfig pointer suitable for use in sdk.Config.JailerCfg.
func (r *resolvedJailerConfig) toSDKConfig() *sdk.JailerConfig {
	uid := r.uid
	gid := r.gid
	jcfg := &sdk.JailerConfig{
		ID:            r.id,
		UID:           &uid,
		GID:           &gid,
		ExecFile:      r.execFile,
		ChrootBaseDir: r.chrootBaseDir,
		CgroupVersion: r.cgroupVersion,
		Daemonize:     r.daemonize,
	}
	if r.numaNode >= 0 {
		numaNode := r.numaNode
		jcfg.NumaNode = &numaNode
	}
	return jcfg
}
