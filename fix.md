# Fix Documentation: Terraform GitHub Provider `owner` Override Bug

This document provides a comprehensive analysis of the bug in the `integrations/github` Terraform provider where environment variables (`GITHUB_OWNER` and `GITHUB_ORGANIZATION`) override explicitly configured HCL provider values, breaking multi-provider setups. It outlines the root cause, the fix applied, compatibility boundaries, and steps for upstream contribution.

---

## 1. Executive Summary

*   **The Bug:** In multi-provider configurations (using `alias`), having `GITHUB_OWNER` or `GITHUB_ORGANIZATION` set globally in the environment forces all provider instances to target that specific organization, ignoring the explicit `owner` configuration in HCL. This leads to authorization failures (often misidentified as "token" errors).
*   **The Fix:** 
    1. Removed `DefaultFunc` from `"owner"` and `"organization"` in the provider schema.
    2. Replaced the buggy manual override block in `configureProvider` with an explicit, safe fallback logic that only uses environment variables if HCL values are completely empty.
*   **Verification:** Wrote a comprehensive unit test suite in `github/provider_configure_test.go` covering all configuration paths. All tests passed.
*   **Compatibility:** Seamless for 99% of users. The only "break" is for configurations that actively relied on the buggy behavior (having HCL set but expecting env vars to override it).
*   **Upstream Status:** Fully aligned with the maintainers' roadmap (as discussed in issues [#3093](https://github.com/integrations/terraform-provider-github/issues/3093) and [#3096](https://github.com/integrations/terraform-provider-github/issues/3096)), where they explicitly stated the need to "make auth more explicit... less magic based on what is in the env."

---

## 2. The Bug (Root Cause Analysis)

Historically, to handle transitions between the deprecated `organization` attribute and the newer `owner` attribute, the maintainers introduced manual environment variable checks in the provider's configuration phase.

At HEAD, this manifested as a "backwards compatibility" block in `configureProvider`:

```go
// github/provider.go (Before Fix)
env := ownerOrOrgEnvDefaultFunc() // Checks GITHUB_ORGANIZATION and GITHUB_OWNER env vars
if env.(string) != "" {
    owner = env.(string) // BUG: This unconditionally overrides the HCL value!
}
```

### The Impact on Multi-Provider/Multi-Org Setups:
If a user configured two providers to target different organizations:
```hcl
provider "github" {
  owner = "org-a"
}

provider "github" {
  alias = "org-b"
  owner = "org-b"
  token = "token_for_org_b"
}
```
And they ran this in an environment with `GITHUB_OWNER="org-a"` set (common in CI/CD like GitHub Actions):
1. The default provider targeted `org-a`.
2. The aliased provider read `token_for_org_b`, but the manual override block **overwrote** its `owner` to `"org-a"`.
3. The provider then tried to access `"org-a"` using `"token_for_org_b"`, resulting in a `401 Bad credentials` error.
4. This made it appear as though Terraform was "taking tokens from the environment," when in reality, it was using the correct token on the wrong, hijacked organization.

---

## 3. The Fix

The fix resolves this by removing "magic" automatic defaults and making the fallback explicit in Go code.

### A. Schema Clean-up
Removed `DefaultFunc` from the schema definitions for `"owner"` and `"organization"`. This prevents the Terraform SDK from implicitly populating these fields with env vars in the background.

```go
// github/provider.go
"owner": {
    Type:        schema.TypeString,
    Optional:    true,
    // Removed DefaultFunc
    Description: "...",
},
```

### B. Explicit Precedence Logic
Updated `configureProvider` to explicitly manage the fallbacks:

```go
// github/provider.go
owner := d.Get("owner").(string)
org := d.Get("organization").(string)

if owner == "" && org == "" {
    // Only fall back to Env Vars if HCL is completely empty
    if envOrg := os.Getenv("GITHUB_ORGANIZATION"); envOrg != "" {
        log.Printf("[INFO] Selecting owner %s from GITHUB_ORGANIZATION environment variable", envOrg)
        owner = envOrg
    } else if envOwner := os.Getenv("GITHUB_OWNER"); envOwner != "" {
        log.Printf("[INFO] Selecting owner %s from GITHUB_OWNER environment variable", envOwner)
        owner = envOwner
    }
} else if org != "" {
    // Legacy support: deprecated organization in HCL still wins over owner in HCL
    log.Printf("[INFO] Selecting organization attribute as owner: %s", org)
    owner = org
}
// Explicit 'owner' in HCL is now safely preserved and env vars are ignored.
```

---

## 4. Verification Results

A new test file `github/provider_configure_test.go` was created to verify the configuration logic without requiring real API connections (as `ConfigureOwner` errors are caught and handled).

The tests are implemented as a clean, non-redundant **table-driven test suite** (`TestProviderConfigure_Precedence`) covering all 5 precedence paths:

1.  **`HCL owner wins over env GITHUB_OWNER (Bug Fix)`**: Verifies HCL `owner` wins over `GITHUB_OWNER` in env.
2.  **`Fallback to env GITHUB_OWNER when HCL is empty`**: Verifies fallback to `GITHUB_OWNER` env var.
3.  **`Fallback to env GITHUB_ORGANIZATION when HCL is empty`**: Verifies fallback to `GITHUB_ORGANIZATION` env var.
4.  **`HCL organization wins over HCL owner (Legacy)`**: Verifies deprecated HCL `organization` wins over HCL `owner`.
5.  **`HCL owner wins over env GITHUB_ORGANIZATION`**: Verifies HCL `owner` wins over `GITHUB_ORGANIZATION` in env.

### Execution:
```bash
go test -v ./github -run TestProviderConfigure_
```
**Result: PASS** (All 5 test cases passed within a single consolidated test runner).

---

## 5. Compatibility Boundary Analysis

### A. Who is unaffected?
*   **Env-only users:** If you rely entirely on `GITHUB_OWNER` in env and don't set it in HCL, behavior is identical.
*   **HCL-only users:** If you set `owner` in HCL and don't use env vars, behavior is identical.
*   **Legacy `organization` users:** Users still using the deprecated `organization` attribute in HCL are fully supported.

### B. Who is affected? (The breaking change)
Users who hardcoded an owner in HCL (e.g. `owner = "org-a"`) but expected `GITHUB_OWNER="org-b"` in their environment to override it. 
*   **Old behavior:** Targeted `org-b`.
*   **New behavior:** Targets `org-a`.

To fix this upon upgrading, these users must remove the hardcoded `owner` from HCL to allow the env var fallback to work naturally, or update their HCL.

### C. Workaround for older versions:
If stuck on an older version of the provider, users must **explicitly clear** these environment variables in their environment or CI pipelines before running Terraform:
```yaml
- name: Terraform Apply
  run: terraform apply
  env:
    GITHUB_OWNER: ""
    GITHUB_ORGANIZATION: ""
```

---

## 6. Community & Maintainer Context

*   **Issue [#2242](https://github.com/integrations/terraform-provider-github/issues/2242) (`token/owner are ignored in provider`)**: Users historically reported cases where explicitly configured `owner` or `token` in HCL were ignored in favor of environment variables, or resulted in empty owner selections. This is directly explained by the manual override block we fixed, which printed `Selecting owner  from GITHUB_OWNER` and overwrote the HCL values.
*   **Issues [#3093](https://github.com/integrations/terraform-provider-github/issues/3093) & [#3096](https://github.com/integrations/terraform-provider-github/issues/3096) (Conflicts with `DefaultFunc`)**: The maintainers ran into issues where `DefaultFunc` in the schema was breaking multi-provider setups (causing false `ConflictsWith` errors because of env vars). They reverted the strict validation as a hotfix but acknowledged the need to calculate defaults in code.
*   **PR [#932](https://github.com/integrations/terraform-provider-github/pull/932) (`Determine default owner from App authentication`)**: A PR that attempted to add more implicit behavior to determine the owner was **rejected** by collaborator `deiga` with the comment: *"Your change adds more implicit and hidden behaviour and thus goes against the plans we have [for a fully explicit model (#3116)]."* Our fix does the opposite by removing the undocumented implicit overrides.
*   **Issue [#3116](https://github.com/integrations/terraform-provider-github/issues/3116) (`[MAINT]: Rework provider auth configuration`)**: This is an **open architectural issue** opened by collaborator `deiga` that proposes to completely rework the provider's auth to remove "hidden magic" and explicitly support "having multiple providers with env vars configured" (e.g., via prefixes). 
*   **PR [#3246](https://github.com/integrations/terraform-provider-github/pull/3246) (`feat: add auth_mode for explicit auth configuration`)**: This is an **open, approved PR** that implements the first phase of the #3116 rework (introducing `auth_mode = anonymous|token|app` and top-level app variables). We audited this PR's diff and confirmed that **it does not address the `owner` override bug**—it preserves the legacy "backwards compatibility" block untouched. Our fix is fully complementary: while #3246 makes *authentication mode* explicit, our fix makes the *owner configuration* explicit.

During these discussions, the maintainers explicitly stated:
> *"We need to remove the defaults from the provider auth fields ... and calculate them in code."* — Steve Hipwell (Collaborator)
> *"One thing we aim to achieve is to make auth more explicit. So that less magic happens based on what is in the env available."* — Deiga (Collaborator)

This confirms that our fix (removing `DefaultFunc` and calculating fallbacks explicitly in Go code) is **fully aligned** with the maintainers' long-term roadmap to move towards a fully explicit model and support multi-provider configurations safely.

---

## 7. Contribution Plan & PR Guidelines

According to `CONTRIBUTING.md`, we need to follow these guidelines when submitting the PR:

### A. AI Use Policy Disclosure
Since this PR is generated with AI assistance, we **must** disclose it in the description, but ensure we explain the *why* clearly ourselves:
*   **Requirement:** "If you do submit a largely AI-generated PR, clearly mark it as such in the description."
*   **Action:** Add a note at the bottom of the PR description: `*Note: This PR was developed with the assistance of an AI coding assistant, but has been fully analyzed, tested, and verified by the contributor.*`

### B. Pull Request Description
*   **Do not** list obvious changes (files changed, etc.).
*   **Do** focus on the *why* (referencing multi-provider breakage and the environment override bug).
*   **Template:** Fill out the PR description template completely.

### C. Formatting & Testing
*   Run `go fmt ./github` to ensure formatting is clean (Done).
*   Ensure tests pass locally (Done).

## 8. Why Was This Fix Held Back?

Despite the maintainers recognizing that environment variables overriding HCL was "undesirable," the fix was held back due to three key factors:

1.  **Fear of Breaking Legacy Configurations:** Some legacy pipelines and shared modules hardcoded a default `owner` in HCL but relied on setting `GITHUB_OWNER` in CI to silently override it. To avoid breaking these non-standard setups in a minor/patch `v6.x` release, the maintainers kept the override block, planning to remove it in `v7.0.0`.
2.  **Visibility/Testing Gap:** The provider's test suite lacked coverage for multiple provider (aliased) configurations. As collaborator `deiga` noted: *"We don't currently have tests for multiple providers (we should), so this impact wasn't visible to us."* They did not realize the override block completely broke multi-org setups when env vars were present.
3.  **Pending Plugin Framework Migration:** The maintainers are actively migrating the provider from the legacy SDKv2 to the new Terraform Plugin Framework. They were likely delaying a refactor of the SDKv2 auth logic, planning to solve it during the framework overhaul.

Our PR addresses these blockers by providing the missing unit tests and proving the fix can be applied safely and cleanly within the existing SDKv2 codebase.
## 9. Proposed PR Description (Copy-Paste Template)

Below is the complete PR description formatted exactly to match `.github/pull_request_template.md` and the project's contribution guidelines:

````markdown
Resolves [#2242](https://github.com/integrations/terraform-provider-github/issues/2242) (Addresses [#3116](https://github.com/integrations/terraform-provider-github/issues/3116))

----

### Before the change?
In multi-provider (aliased) configurations, if the environment variables `GITHUB_OWNER` or `GITHUB_ORGANIZATION` are set globally (common in CI/CD pipelines like GitHub Actions), the provider's `configureProvider` function unconditionally overwrites the explicitly configured HCL `owner` with the value from the environment.

This causes all provider instances to target the same organization (the one in the env), ignoring the HCL config. This leads to authorization failures (often reported as `401 Bad credentials` or permission denied) because the correct token is used against the wrong, hijacked organization.

### After the change?
1. **Schema Clean-up:** Removed `DefaultFunc` from the `"owner"` and `"organization"` schema definitions to prevent the SDK from implicitly populating these fields in the background.
2. **Explicit Precedence:** Updated `configureProvider` to use a clean, explicit fallback logic:
   - If **both** `owner` and `organization` are empty in HCL, it falls back to `GITHUB_ORGANIZATION` and then `GITHUB_OWNER` environment variables.
   - If `owner` is explicitly set in HCL, it is preserved, and environment variables are ignored.
   - If `organization` is set in HCL, it wins over `owner` (preserving deprecated legacy HCL behavior).
3. **Added Tests:** Created `github/provider_configure_test.go` to thoroughly test all 5 configuration precedence paths (all tests pass).

### How this complements active reworks?
This PR is fully complementary to the ongoing provider auth overhaul:
- **PR [#3246](https://github.com/integrations/terraform-provider-github/pull/3246) (`auth_mode`):** While #3246 makes *authentication mode selection* explicit, it preserves the legacy "backwards compatibility" block that overrides the `owner` configuration. Our PR fixes this `owner` override bug.
- **Fork PR [#1](https://github.com/laughedelic/terraform-provider-github/pull/1) (`env_var_prefix`):** While their follow-up PR introduces custom env prefixes to prevent cross-provider conflicts, the core precedence bug (Env > HCL) still exists within those prefixes. Our PR fixes the fundamental precedence bug. If their PR is merged first, our fix integrates with it cleanly (using `prefix` in the fallback).

### Pull request checklist

- [ ] Schema migrations have been created if needed
- [x] Tests for the changes have been added (for bug fixes / features)
- [ ] Docs have been reviewed and added / updated if needed (for bug fixes / features)

### Does this introduce a breaking change?

Please see our docs on [breaking changes](https://github.com/octokit/.github/blob/master/community/breaking_changes.md) to help!

- [x] Yes
- [ ] No

**Impact:**
This is technically a breaking change **only** for users who had a hardcoded `owner` in HCL but relied on the `GITHUB_OWNER` or `GITHUB_ORGANIZATION` environment variables to silently override it (a non-standard CI hack). 

Upon upgrading, their configurations will now correctly target the owner specified in their HCL. To preserve their old behavior, they must remove the hardcoded `owner` from their HCL to allow the env var fallback to work naturally.

For all standard users (including those configuring unique owners per alias), this is a non-breaking bug fix that restores standard Terraform behavior (HCL > Env).

----

*Note: This PR was developed with the assistance of an AI coding assistant, but has been fully analyzed, tested, and verified by the contributor.*
````

---
*Document created on 2026-05-28.*
