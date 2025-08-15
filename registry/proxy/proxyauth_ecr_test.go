package proxy

import (
	"testing"
	"time"

	"github.com/distribution/distribution/v3/configuration"
)

func TestParseECRURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantAccount string
		wantRegion  string
		wantErr     bool
	}{
		{
			name:        "valid ECR URL",
			url:         "https://123456789012.dkr.ecr.us-west-2.amazonaws.com",
			wantAccount: "123456789012",
			wantRegion:  "us-west-2",
			wantErr:     false,
		},
		{
			name:        "valid ECR URL with path",
			url:         "https://123456789012.dkr.ecr.eu-central-1.amazonaws.com/my-repo",
			wantAccount: "123456789012",
			wantRegion:  "eu-central-1",
			wantErr:     false,
		},
		{
			name:    "non-ECR URL",
			url:     "https://registry-1.docker.io",
			wantErr: true,
		},
		{
			name:    "invalid URL",
			url:     "not-a-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account, region, err := parseECRURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseECRURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if account != tt.wantAccount {
					t.Errorf("parseECRURL() account = %v, want %v", account, tt.wantAccount)
				}
				if region != tt.wantRegion {
					t.Errorf("parseECRURL() region = %v, want %v", region, tt.wantRegion)
				}
			}
		})
	}
}

func TestIsECRURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "valid ECR URL",
			url:  "https://123456789012.dkr.ecr.us-west-2.amazonaws.com",
			want: true,
		},
		{
			name: "Docker Hub URL",
			url:  "https://registry-1.docker.io",
			want: false,
		},
		{
			name: "generic registry URL",
			url:  "https://my-registry.com",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isECRURL(tt.url); got != tt.want {
				t.Errorf("isECRURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigureECRAuth(t *testing.T) {
	cfg := configuration.ECRConfig{
		AccessKeyID:     "test-key",
		SecretAccessKey: "test-secret",
		Region:          "us-west-2",
		AccountID:       "123456789012",
		Lifetime:        func() *time.Duration { d := time.Hour; return &d }(),
	}

	_, err := configureECRAuth(cfg, "https://123456789012.dkr.ecr.us-west-2.amazonaws.com")
	if err != nil {
		t.Errorf("configureECRAuth() error = %v", err)
	}
}
