package channels

import "testing"

func TestDedashContent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"em-dash to hyphen", "wait — really?", "wait - really?"},
		{"em-dash unspaced", "word—word", "word-word"},
		{"horizontal bar to hyphen", "a ― b", "a - b"},
		{"en-dash range preserved", "2020–2024, Mon–Fri", "2020–2024, Mon–Fri"},
		{"multiple em-dashes", "a — b — c", "a - b - c"},
		{"no fancy dash is unchanged", "plain - hyphen only", "plain - hyphen only"},
		{"empty", "", ""},
		// Deliberate: no code-fence skipping — a stray em-dash in code is replaced too.
		{"em-dash inside code fence still replaced", "```\nx — y\n```", "```\nx - y\n```"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dedashContent(tc.in); got != tc.want {
				t.Errorf("dedashContent(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
