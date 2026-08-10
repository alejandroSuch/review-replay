package main

import (
	"os"
	"testing"
)

func withArgs(t *testing.T, argv ...string) (args, error) {
	t.Helper()
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })
	os.Args = append([]string{"review-replay"}, argv...)
	return parseArgs()
}

// Flags placed after the PR are the form every caller uses (the pr-review loop and
// the pr-review workflow both run `review-replay <pr> --json --no-llm`). An earlier
// build stopped parsing at the positional, so --no-llm was silently dropped and the
// run died asking for a provider API key it had been told not to need.
func TestFlagsAfterThePositionalAreParsed(t *testing.T) {
	a, err := withArgs(t, "gitkraken/merge-mate#345", "--json", "--no-llm")
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if a.pr != "gitkraken/merge-mate#345" {
		t.Errorf("pr = %q", a.pr)
	}
	if !a.jsonOut || !a.noLLM {
		t.Errorf("jsonOut = %v, noLLM = %v, want both true", a.jsonOut, a.noLLM)
	}
}

func TestFlagsOnEitherSideOfThePositional(t *testing.T) {
	a, err := withArgs(t, "--no-llm", "https://github.com/o/r/pull/7", "--reviewer", "someone")
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if a.pr != "https://github.com/o/r/pull/7" {
		t.Errorf("pr = %q", a.pr)
	}
	if !a.noLLM || a.reviewer != "someone" {
		t.Errorf("noLLM = %v, reviewer = %q", a.noLLM, a.reviewer)
	}
}

func TestASecondPositionalIsRejected(t *testing.T) {
	if _, err := withArgs(t, "o/r#1", "o/r#2"); err == nil {
		t.Fatal("want an error for two PR arguments")
	}
}

func TestAMissingPositionalIsRejected(t *testing.T) {
	if _, err := withArgs(t, "--no-llm"); err == nil {
		t.Fatal("want an error when no PR is given")
	}
}

// --help and --ping-llm return before the PR argument is required.
func TestModesThatNeedNoPR(t *testing.T) {
	for _, flag := range []string{"--help", "--ping-llm"} {
		if _, err := withArgs(t, flag); err != nil {
			t.Errorf("%s: %v", flag, err)
		}
	}
}
