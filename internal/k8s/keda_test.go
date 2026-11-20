package k8s

import (
	"os"
	"path/filepath"
	"testing"

	"helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/repo/v1"
)

func TestLoadRepoFileCreatesNewFileWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "helm", "repositories.yaml")

	f, err := loadRepoFile(path)
	if err != nil {
		t.Fatalf("loadRepoFile() error = %v", err)
	}
	if f.APIVersion != repo.APIVersionV1 {
		t.Errorf("loadRepoFile() APIVersion = %q, want %q", f.APIVersion, repo.APIVersionV1)
	}

	f.Update(&repo.Entry{Name: "kedacore", URL: "https://kedacore.github.io/charts"})
	if err := f.WriteFile(path, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("repository file was not created: %v", err)
	}
}

func TestReleaseIsDeployed(t *testing.T) {
	tests := []struct {
		name    string
		release any
		want    bool
		wantErr bool
	}{
		{
			name: "deployed",
			release: &releasev1.Release{
				Info: &releasev1.Info{Status: common.StatusDeployed},
			},
			want: true,
		},
		{
			name: "not deployed",
			release: &releasev1.Release{
				Info: &releasev1.Info{Status: common.StatusPendingInstall},
			},
		},
		{
			name:    "unexpected type",
			release: "not a Helm release",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := releaseIsDeployed(tt.release)
			if (err != nil) != tt.wantErr {
				t.Fatalf("releaseIsDeployed() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("releaseIsDeployed() = %v, want %v", got, tt.want)
			}
		})
	}
}
