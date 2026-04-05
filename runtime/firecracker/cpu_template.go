package firecracker

import (
	"fmt"
	"os"
)

// StaticTemplate represents a built-in CPU template name.
type StaticTemplate string

const (
	// TemplateNone indicates no static CPU template is applied.
	TemplateNone StaticTemplate = ""
	// TemplateT2 is the T2 static CPU template for Intel hosts.
	TemplateT2 StaticTemplate = "T2"
	// TemplateT2S is the T2S static CPU template for Intel hosts.
	TemplateT2S StaticTemplate = "T2S"
	// TemplateC3 is the C3 static CPU template for AMD hosts.
	TemplateC3 StaticTemplate = "C3"
)

// validStaticTemplates enumerates the allowed static template values.
var validStaticTemplates = map[StaticTemplate]bool{
	TemplateT2:  true,
	TemplateT2S: true,
	TemplateC3:  true,
}

// CPUTemplateConfig holds configuration for CPU template selection.
// Either Static or CustomPath may be set, but not both.
type CPUTemplateConfig struct {
	// Static is one of T2, T2S, C3, or empty for no static template.
	Static StaticTemplate
	// CustomPath is the path to a custom CPU template JSON file.
	// Mutually exclusive with Static.
	CustomPath string
}

// Validate checks that the CPUTemplateConfig is valid. Returns an error if
// both Static and CustomPath are set, if Static is not a recognized template,
// or if CustomPath references a non-existent file.
func (c CPUTemplateConfig) Validate() error {
	if c.Static != TemplateNone && c.CustomPath != "" {
		return fmt.Errorf("firecracker: cpu template: cannot set both static and custom template")
	}
	if c.Static != TemplateNone {
		if !validStaticTemplates[c.Static] {
			return fmt.Errorf("firecracker: cpu template: unknown static template %q, must be one of T2, T2S, C3", c.Static)
		}
	}
	if c.CustomPath != "" {
		if _, err := os.Stat(c.CustomPath); err != nil {
			return fmt.Errorf("firecracker: cpu template: custom template file: %w", err)
		}
	}
	return nil
}

// IsSet returns true if either a static or custom CPU template is configured.
func (c CPUTemplateConfig) IsSet() bool {
	return c.Static != TemplateNone || c.CustomPath != ""
}
