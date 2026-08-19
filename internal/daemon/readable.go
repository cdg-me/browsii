package daemon

import (
	_ "embed"
)

// Readability.js and isProbablyReaderable, vendored from mozilla/readability
// (Apache-2.0), pinned at the commit fetched on 2026-08-17. Evaluated in the
// page context: the module-export guards are inert there, leaving Readability
// and isProbablyReaderable as globals within the eval scope.
var (
	//go:embed readability/Readability.js
	readabilityJS string

	//go:embed readability/readerable.js
	readerableJS string
)

// readableRunJS gates on isProbablyReaderable, then parses a clone of the
// document. document.cloneNode(true) is used rather than DOMParser because
// a cloned document keeps the live document's base URI, which Readability
// relies on to make link and image URLs absolute. Returns JSON with ok,
// title, byline, and text.
var readableRunJS = `() => {` + readabilityJS + `
` + readerableJS + `
	if (typeof isProbablyReaderable !== 'function' || !isProbablyReaderable(document)) {
		return JSON.stringify({ok: false});
	}
	try {
		const article = new Readability(document.cloneNode(true)).parse();
		if (!article || !article.textContent || article.textContent.trim().length === 0) {
			return JSON.stringify({ok: false});
		}
		// article.textContent joins block elements without line breaks.
		// Render the parsed content offscreen and read innerText, which
		// reflects block boundaries; detach immediately after.
		// (visibility:hidden would make innerText fall back to textContent,
		// so the holder is only moved offscreen.)
		const holder = document.createElement('div');
		holder.style.cssText = 'position:absolute;left:-99999px;top:0';
		holder.innerHTML = article.content;
		document.documentElement.appendChild(holder);
		let text = holder.innerText || article.textContent;
		holder.remove();
		return JSON.stringify({
			ok: true,
			title: article.title || '',
			byline: article.byline || '',
			text: text.trim(),
		});
	} catch (e) {
		return JSON.stringify({ok: false});
	}
}`
