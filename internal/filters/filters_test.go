package filters

import "testing"

func TestIsClassifiableText(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"plain lgtm", "LGTM", false},
		{"thanks", "Thanks!", false},
		{"emoji", "👍", false},
		{"approved", "approved", false},
		{"empty", "", false},
		{"substantive", "LGTM. v1→v2 manifest mapping and selectVisibleChanges filtering are internally consistent.", true},
		{"only quotes", "> ## Blocking\n> None\n> ## Non-blocking\n> - some bullet", false},
		{"longer with nits", "LGTM. A few non-blocking nits below: please consider extracting the constant.", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := IsClassifiableText(c.body)
			if got != c.want {
				t.Fatalf("IsClassifiableText(%q) = %v, want %v", c.body, got, c.want)
			}
		})
	}
}

func TestStripQuotedLines(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"> quote\nactual content here", "actual content here"},
		{">> deeply\n> shallow\nreal text", "real text"},
		{"real start\n\n\n\nreal end", "real start\n\nreal end"},
		{"> a\n> b\n> c", ""},
	}
	for _, c := range cases {
		got := StripQuotedLines(c.in)
		if got != c.want {
			t.Fatalf("StripQuotedLines(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBodySimilarity(t *testing.T) {
	t.Run("identical", func(t *testing.T) {
		if got := BodySimilarity("hello world test", "hello world test"); got != 1 {
			t.Fatalf("identical bodies: got %f, want 1", got)
		}
	})
	t.Run("paraphrased", func(t *testing.T) {
		a := "Please rename the variable buffer to pendingWriteBuffer."
		b := "rename buffer to pendingWriteBuffer please"
		if got := BodySimilarity(a, b); got <= 0.5 {
			t.Fatalf("paraphrased: got %f, want > 0.5", got)
		}
	})
	t.Run("unrelated", func(t *testing.T) {
		if got := BodySimilarity("rebase against main first", "extract this constant"); got >= 0.2 {
			t.Fatalf("unrelated: got %f, want < 0.2", got)
		}
	})
}
