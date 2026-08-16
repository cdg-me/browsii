package daemon

import (
	"testing"

	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/gson"
)

func TestFingerprintOf(t *testing.T) {
	a := elementInfo{Tag: "button", Role: "button", Text: "Submit order", Selector: "#a"}
	b := elementInfo{Tag: "button", Role: "button", Text: "Submit order", Selector: "body > div > button:nth-of-type(3)"}
	if fingerprintOf(a) != fingerprintOf(b) {
		t.Fatal("identity fields match, selector differs — fingerprints must be equal")
	}

	c := elementInfo{Tag: "button", Role: "button", Text: "Submit payment"}
	if fingerprintOf(a) == fingerprintOf(c) {
		t.Fatal("different text must produce different fingerprints")
	}

	// Volatile fields must not participate: value/checked/disabled/visible
	// change legitimately without the element being replaced.
	d := a
	d.Value = "typed"
	d.Checked = new(bool)
	d.Disabled = true
	d.Visible = false
	if fingerprintOf(a) != fingerprintOf(d) {
		t.Fatal("volatile fields must not affect the fingerprint")
	}

	if fingerprintOf(elementInfo{}) == "" {
		t.Fatal("zero element must still produce a (separable) fingerprint")
	}
}

func TestDefaultCaptureBody(t *testing.T) {
	jsonHdr := proto.NetworkHeaders{"Content-Type": gson.New("application/json")}
	formHdr := proto.NetworkHeaders{"content-type": gson.New("application/x-www-form-urlencoded")} //nolint:goconst // header casing varies by client
	binHdr := proto.NetworkHeaders{"Content-Type": gson.New("image/png")}

	cases := []struct {
		name string
		req  proto.NetworkRequest
		want bool
	}{
		{"json POST", proto.NetworkRequest{Method: "POST", Headers: jsonHdr, PostData: `{"a":1}`}, true},
		{"form POST case-insensitive header", proto.NetworkRequest{Method: "POST", Headers: formHdr, PostData: "a=1"}, true},
		{"text PUT", proto.NetworkRequest{Method: "PUT", Headers: proto.NetworkHeaders{"Content-Type": gson.New("text/plain")}, PostData: "hi"}, true},
		{"no content-type POST", proto.NetworkRequest{Method: "POST", PostData: "a=1"}, true},
		{"PATCH json", proto.NetworkRequest{Method: "PATCH", Headers: jsonHdr, PostData: "{}"}, true},
		{"GET never", proto.NetworkRequest{Method: "GET", Headers: jsonHdr, PostData: "a=1"}, false},
		{"binary POST", proto.NetworkRequest{Method: "POST", Headers: binHdr, PostData: "pngbytes"}, false},
		{"oversized POST", proto.NetworkRequest{Method: "POST", Headers: jsonHdr, PostData: string(make([]byte, maxDefaultBodyBytes+1))}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := defaultCaptureBody(&tc.req); got != tc.want {
				t.Fatalf("defaultCaptureBody(%s %s) = %v, want %v", tc.req.Method, tc.req.Headers, got, tc.want)
			}
		})
	}
}

func TestSelectorTokens(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{"id selector", "#submit-btn", []string{"submit", "btn"}},
		{"class selector", ".cta.primary", []string{"cta", "primary"}},
		{"attribute selector", `input[name="email"]`, []string{"input", "name", "email"}},
		{"compound", "button.Submit.order", []string{"button", "submit", "order"}},
		{"single chars dropped", "#a.b", nil},
		{"empty", "", nil},
		{"punctuation only", "###", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := selectorTokens(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("selectorTokens(%q) = %v, want %v", tc.input, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("selectorTokens(%q) = %v, want %v", tc.input, got, tc.want)
				}
			}
		})
	}
}

func TestFindCandidates(t *testing.T) {
	elems := []elementInfo{
		{Ref: 1, Tag: "a", Role: "link", Text: "Home", Selector: "#logo", Visible: true},
		{Ref: 2, Tag: "button", Role: "button", Text: "Submit order", Selector: "#order-btn", Visible: true},
		{Ref: 3, Tag: "button", Role: "button", Text: "Submit payment", Selector: "#pay-btn", Visible: true, Disabled: true},
		{Ref: 4, Tag: "button", Role: "button", Text: "Cancel", Selector: "#cancel-btn", Visible: true},
		{Ref: 5, Tag: "input", Role: "textbox", Name: "Email address", Selector: "#email", Visible: true},
		{Ref: 6, Tag: "button", Role: "button", Text: "Secret action", Selector: "body > div > button", Visible: false},
	}

	t.Run("matches by text tokens, best first, notes attached", func(t *testing.T) {
		got := findCandidates(elems, ".submit", 5)
		if len(got) == 0 {
			t.Fatal("expected candidates, got none")
		}
		if got[0].Text != "Submit order" && got[0].Text != "Submit payment" {
			t.Fatalf("best candidate = %q, want a Submit button", got[0].Text)
		}
		// Both submit buttons must appear in the top matches.
		texts := map[string]bool{}
		for _, c := range got {
			texts[c.Text] = true
		}
		if !texts["Submit order"] || !texts["Submit payment"] {
			t.Fatalf("expected both submit buttons, got %v", got)
		}
		// The disabled one carries a note.
		for _, c := range got {
			if c.Text == "Submit payment" && c.Note != "disabled" {
				t.Fatalf("expected note %q on Submit payment, got %q", "disabled", c.Note)
			}
		}
	})

	t.Run("matches by name and selector tokens", func(t *testing.T) {
		got := findCandidates(elems, "#email-input", 5)
		if len(got) == 0 {
			t.Fatal("expected the email textbox to match")
		}
		if got[0].Ref != 5 {
			t.Fatalf("best candidate ref = %d, want 5", got[0].Ref)
		}
	})

	t.Run("marks hidden elements", func(t *testing.T) {
		got := findCandidates(elems, "secret", 5)
		if len(got) != 1 {
			t.Fatalf("expected 1 candidate, got %d", len(got))
		}
		if got[0].Note != "hidden" {
			t.Fatalf("expected note %q, got %q", "hidden", got[0].Note)
		}
	})

	t.Run("no match returns nil", func(t *testing.T) {
		if got := findCandidates(elems, ".zzz-nothing", 5); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("limit is respected", func(t *testing.T) {
		many := make([]elementInfo, 10)
		for i := range many {
			many[i] = elementInfo{Ref: i + 1, Tag: "button", Role: "button", Text: "go button", Selector: "#b"}
		}
		got := findCandidates(many, "button", 3)
		if len(got) != 3 {
			t.Fatalf("expected 3 candidates, got %d", len(got))
		}
	})

	t.Run("ties break by ascending ref", func(t *testing.T) {
		tie := []elementInfo{
			{Ref: 7, Tag: "button", Role: "button", Text: "Submit order", Selector: "#a", Visible: true},
			{Ref: 3, Tag: "button", Role: "button", Text: "Submit order", Selector: "#b", Visible: true},
			{Ref: 5, Tag: "button", Role: "button", Text: "Submit order", Selector: "#c", Visible: true},
		}
		got := findCandidates(tie, ".submit", 5)
		if len(got) != 3 {
			t.Fatalf("expected 3 candidates, got %d", len(got))
		}
		if got[0].Ref != 3 || got[1].Ref != 5 || got[2].Ref != 7 {
			t.Fatalf("expected refs ordered 3,5,7 — got %d,%d,%d", got[0].Ref, got[1].Ref, got[2].Ref)
		}
	})
}
