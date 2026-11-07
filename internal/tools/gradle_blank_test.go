package tools

import (
	"context"
	"strings"
	"testing"
)

func TestBuildStatusRejectsBlankJobID(t *testing.T) {
	_, err := buildStatus(context.Background(), buildStatusArgs{JobID: ""})
	if err == nil || !strings.Contains(err.Error(), "job_id is required") {
		t.Errorf("err = %v, want job_id is required", err)
	}
}

func TestGradleProjectPropertiesRejectsBlankModule(t *testing.T) {
	_, err := gradleProjectProperties(context.Background(), gradlePropertiesArgs{projectDirArg: projectDirArg{ProjectDir: t.TempDir()}, Module: " "})
	if err == nil || !strings.Contains(err.Error(), "module is required") {
		t.Errorf("err = %v, want module is required", err)
	}
}

func TestGetFileCoverageRejectsBlankFile(t *testing.T) {
	_, err := getFileCoverage(context.Background(), fileCoverageArgs{coverageTaskArgs: coverageTaskArgs{projectDirArg: projectDirArg{ProjectDir: t.TempDir()}}, File: ""})
	if err == nil || !strings.Contains(err.Error(), "file is required") {
		t.Errorf("err = %v, want file is required", err)
	}
}
