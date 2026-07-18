package main

import (
	"encoding/xml"
	"os"
	"strings"
	"testing"
)

func TestWindowsExecutablesDeclarePerMonitorDPIAwareness(t *testing.T) {
	for _, path := range []string{
		"build/windows/cusus.exe.manifest",
		"build/windows/supervisor.exe.manifest",
	} {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var document struct{}
			if err := xml.Unmarshal(data, &document); err != nil {
				t.Fatalf("invalid manifest XML: %v", err)
			}
			manifest := string(data)
			for _, declaration := range []string{
				`<dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware>`,
				`<dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2,PerMonitor</dpiAwareness>`,
			} {
				if !strings.Contains(manifest, declaration) {
					t.Errorf("manifest is missing %s", declaration)
				}
			}
		})
	}
}
