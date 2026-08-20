package dateparsa

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The commit-msg hook is shell, and these tests run the file in .githooks as a
// subprocess rather than reimplementing its rules here. A second copy of the
// rules in Go is a second thing that can disagree with the shell, and what has
// to be true is that the file git actually runs refuses these messages.
//
// Nothing else in this package shells out. The whole of the precedent is
// `sh <hook> <file>` plus an exit status, and it stays that small on purpose.

// The hook lives beside this file, so the test's own path is the repository
// root. Deriving it that way rather than walking up looking for go.mod means
// the test cannot be fooled by being run from somewhere unexpected.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not report this test's own path")
	}
	return filepath.Dir(self)
}

func hookPath(t *testing.T) string {
	t.Helper()
	p := filepath.Join(repoRoot(t), ".githooks", "commit-msg")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("hook not found at %s: %v", p, err)
	}
	return p
}

// runHook writes msg to a file and runs the hook over it, the way git does.
func runHook(t *testing.T, msg string) (ok bool, output string) {
	t.Helper()
	f := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
	if err := os.WriteFile(f, []byte(msg), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("sh", hookPath(t), f).CombinedOutput()
	return err == nil, string(out)
}

// A header of exactly 72 is allowed and 73 is not, so both are built from the
// same prefix rather than written out and counted by hand.
const hdrPrefix = "feat(detect): "

func headerOfLen(n int) string {
	return hdrPrefix + strings.Repeat("a", n-len(hdrPrefix))
}

func TestCommitMsgHook(t *testing.T) {
	tests := []struct {
		name   string
		msg    string
		accept bool
	}{
		// Accepted.
		{"subject and body", "feat(detect): add duration signatures\n\nA body that explains it.\n", true},
		{"subject alone", "chore(fmt): run gofumpt\n", true},
		{"breaking change marker", "feat(detect)!: read bare DD/MM/YYYY as day-first\n\nBREAKING CHANGE: inputs move.\n", true},
		{"exactly 72", headerOfLen(72) + "\n\nA body.\n", true},
		{
			"git's trailing comment lines",
			"fix(compile): reject a 5-digit year\n\nA body.\n\n# Please enter the commit message for your changes.\n# On branch main\n",
			true,
		},
		{
			"a comment before the subject",
			"# Please enter the commit message for your changes.\nfeat(detect): add ISO 8601 duration signatures\n\nA body.\n",
			true,
		},

		{
			// Dependabot's body is markdown and no human can rewrite it. The
			// header rules still apply to it; the body rules cannot.
			"a dependency bump signed by the bot",
			"ci(deps): bump softprops/action-gh-release\n\n" +
				"Updates [action-gh-release](https://github.com/softprops/action-gh-release) from 2.6.2 to 3.0.2.\n\n" +
				"Signed-off-by: dependabot[bot] <support@github.com>\n",
			true,
		},
		{
			// The exemption is the bot's signature, not the scope, so a person
			// writing ci(deps) by hand still gets the rules.
			"a human writing ci(deps) with a markdown link",
			"ci(deps): bump something\n\nSee [the notes](https://example.com).\n",
			false,
		},

		// Refused.
		{
			"subject wrapped across two lines",
			"feat(parse): detect the layout once and reuse it across every row parsed\ntoday\n\nA body.\n",
			false,
		},
		{"body with no blank line before it", "fix(epoch): bound the range\nThis is meant to be the body.\n", false},
		{"73 characters", headerOfLen(73) + "\n\nA body.\n", false},
		{"not a Conventional Commit", "made the parser faster\n\nA body.\n", false},
		{"subject ends with a period", "fix(epoch): bound the range.\n\nA body.\n", false},
		{"markdown heading", "docs(readme): note the bound\n\n## Heading\n", false},
		{"markdown table", "docs(readme): note the bound\n\n| a | b |\n", false},
		{"task list", "docs(readme): note the bound\n\n- [ ] a thing\n", false},
		{"fenced code block", "docs(readme): note the bound\n\n```go\nx := 1\n```\n", false},
		{"markdown emphasis", "docs(readme): note the bound\n\nThis is **important** to know.\n", false},
		{"markdown link", "docs(readme): note the bound\n\nSee [the docs](https://example.com).\n", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ok, out := runHook(t, tc.msg)
			if ok != tc.accept {
				verb := map[bool]string{true: "accepted", false: "refused"}
				t.Fatalf("hook %s this message, want %s:\n---\n%s---\nhook said:\n%s",
					verb[ok], verb[tc.accept], tc.msg, out)
			}
		})
	}
}

// The refusal has to name the subject git would record. The first line is the
// thing that looked fine; the joined one is the thing that is wrong, and a
// message quoting the first line sends the reader to the wrong place.
func TestCommitMsgHookQuotesTheJoinedSubject(t *testing.T) {
	const first = "feat(parse): detect the layout once and reuse it across every row parsed"
	joined := first + " today"

	ok, out := runHook(t, first+"\ntoday\n\nA body.\n")
	if ok {
		t.Fatal("hook accepted a subject git records as 78 characters")
	}
	if !strings.Contains(out, joined) {
		t.Errorf("refusal does not quote the subject git would record (%q):\n%s", joined, out)
	}
	if !strings.Contains(out, "78") {
		t.Errorf("refusal does not give the recorded length of 78:\n%s", out)
	}
}

// GitHub appends " (#123)" to the header when a pull request is squashed, and
// the hook never sees that: the message it checks is written on this machine and
// the suffix is added on GitHub's. A 69-character subject entered this history
// at 75 that way, and the history test below caught it, which is the only reason
// this is a rule rather than a surprise.
func TestCommitMsgHookDiscountsASquashSuffix(t *testing.T) {
	const subject = "chore(ci): take a squash header from the commit, not the pull request"
	if len(subject) != 69 {
		t.Fatalf("the fixture is %d characters, not the 69 this test is about", len(subject))
	}

	squashed := subject + " (#10)"
	if len(squashed) != 75 {
		t.Fatalf("the squashed fixture is %d characters, not 75", len(squashed))
	}
	if ok, out := runHook(t, squashed+"\n\nA body.\n"); !ok {
		t.Errorf("hook refuses a header GitHub squashed, which the author cannot prevent:\n%s", out)
	}

	// The discount is the suffix and nothing else: the same length written by
	// hand is still refused, or the rule has been widened to 78 by accident.
	sameLength := subject + " today"
	if len(sameLength) != 75 {
		t.Fatalf("the control fixture is %d characters, not 75", len(sameLength))
	}
	if ok, _ := runHook(t, sameLength+"\n\nA body.\n"); ok {
		t.Error("hook accepted 75 characters that no squash produced")
	}

	// Only a number is a pull request reference. Anything else in the same
	// shape is text the author wrote and is measured.
	notARef := subject + " (#ten)"
	if ok, _ := runHook(t, notARef+"\n\nA body.\n"); ok {
		t.Errorf("hook discounted %q, which is not a pull request reference", " (#ten)")
	}
}

// The rule that matters more than refusing bad messages: a gate that refuses
// the project's own history is a gate somebody turns off. Skipped rather than
// failed where the history is not there, because CI checks out one commit.
func TestCommitMsgHookAcceptsTheExistingHistory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	cmd := exec.Command("git", "-C", repoRoot(t), "log", "-50", "--format=%B%x00")
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("cannot read the log, which a shallow clone cannot: %v", err)
	}
	msgs := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	if len(msgs) < 10 {
		t.Skipf("only %d commits available, too few to be worth asserting on", len(msgs))
	}

	for _, m := range msgs {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		subject := strings.SplitN(m, "\n", 2)[0]
		if ok, hookOut := runHook(t, m+"\n"); !ok {
			t.Errorf("the hook refuses a commit already in this history:\n  %s\n%s", subject, hookOut)
		}
	}
}
