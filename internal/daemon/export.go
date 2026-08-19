package daemon

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// buildPlaywrightSpec renders a recording as a Playwright TypeScript spec.
// Element actions with a fingerprint become role-based locators; expects
// become assertions; a recorded HAR becomes routeFromHAR. Actions with no
// Playwright equivalent are emitted as comments so nothing is silently
// dropped.
func buildPlaywrightSpec(url, har string, events []RecordedEvent) string {
	var b strings.Builder

	b.WriteString("import { test, expect } from '@playwright/test';\n\n")

	name := "recorded flow"
	if har != "" {
		name = strings.TrimSuffix(filepath.Base(har), ".har")
	}
	fmt.Fprintf(&b, "test(%q, async ({ page }) => {\n", name)

	if har != "" {
		fmt.Fprintf(&b, "  await page.routeFromHAR(%q, { url: \"*\" });\n", har)
	}
	if url != "" {
		fmt.Fprintf(&b, "  await page.goto(%q);\n", url)
	}
	if url != "" {
		for _, ev := range events {
			if ev.Action == "navigate" && paramString(ev.Params, "url") == url {
				ev.Params["__emitted__"] = true
			} else {
				break
			}
		}
	}

	for _, ev := range events {
		if paramBool(ev.Params, "__emitted__") {
			continue
		}
		switch ev.Action {
		case "expect":
			writeExpectSpec(&b, ev)
		case "navigate":
			fmt.Fprintf(&b, "  await page.goto(%q);\n", paramString(ev.Params, "url"))
		case "click":
			fmt.Fprintf(&b, "  await %s.click();\n", specLocatorWithComment(&b, ev))
		case "hover":
			fmt.Fprintf(&b, "  await %s.hover();\n", specLocatorWithComment(&b, ev))
		case "type":
			fmt.Fprintf(&b, "  await %s.fill(%q);\n", specLocatorWithComment(&b, ev), paramString(ev.Params, "text"))
		case "fill":
			if raw, ok := ev.Params["fields"].([]interface{}); ok {
				for _, rf := range raw {
					m, _ := rf.(map[string]interface{})
					if m == nil {
						continue
					}
					sel, _ := m["selector"].(string)
					val, _ := m["value"].(string)
					fev := RecordedEvent{
						Params: m,
						FP:     fpFromMap(m),
					}
					if sel != "" {
						fev.Params = map[string]interface{}{"selector": sel}
					}
					fmt.Fprintf(&b, "  await %s.fill(%q);\n", specLocatorWithComment(&b, fev), val)
				}
			}
		case "select":
			fmt.Fprintf(&b, "  await %s.selectOption(%q);\n", specLocatorWithComment(&b, ev), paramString(ev.Params, "value"))
		case "check":
			if paramBool(ev.Params, "checked") {
				fmt.Fprintf(&b, "  await %s.check();\n", specLocatorWithComment(&b, ev))
			} else {
				fmt.Fprintf(&b, "  await %s.uncheck();\n", specLocatorWithComment(&b, ev))
			}
		case "press":
			fmt.Fprintf(&b, "  await page.keyboard.press(%q);\n", paramString(ev.Params, "key"))
		case "scroll":
			dir := paramString(ev.Params, "direction")
			pixels := 300
			if p, ok := ev.Params["pixels"].(float64); ok && p > 0 {
				pixels = int(p)
			}
			switch dir {
			case "down":
				fmt.Fprintf(&b, "  await page.mouse.wheel(0, %d);\n", pixels)
			case "up":
				fmt.Fprintf(&b, "  await page.mouse.wheel(0, -%d);\n", pixels)
			case "top":
				b.WriteString("  await page.evaluate(() => window.scrollTo(0, 0));\n")
			case "bottom":
				b.WriteString("  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));\n")
			}
		case "reload":
			b.WriteString("  await page.reload();\n")
		case "back":
			b.WriteString("  await page.goBack();\n")
		case "forward":
			b.WriteString("  await page.goForward();\n")
		case "js":
			fmt.Fprintf(&b, "  await page.evaluate(%q);\n", paramString(ev.Params, "script"))
		default:
			fmt.Fprintf(&b, "  // skipped: %s %v\n", ev.Action, ev.Params)
		}
	}

	b.WriteString("});\n")
	return b.String()
}

// specLocator renders the locator expression for an element action. With a
// fingerprint it uses getByRole plus accessible name, disambiguating
// repeated elements with .nth(fpIndex); the recorded CSS selector is kept
// as a comment line above for debugging.
func specLocator(ev RecordedEvent) string {
	selector := paramString(ev.Params, "selector")
	if ev.FP == nil {
		return fmt.Sprintf("page.locator(%q)", selector)
	}
	name := ev.FP.Name
	if name == "" {
		name = ev.FP.Text
	}
	if name == "" {
		return fmt.Sprintf("page.locator(%q)", selector)
	}
	expr := fmt.Sprintf("page.getByRole(%q, { name: %q })", ev.FP.Role, name)
	if ev.FPIndex > 0 {
		expr += fmt.Sprintf(".nth(%d)", ev.FPIndex-1)
	}
	return expr
}

// specLocatorWithComment pairs specLocator with its recorded selector as a
// leading comment line.
func specLocatorWithComment(b *strings.Builder, ev RecordedEvent) string {
	if ev.FP == nil {
		return specLocator(ev)
	}
	selector := paramString(ev.Params, "selector")
	fmt.Fprintf(b, "  // recorded selector: %s\n", selector)
	return specLocator(ev)
}

// writeExpectSpec renders a recorded expect event as a Playwright assertion.
func writeExpectSpec(b *strings.Builder, ev RecordedEvent) {
	timeout := ""
	if ev.TimeoutMs > 0 {
		timeout = fmt.Sprintf("{ timeout: %d }", ev.TimeoutMs)
	}
	switch {
	case paramString(ev.Params, "text") != "":
		fmt.Fprintf(b, "  await expect(page.getByText(%q)).toBeVisible(%s);\n", paramString(ev.Params, "text"), timeout)
	case paramString(ev.Params, "textGone") != "":
		fmt.Fprintf(b, "  await expect(page.getByText(%q)).toBeHidden(%s);\n", paramString(ev.Params, "textGone"), timeout)
	case paramString(ev.Params, "urlPattern") != "":
		pattern := paramString(ev.Params, "urlPattern")
		regex := globToSpecRegex(pattern)
		fmt.Fprintf(b, "  await expect(page).toHaveURL(/%s/)%s;\n", regex, timeoutSuffix(timeout))
	case paramString(ev.Params, "value") != "":
		valueTimeout := ""
		if timeout != "" {
			valueTimeout = ", " + timeout
		}
		fmt.Fprintf(b, "  await expect(page.locator(%q)).toHaveValue(%q%s);\n", paramString(ev.Params, "selector"), paramString(ev.Params, "value"), valueTimeout)
	case paramString(ev.Params, "selector") != "":
		if paramBool(ev.Params, "hidden") {
			fmt.Fprintf(b, "  await expect(page.locator(%q)).toBeHidden(%s);\n", paramString(ev.Params, "selector"), timeout)
		} else {
			fmt.Fprintf(b, "  await expect(page.locator(%q)).toBeVisible(%s);\n", paramString(ev.Params, "selector"), timeout)
		}
	default:
		fmt.Fprintf(b, "  // skipped expect: %v\n", ev.Params)
	}
}

// fpFromMap extracts an embedded fingerprint from a recorded field map.
func fpFromMap(m map[string]interface{}) *elementIdentity {
	raw, ok := m["fp"]
	if !ok {
		return nil
	}
	enc, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var id elementIdentity
	if json.Unmarshal(enc, &id) != nil {
		return nil
	}
	return &id
}

// timeoutSuffix renders the options argument for assertions that take it as
// a second parameter.
func timeoutSuffix(timeout string) string {
	if timeout == "" {
		return ""
	}
	return ", " + timeout
}

// globToSpecRegex converts a browsii glob (* wildcards) to a regex source
// string suitable for embedding in a toHaveURL literal.
func globToSpecRegex(pattern string) string {
	var b strings.Builder
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '/':
			b.WriteString("\\/")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
