package github

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestProviderConfigure_Precedence(t *testing.T) {
	tests := []struct {
		name         string
		envOwner     string
		envOrg       string
		configOwner  any
		configOrg    any
		expectedName string
	}{
		{
			name:         "HCL owner wins over env GITHUB_OWNER (Bug Fix)",
			envOwner:     "env-org",
			configOwner:  "explicit-org",
			expectedName: "explicit-org",
		},
		{
			name:         "Fallback to env GITHUB_OWNER when HCL is empty",
			envOwner:     "env-org",
			expectedName: "env-org",
		},
		{
			name:         "Fallback to env GITHUB_ORGANIZATION when HCL is empty",
			envOrg:       "env-org",
			expectedName: "env-org",
		},
		{
			name:         "HCL organization wins over HCL owner (Legacy)",
			configOwner:  "explicit-owner",
			configOrg:    "explicit-org",
			expectedName: "explicit-org",
		},
		{
			name:         "HCL owner wins over env GITHUB_ORGANIZATION",
			envOrg:       "env-org",
			configOwner:  "explicit-owner",
			expectedName: "explicit-owner",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := NewProvider()()

			// Set env vars
			t.Setenv("GITHUB_OWNER", tc.envOwner)
			t.Setenv("GITHUB_ORGANIZATION", tc.envOrg)

			// Prepare HCL config map
			config := map[string]interface{}{
				"token": "dummy-token",
			}
			if tc.configOwner != nil {
				config["owner"] = tc.configOwner
			}
			if tc.configOrg != nil {
				config["organization"] = tc.configOrg
			}

			d := schema.TestResourceDataRaw(t, p.Schema, config)

			configureFunc := configureProvider()
			meta, diags := configureFunc(context.Background(), d)
			if diags.HasError() {
				t.Fatalf("diags has error: %v", diags)
			}

			owner := meta.(*Owner)
			if owner.name != tc.expectedName {
				t.Errorf("expected owner to be %q, got %q", tc.expectedName, owner.name)
			}
		})
	}
}
