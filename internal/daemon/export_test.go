package daemon

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRecordedEventRoundTrip(t *testing.T) {
	events := []RecordedEvent{
		{T: 100, Action: "navigate", Params: map[string]interface{}{"url": "http://x/"}},
		{T: 200, Action: "click", Params: map[string]interface{}{"selector": "#a"},
			FP: &elementIdentity{Tag: "button", Role: "button", Text: "Add to cart"}, FPIndex: 1},
		{T: 300, Action: "expect", Params: map[string]interface{}{"text": "Total: $30"}, TimeoutMs: 5000},
	}

	data, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	var back []RecordedEvent
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}

	if len(back) != 3 {
		t.Fatalf("got %d events", len(back))
	}
	if back[0].Action != "navigate" || back[0].T != 100 {
		t.Errorf("navigate mismatch: %+v", back[0])
	}
	if back[1].FP == nil || back[1].FP.Text != "Add to cart" || back[1].FPIndex != 1 {
		t.Errorf("fingerprint lost: %+v", back[1])
	}
	if back[2].Action != "expect" || back[2].TimeoutMs != 5000 {
		t.Errorf("expect mismatch: %+v", back[2])
	}

	encoded := string(data)
	if !strings.Contains(encoded, `"fp"`) || !strings.Contains(encoded, `"fpIndex":1`) {
		t.Errorf("json field names wrong: %s", encoded)
	}
	if strings.Contains(encoded, `"fpIndex":0`) {
		t.Errorf("zero fpIndex must be omitted: %s", encoded)
	}
}

func TestBuildPlaywrightSpec(t *testing.T) {
	events := []RecordedEvent{
		{T: 1, Action: "navigate", Params: map[string]interface{}{"url": "http://shop/"}},
		{T: 2, Action: "click", Params: map[string]interface{}{"selector": "#p2-add"},
			FP: &elementIdentity{Tag: "button", Role: "button", Text: "Add to cart"}},
		{T: 3, Action: "type", Params: map[string]interface{}{"selector": "#email", "text": "a@b.c"},
			FP: &elementIdentity{Tag: "input", Role: "textbox", Name: "Email"}},
		{T: 4, Action: "expect", Params: map[string]interface{}{"text": "Total: $30"}, TimeoutMs: 4000},
		{T: 5, Action: "expect", Params: map[string]interface{}{"urlPattern": "*/orders/*"}},
		{T: 6, Action: "expect", Params: map[string]interface{}{"selector": ".spin", "hidden": true}},
		{T: 7, Action: "expect", Params: map[string]interface{}{"selector": "#email", "value": "a@b.c"}},
		{T: 8, Action: "screenshot", Params: map[string]interface{}{"filename": "x.png"}},
	}

	spec := buildPlaywrightSpec("http://shop/", "/tmp/shop.har", events)

	want := []string{
		`import { test, expect } from '@playwright/test';`,
		`await page.routeFromHAR("/tmp/shop.har"`,
		`await page.goto("http://shop/")`,
		`page.getByRole("button", { name: "Add to cart" })`,
		`// recorded selector: #p2-add`,
		`.fill("a@b.c")`,
		`getByText("Total: $30")).toBeVisible({ timeout: 4000 });`,
		`toHaveURL(/.*\/orders\/.*/)`,
		`toBeHidden()`,
		`toHaveValue("a@b.c")`,
		`// skipped: screenshot`,
	}
	for _, w := range want {
		if !strings.Contains(spec, w) {
			t.Errorf("spec missing %q\n%s", w, spec)
		}
	}

	if strings.Contains(spec, "test('recorded flow'") == false && !strings.Contains(spec, `test("shop"`) {
		t.Errorf("test name not derived from har: %s", spec)
	}
}

func TestBuildPlaywrightSpec_NoHarNoFP(t *testing.T) {
	events := []RecordedEvent{
		{T: 1, Action: "click", Params: map[string]interface{}{"selector": "#plain"}},
	}
	spec := buildPlaywrightSpec("", "", events)
	if !strings.Contains(spec, `page.locator("#plain").click()`) {
		t.Errorf("plain selector must map to locator: %s", spec)
	}
	if strings.Contains(spec, "routeFromHAR") {
		t.Errorf("no har must not emit routeFromHAR: %s", spec)
	}
}

func TestGlobToSpecRegex(t *testing.T) {
	cases := map[string]string{
		"*/orders/*": `.*\/orders\/.*`,
		"/a/b":       `\/a\/b`,
		"*":          `.*`,
	}
	for in, want := range cases {
		if got := globToSpecRegex(in); got != want {
			t.Errorf("globToSpecRegex(%q) = %q, want %q", in, got, want)
		}
	}
}
