package daemon

import (
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// isPiercingSelector reports whether sel addresses an element through a
// shadow-root or iframe chain ("host >>> inner").
func isPiercingSelector(sel string) bool {
	return strings.Contains(sel, ">>>")
}

// elementByPiercingSelector resolves sel (a ">>>"-chained selector) to a
// rod Element: the chain is evaluated in the page to locate the node, then
// the node is described (pierced) and materialized via DOMResolveNode so
// CDP-level actions (click, hover, visibility) work on it like any other
// element.
func elementByPiercingSelector(page *rod.Page, sel string) (*rod.Element, error) {
	// ByObject keeps the returned node as a remote object reference rather
	// than a serialized value, so its ObjectID can be described and
	// materialized into an Element.
	res, err := page.Evaluate(rod.Eval(`(sel) => {`+elementsHelpersJS+`
		return resolveOne(sel);
	}`, sel).ByObject())
	if err != nil {
		return nil, err
	}
	if res == nil || res.ObjectID == "" {
		return nil, &rod.ElementNotFoundError{}
	}

	node, err := proto.DOMDescribeNode{ObjectID: res.ObjectID, Pierce: true}.Call(page)
	if err != nil {
		return nil, err
	}
	if node == nil || node.Node == nil {
		return nil, &rod.ElementNotFoundError{}
	}
	return page.ElementFromNode(node.Node)
}

// elementForSelector is the single seam for locating elements: plain
// selectors use rod's waiting Element(); piercing chains resolve via
// elementByPiercingSelector with the same wait budget.
func elementForSelector(page *rod.Page, sel string, wait time.Duration) (*rod.Element, error) {
	if !isPiercingSelector(sel) {
		return page.Timeout(wait).Element(sel)
	}
	deadline := time.Now().Add(wait)
	for {
		el, err := elementByPiercingSelector(page, sel)
		if err == nil && el != nil {
			return el, nil
		}
		if time.Now().After(deadline) {
			return nil, &rod.ElementNotFoundError{}
		}
		time.Sleep(100 * time.Millisecond)
	}
}
