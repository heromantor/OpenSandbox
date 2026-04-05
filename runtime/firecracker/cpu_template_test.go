package firecracker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCPUTemplateValidate_None(t *testing.T) {
	cfg := CPUTemplateConfig{}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected nil for empty config, got: %v", err)
	}
}

func TestCPUTemplateValidate_StaticT2(t *testing.T) {
	cfg := CPUTemplateConfig{Static: TemplateT2}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected nil for T2, got: %v", err)
	}
}

func TestCPUTemplateValidate_StaticT2S(t *testing.T) {
	cfg := CPUTemplateConfig{Static: TemplateT2S}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected nil for T2S, got: %v", err)
	}
}

func TestCPUTemplateValidate_StaticC3(t *testing.T) {
	cfg := CPUTemplateConfig{Static: TemplateC3}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected nil for C3, got: %v", err)
	}
}

func TestCPUTemplateValidate_InvalidStatic(t *testing.T) {
	cfg := CPUTemplateConfig{Static: StaticTemplate("X99")}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid static template, got nil")
	}
	if !strings.Contains(err.Error(), "X99") {
		t.Errorf("error should mention the invalid template name, got: %v", err)
	}
}

func TestCPUTemplateValidate_CustomPathExists(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "custom-template.json")
	if err := os.WriteFile(tmpFile, []byte(`{"cpuid_modifiers": []}`), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	cfg := CPUTemplateConfig{CustomPath: tmpFile}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected nil for valid custom path, got: %v", err)
	}
}

func TestCPUTemplateValidate_CustomPathMissing(t *testing.T) {
	cfg := CPUTemplateConfig{CustomPath: "/nonexistent/template.json"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing custom path, got nil")
	}
}

func TestCPUTemplateValidate_BothSet(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "custom-template.json")
	if err := os.WriteFile(tmpFile, []byte(`{}`), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	cfg := CPUTemplateConfig{
		Static:     TemplateT2,
		CustomPath: tmpFile,
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when both static and custom are set, got nil")
	}
	if !strings.Contains(err.Error(), "both") {
		t.Errorf("error should mention 'both', got: %v", err)
	}
}

func TestCPUTemplateIsSet_Empty(t *testing.T) {
	cfg := CPUTemplateConfig{}
	if cfg.IsSet() {
		t.Error("expected IsSet()=false for empty config")
	}
}

func TestCPUTemplateIsSet_Static(t *testing.T) {
	cfg := CPUTemplateConfig{Static: TemplateT2}
	if !cfg.IsSet() {
		t.Error("expected IsSet()=true for static template")
	}
}

func TestCPUTemplateIsSet_Custom(t *testing.T) {
	cfg := CPUTemplateConfig{CustomPath: "/some/path.json"}
	if !cfg.IsSet() {
		t.Error("expected IsSet()=true for custom path")
	}
}
