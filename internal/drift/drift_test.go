package drift

import (
	"testing"

	"github.com/eduardofrafre/confdrift/internal/config"
)

func envs() []Env {
	return []Env{
		{"dev", config.Values{"A": `"1"`, "B": `"x"`, "C": `"same"`, "HOST": `"localhost"`}},
		{"prod", config.Values{"A": `"2"`, "C": `"same"`, "HOST": `"db.internal"`, "D": `"new"`}},
	}
}

func summarize(r Report) map[string]Kind {
	out := map[string]Kind{}
	for _, d := range r.Diffs {
		out[d.Key] = d.Kind
	}
	return out
}

func TestCompare(t *testing.T) {
	r := Compare(envs(), Options{})
	if r.Keys != 5 {
		t.Errorf("Keys = %d, want 5", r.Keys)
	}
	got := summarize(r)
	want := map[string]Kind{"A": Changed, "B": Missing, "D": Missing, "HOST": Changed}
	if len(got) != len(want) {
		t.Fatalf("diffs = %v, want %v", got, want)
	}
	for k, kind := range want {
		if got[k] != kind {
			t.Errorf("%s: got %q, want %q", k, got[k], kind)
		}
	}
	// Sorted by key, values aligned with envs, nil where absent.
	if r.Diffs[0].Key != "A" || r.Diffs[1].Key != "B" {
		t.Errorf("diffs not sorted: %v", r.Diffs)
	}
	b := r.Diffs[1]
	if *b.Values[0] != `"x"` || b.Values[1] != nil {
		t.Errorf("B values = %v", b.Values)
	}
}

func TestCompareOptions(t *testing.T) {
	r := Compare(envs(), Options{KeysOnly: true, Ignore: []string{"H*"}})
	got := summarize(r)
	if len(got) != 2 || got["B"] != Missing || got["D"] != Missing {
		t.Errorf("got %v, want only B and D missing", got)
	}
	if r.Keys != 4 {
		t.Errorf("ignored keys should not be counted: Keys = %d", r.Keys)
	}
}

func TestCompareIdentical(t *testing.T) {
	v := config.Values{"A": `1`}
	r := Compare([]Env{{"a", v}, {"b", v}, {"c", v}}, Options{})
	if len(r.Diffs) != 0 {
		t.Errorf("identical envs drifted: %v", r.Diffs)
	}
}

// With three environments a key can be both missing in one and different in
// the others; missing is the more urgent fact and wins.
func TestMissingWinsOverChanged(t *testing.T) {
	r := Compare([]Env{
		{"a", config.Values{"K": `1`}},
		{"b", config.Values{"K": `2`}},
		{"c", config.Values{}},
	}, Options{})
	if len(r.Diffs) != 1 || r.Diffs[0].Kind != Missing {
		t.Errorf("got %v", r.Diffs)
	}
}

func TestIgnoreGlobIsLiteral(t *testing.T) {
	// "." and "[" in a pattern are literal, not regexp syntax.
	r := Compare([]Env{
		{"a", config.Values{"db.host": `1`, "dbXhost": `1`, "s[0]": `1`}},
		{"b", config.Values{}},
	}, Options{Ignore: []string{"db.host", "s[0]"}})
	got := summarize(r)
	if len(got) != 1 || got["dbXhost"] != Missing {
		t.Errorf("got %v", got)
	}
}

func TestRedact(t *testing.T) {
	cases := []struct{ key, in, want string }{
		{"DB_PASSWORD", `"hunter2"`, Redacted},
		{"stripe.apiKey", `"sk_live"`, Redacted},
		{"GITHUB_TOKEN", `"ghp_x"`, Redacted},
		{"DATABASE_URL", `"postgres://app:s3cret@db:5432/app"`, `"postgres://app:<redacted>@db:5432/app"`},
		{"REDIS_URL", `"redis://:pw@cache:6379"`, `"redis://:<redacted>@cache:6379"`},
		{"PUBLIC_URL", `"https://example.com:8443/path"`, `"https://example.com:8443/path"`},
		{"LOG_LEVEL", `"debug"`, `"debug"`},
	}
	for _, c := range cases {
		if got := Redact(c.key, c.in); got != c.want {
			t.Errorf("Redact(%s, %s) = %s, want %s", c.key, c.in, got, c.want)
		}
	}
}
