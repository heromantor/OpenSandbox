package firecracker

// StaticTemplate represents a built-in CPU template name.
type StaticTemplate string

// CPUTemplateConfig holds configuration for CPU template selection.
// Either Static or CustomPath may be set, but not both.
type CPUTemplateConfig struct {
	// Static is one of T2, T2S, C3, or empty for no static template.
	Static StaticTemplate
	// CustomPath is the path to a custom CPU template JSON file.
	// Mutually exclusive with Static.
	CustomPath string
}
