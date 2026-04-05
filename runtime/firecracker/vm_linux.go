//go:build linux

package firecracker

import (
	"fmt"

	sdk "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
)

// toFirecrackerConfig translates a VMConfig into a firecracker-go-sdk Config
// suitable for creating a Machine. If JailerEnabled, the JailerCfg is populated
// from resolveJailerConfig.
func (c *VMConfig) toFirecrackerConfig() (sdk.Config, error) {
	// Build the root drive. When ReadOnlyRootfs is true, pass WithReadOnly
	// so Firecracker opens the ext4 file read-only — multiple VMs can then
	// safely share the same image file (Phase 2, IMG-03).
	var drives []models.Drive
	if c.ReadOnlyRootfs {
		drives = sdk.NewDrivesBuilder("").
			WithRootDrive(c.RootfsPath, sdk.WithReadOnly(true)).
			Build()
	} else {
		drives = sdk.NewDrivesBuilder(c.RootfsPath).Build()
	}

	cfg := sdk.Config{
		VMID:            c.ID,
		SocketPath:      c.socketPath(),
		KernelImagePath: c.KernelImagePath,
		KernelArgs:      c.KernelArgs,
		Drives:          drives,
		MachineCfg: models.MachineConfiguration{
			VcpuCount:       sdk.Int64(c.VCPUs),
			MemSizeMib:      sdk.Int64(c.MemoryMiB),
			TrackDirtyPages: c.TrackDirtyPages,
		},
		// Disable SDK-level validation since we validate at the VMConfig level
		// and the kernel/rootfs may not exist on this host (e.g., macOS dev).
		DisableValidation: true,
	}

	// Apply static CPU template if configured.
	if c.CPUTemplate.Static != TemplateNone {
		cfg.MachineCfg.CPUTemplate = models.CPUTemplate(c.CPUTemplate.Static)
	}

	// Build jailer configuration if enabled.
	if c.JailerEnabled {
		resolved, err := resolveJailerConfig(c.ID, c.Jailer, c.FirecrackerBin)
		if err != nil {
			return sdk.Config{}, fmt.Errorf("firecracker: resolve jailer config: %w", err)
		}
		cfg.JailerCfg = resolved.toSDKConfig()
	}

	return cfg, nil
}
