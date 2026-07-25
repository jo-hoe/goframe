package controller

import (
	"strings"
	"testing"

	goframev1alpha1 "github.com/jo-hoe/goframe/internal/operator/api/v1alpha1"
)

// TestBuildSchedulerConfig_ImageListHeaders verifies the operator renders imagelist
// headers into image-scheduler.yaml in the shape the scheduler binary parses.
func TestBuildSchedulerConfig_ImageListHeaders(t *testing.T) {
	gf := &goframev1alpha1.GoFrame{}
	gf.Name = "gf"
	gf.Namespace = "default"

	sched := goframev1alpha1.SchedulerSpec{
		Name:   "natgeo",
		Cron:   "0 8 * * *",
		Source: "imagelist",
		ImageList: &goframev1alpha1.ImageListConfig{
			Headers: []goframev1alpha1.HTTPHeader{
				{Name: "User-Agent", Value: "Mozilla/5.0 (compatible)"},
				{Name: "Referer", Value: "https://example.com/"},
			},
		},
	}

	out, err := buildSchedulerConfig(gf, sched)
	if err != nil {
		t.Fatalf("buildSchedulerConfig failed: %v", err)
	}
	for _, want := range []string{
		"headers:",
		"name: User-Agent",
		"value: Mozilla/5.0 (compatible)",
		"name: Referer",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered config missing %q\n---\n%s", want, out)
		}
	}
}
