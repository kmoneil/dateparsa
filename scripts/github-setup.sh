#!/usr/bin/env bash
#
# Apply this project's GitHub settings: branch protection, the checks a pull
# request has to pass, and the security features that are off by default.
#
# Idempotent, and it prints what it is about to do. Run it after the repository
# exists and `gh auth status` is happy:
#
#   ./scripts/github-setup.sh
#   ./scripts/github-setup.sh --dry-run
#
# It never creates the repository and never pushes. Those are deliberate acts
# and belong to a human at a terminal.
set -euo pipefail

# The module path is github.com/kmoneil/dateparsa, so the repository has to be
# kmoneil/dateparsa. `go get` resolves the import path to a repository URL by
# reading it literally; a repository under any other name cannot serve this
# module without a vanity import host in front of it.
REPO=${REPO:-kmoneil/dateparsa}
DRY=${DRY:-}
[ "${1:-}" = "--dry-run" ] && DRY=1

say() { printf '%s\n' "$*" >&2; }

run() {
	if [ -n "$DRY" ]; then
		say "  would run: $*"
		return 0
	fi
	"$@"
}

command -v gh >/dev/null || {
	say "gh is not installed: https://cli.github.com"
	exit 2
}
gh auth status >/dev/null 2>&1 || {
	say "gh is not authenticated. Run: gh auth login"
	exit 2
}
gh repo view "$REPO" >/dev/null 2>&1 || {
	say "$REPO does not exist or is not visible to this account."
	say "Create it first. That is a deliberate act and this script will not do it:"
	say "  gh repo create $REPO --public --source . --remote origin"
	exit 2
}

# The contexts a pull request has to report before it can merge. Every name
# here is a job name in .github/workflows/ci.yml, and a required check that no
# job reports sits pending forever, making every pull request unmergeable
# including the one that would fix it. Keep this list and that file in step.
#
# Two jobs are deliberately absent.
#
# `benchmarks` runs on a shared runner whose timings are too noisy to gate a
# merge on. A flaky red teaches people to press re-run, which is how a real
# regression gets waved through.
#
# `fuzz` runs on push and in the nightly, never on a pull request, so requiring
# it here would require a context that only ever reports "skipped". The
# regression half of fuzzing is inside the test legs anyway, where `go test`
# runs the seed corpus and every committed crasher deterministically.
CHECKS=(
	"format, vet, lint"
	"test (ubuntu-latest)"
	"test (macos-latest)"
	"test (windows-latest)"
	"zero-alloc"
	"vulnerability scan"
	"size budget"
)

say "== ruleset on main =="
# A ruleset rather than the older branch-protection API: it is what GitHub
# develops now, and it can be read back as one object.
checks_json=$(printf '%s\n' "${CHECKS[@]}" | jq -R . | jq -s 'map({context: .})')
ruleset=$(
	jq -n --argjson checks "$checks_json" '{
    name: "main",
    target: "branch",
    enforcement: "active",
    conditions: {ref_name: {include: ["~DEFAULT_BRANCH"], exclude: []}},
    rules: [
      # No direct pushes: everything arrives by pull request.
      #
      # Zero approvals required, and not an oversight: GitHub does not let an
      # author approve their own pull request, so on a single-maintainer
      # repository a requirement of 1 locks the only maintainer out of their
      # own main branch. The pull request itself is what is being required
      # here, because it is what makes the checks run and the change readable
      # before it lands. Raise this to 1 on the day there is a second
      # maintainer, not before.
      {type: "pull_request", parameters: {
        required_approving_review_count: 0,
        dismiss_stale_reviews_on_push: true,
        require_code_owner_review: false,
        require_last_push_approval: false,
        required_review_thread_resolution: true
      }},
      {type: "required_status_checks", parameters: {
        strict_required_status_checks_policy: true,
        required_status_checks: $checks
      }},
      # The history is a list of changes worth reading, so no merge commits.
      {type: "required_linear_history"},
      {type: "non_fast_forward"},
      {type: "deletion"}
    ]
  }'
)
if [ -n "$DRY" ]; then
	say "  would apply this ruleset:"
	printf '%s\n' "$ruleset" | sed 's/^/    /' >&2
else
	existing=$(gh api "repos/$REPO/rulesets" --jq '.[] | select(.name=="main") | .id' 2>/dev/null || true)
	if [ -n "$existing" ]; then
		say "  updating ruleset $existing"
		printf '%s' "$ruleset" | gh api -X PUT "repos/$REPO/rulesets/$existing" --input - >/dev/null
	else
		say "  creating it"
		printf '%s' "$ruleset" | gh api -X POST "repos/$REPO/rulesets" --input - >/dev/null
	fi
fi

say "== tag protection =="
# A tag is a published release that the Go module proxy caches forever, so the
# only safe tag is one that was never wrong. This blocks moving or deleting a
# v* tag once it exists, which is the mistake that cannot be undone by hand.
tagruleset=$(
	jq -n '{
    name: "release tags",
    target: "tag",
    enforcement: "active",
    conditions: {ref_name: {include: ["refs/tags/v*"], exclude: []}},
    rules: [{type: "non_fast_forward"}, {type: "deletion"}, {type: "update"}]
  }'
)
if [ -n "$DRY" ]; then
	say "  would protect refs/tags/v* against update and deletion"
else
	existing=$(gh api "repos/$REPO/rulesets" --jq '.[] | select(.name=="release tags") | .id' 2>/dev/null || true)
	if [ -n "$existing" ]; then
		say "  updating ruleset $existing"
		printf '%s' "$tagruleset" | gh api -X PUT "repos/$REPO/rulesets/$existing" --input - >/dev/null
	else
		say "  creating it"
		printf '%s' "$tagruleset" | gh api -X POST "repos/$REPO/rulesets" --input - >/dev/null
	fi
fi

say "== merge behaviour =="
# Squash only. The subject comes from the pull request title, and the body from
# the commits on the branch.
#
# COMMIT_MESSAGES rather than PR_BODY, for two reasons that only show up later
# in `git log`. GitHub re-wraps a pull request description at about 70 columns
# on top of whatever wrapping it already has, so prose written at 72 arrives
# double-wrapped with breaks mid-sentence. And a description is markdown, read
# on a page that renders it, so its headings and checklists land in the commit
# as literal `## What was wrong` and `- [ ]`.
run gh api -X PATCH "repos/$REPO" \
	-F allow_squash_merge=true \
	-F allow_merge_commit=false \
	-F allow_rebase_merge=true \
	-F delete_branch_on_merge=true \
	-F squash_merge_commit_title=PR_TITLE \
	-F squash_merge_commit_message=COMMIT_MESSAGES \
	>/dev/null

say "== security features =="
# Dependabot alerts and secret scanning are off by default on a new repository.
# Push protection is the one that matters most: a token in a commit on a public
# repository is a token that has to be rotated, not deleted.
security=$(jq -n '{
  security_and_analysis: {
    secret_scanning: {status: "enabled"},
    secret_scanning_push_protection: {status: "enabled"}
  }
}')
if [ -n "$DRY" ]; then
	say "  would enable secret scanning and push protection"
else
	printf '%s' "$security" | gh api -X PATCH "repos/$REPO" --input - >/dev/null
fi
run gh api -X PUT "repos/$REPO/vulnerability-alerts" >/dev/null
run gh api -X PUT "repos/$REPO/automated-security-fixes" >/dev/null

say "== labels the workflows reference =="
# fuzz-nightly.yml opens an issue with --label fuzz, and gh fails the whole
# call if the label does not exist. Creating it here means the first find gets
# reported rather than lost.
if [ -n "$DRY" ]; then
	say "  would ensure labels: fuzz, dependencies, ci"
else
	gh label create fuzz --repo "$REPO" --color B60205 \
		--description "found by the fuzz sweep" --force >/dev/null
	gh label create dependencies --repo "$REPO" --color 0366D6 \
		--description "dependency or action bump" --force >/dev/null
	gh label create ci --repo "$REPO" --color 5319E7 \
		--description "build and continuous integration" --force >/dev/null
fi

say "== done =="
say "Verify with:  gh api repos/$REPO/rulesets --jq '.[].name'"
