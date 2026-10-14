package cli

import (
	"math/rand"
	"testing"
)

// Feature: interactive-tui-mode, Property 1: Bare invocation routes to the TUI
//
// Property 1: Bare invocation routes to the TUI
//
// For any argument slice whose first argument after the program name (args[1])
// is neither "list" nor "delete" (including a slice with no such argument),
// classify returns cmdTUI; a first argument of "list" or "delete" returns
// cmdList or cmdDelete respectively.
//
// Validates: Requirements 1.1, 1.2, 1.3
func TestClassify_Property1_BareInvocationRoutesToTUI(t *testing.T) {
	// nonMatching is a pool of first tokens that are NOT the "list" or
	// "delete" subcommands. It mixes empty strings, flags, case variants that
	// must not match (classify is case-sensitive), and near-miss tokens.
	nonMatching := []string{
		"", "-n", "--help", "-h", "LIST", "List", "DELETE", "Delete",
		"deletex", "listx", "lis", "del", "listing", "deleted", "tui",
		"5", "foo", "bar", "--domain", "x", " list", "list ", " delete",
	}

	// expected returns the command classify should produce for the given
	// args, derived independently from the property statement rather than from
	// classify's implementation.
	expected := func(args []string) command {
		if len(args) > 1 {
			switch args[1] {
			case "list":
				return cmdList
			case "delete":
				return cmdDelete
			}
		}
		return cmdTUI
	}

	rng := rand.New(rand.NewSource(1))

	const iterations = 500
	for i := 0; i < iterations; i++ {
		args := []string{"prog"}

		// Choose the first token after the program name. With some
		// probability pick "list"/"delete" so both the matching and
		// non-matching branches are exercised; otherwise draw a non-matching
		// token or a random string.
		var first string
		var hasFirst bool
		switch pick := rng.Intn(10); {
		case pick == 0:
			// No first argument: bare invocation ({"prog"}).
			hasFirst = false
		case pick == 1:
			first, hasFirst = "list", true
		case pick == 2:
			first, hasFirst = "delete", true
		case pick <= 5:
			first, hasFirst = nonMatching[rng.Intn(len(nonMatching))], true
		default:
			first, hasFirst = randToken(rng), true
		}

		if hasFirst {
			args = append(args, first)
		}

		// Append a random number of trailing args; these must never affect the
		// classification, which depends only on args[1].
		for j, extra := 0, rng.Intn(4); j < extra; j++ {
			args = append(args, randToken(rng))
		}

		got := classify(args)
		want := expected(args)
		if got != want {
			t.Fatalf("iteration %d: classify(%#v) = %v, want %v", i, args, got, want)
		}
	}

	// Explicitly cover the two bare-invocation edge cases called out in the
	// property: no first argument and an empty slice both route to the TUI.
	if got := classify([]string{"prog"}); got != cmdTUI {
		t.Fatalf("classify({\"prog\"}) = %v, want cmdTUI", got)
	}
	if got := classify([]string{}); got != cmdTUI {
		t.Fatalf("classify({}) = %v, want cmdTUI", got)
	}
}

// randToken builds a short random token. It may occasionally equal "list" or
// "delete", which is fine: the independent oracle classifies those correctly,
// so such draws simply add coverage of the matching branches.
func randToken(rng *rand.Rand) string {
	// With small probability emit an empty token.
	if rng.Intn(8) == 0 {
		return ""
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-0123456789"
	n := 1 + rng.Intn(8)
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[rng.Intn(len(alphabet))]
	}
	return string(b)
}
