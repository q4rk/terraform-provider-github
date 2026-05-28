package github

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestProviderConfigure_OwnerOverride(t *testing.T) {
	p := NewProvider()()

	t.Setenv("GITHUB_OWNER", "env-org")

	d := schema.TestResourceDataRaw(t, p.Schema, map[string]interface{}{
		"owner": "explicit-org",
		"token": "dummy-token",
	})

	configureFunc := configureProvider()
	meta, diags := configureFunc(context.Background(), d)
	if diags.HasError() {
		t.Fatalf("diags has error: %v", diags)
	}

	owner := meta.(*Owner)
	if owner.name != "explicit-org" {
		t.Errorf("expected owner to be 'explicit-org', got %q", owner.name)
	}
}

func TestProviderConfigure_EnvOwnerFallback(t *testing.T) {
	p := NewProvider()()

	t.Setenv("GITHUB_OWNER", "env-org")

	d := schema.TestResourceDataRaw(t, p.Schema, map[string]interface{}{
		"token": "dummy-token",
	})

	configureFunc := configureProvider()
	meta, diags := configureFunc(context.Background(), d)
	if diags.HasError() {
		t.Fatalf("diags has error: %v", diags)
	}

	owner := meta.(*Owner)
	if owner.name != "env-org" {
		t.Errorf("expected owner to be 'env-org', got %q", owner.name)
	}
}

func TestProviderConfigure_EnvOrgFallback(t *testing.T) {
	p := NewProvider()()

	t.Setenv("GITHUB_ORGANIZATION", "env-org")

	d := schema.TestResourceDataRaw(t, p.Schema, map[string]interface{}{
		"token": "dummy-token",
	})

	configureFunc := configureProvider()
	meta, diags := configureFunc(context.Background(), d)
	if diags.HasError() {
		t.Fatalf("diags has error: %v", diags)
	}

	owner := meta.(*Owner)
	if owner.name != "env-org" {
		t.Errorf("expected owner to be 'env-org', got %q", owner.name)
	}
}

func TestProviderConfigure_OrgWinsOverOwner_HCL(t *testing.T) {
	p := NewProvider()()

	d := schema.TestResourceDataRaw(t, p.Schema, map[string]interface{}{
		"owner":        "explicit-owner",
		"organization": "explicit-org",
		"token":        "dummy-token",
	})

	configureFunc := configureProvider()
	meta, diags := configureFunc(context.Background(), d)
	if diags.HasError() {
		t.Fatalf("diags has error: %v", diags)
	}

	owner := meta.(*Owner)
	if owner.name != "explicit-org" {
		t.Errorf("expected owner to be 'explicit-org', got %q", owner.name)
	}
}

func TestProviderConfigure_OwnerWinsOverEnvOrg(t *testing.T) {
	p := NewProvider()()

	t.Setenv("GITHUB_ORGANIZATION", "env-org")

	d := schema.TestResourceDataRaw(t, p.Schema, map[string]interface{}{
		"owner": "explicit-owner",
		"token": "dummy-token",
	})

	configureFunc := configureProvider()
	meta, diags := configureFunc(context.Background(), d)
	if diags.HasError() {
		t.Fatalf("diags has error: %v", diags)
	}

	owner := meta.(*Owner)
	if owner.name != "explicit-owner" {
		t.Errorf("expected owner to be 'explicit-owner', got %q", owner.name)
	}
}
