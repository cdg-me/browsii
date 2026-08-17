package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
)

// setupShopServer serves a shop page in four variants for replay tests:
//
//	canonical    — stable layout
//	drifted      — same elements with different ids, classes, nesting, and order
//	broken       — the checkout button replaced by a differently-labelled one
//	wrong-total  — the API returns a different total
//
// The drifted variant keeps every element's tag, role, text, name, and href
// identical so replay must relocate elements by fingerprint rather than
// selector.
func setupShopServer() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/total", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.Referer(), "variant=wrong-total") {
			fmt.Fprint(w, `{"total":"$300"}`) //nolint:errcheck
			return
		}
		fmt.Fprint(w, `{"total":"$30"}`) //nolint:errcheck
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		variant := r.URL.Query().Get("variant")

		var b strings.Builder
		b.WriteString(`<!DOCTYPE html><html><head><title>Shop</title></head><body>`)

		type product struct{ id, name, price string }
		products := []product{
			{"p1", "Widget", "$10"},
			{"p2", "Gadget", "$20"},
			{"p3", "Gizmo", "$30"},
		}

		if variant == "drifted" {
			b.WriteString(`<main class="catalog v2"><section id="listings">`)
			for _, p := range []product{products[2], products[0], products[1]} {
				fmt.Fprintf(&b, `<div class="listing" data-sku="%s"><div class="listing-inner"><span class="listing-name">%s</span><span class="listing-price">%s</span><div class="listing-actions"><button class="buy">Add to cart</button></div></div></div>`,
					p.id, p.name, p.price)
			}
			b.WriteString(`</section><footer class="totals"><div id="grand-total"></div><div class="pay"><span class="btn-wrap"><button class="buy confirm">Checkout</button></span></div></footer>`)
		} else {
			b.WriteString(`<div id="products">`)
			for _, p := range products {
				fmt.Fprintf(&b, `<div class="product" id="%s"><span class="name">%s</span><span class="price">%s</span><button id="%s-add">Add to cart</button></div>`,
					p.id, p.name, p.price, p.id)
			}
			b.WriteString(`</div><div id="cart"><span id="total"></span><button id="checkout">Checkout</button></div>`)
		}

		if variant == "broken" {
			b.WriteString(`<script>document.addEventListener('DOMContentLoaded', () => {
				const c = document.getElementById('checkout');
				if (c) { const r = document.createElement('button'); r.id = 'checkout'; r.textContent = 'Pay now'; c.replaceWith(r); }
			});</script>`)
		}

		b.WriteString(`<script>
			window.cartCount = 0;
			function refreshTotal() {
				fetch('/api/total').then(r => r.json()).then(d => {
					const el = document.getElementById('grand-total') || document.getElementById('total');
					el.textContent = 'Total: ' + d.total;
				});
			}
			document.addEventListener('click', e => {
				const b = e.target.closest('button');
				if (!b) return;
				if (b.id.endsWith('-add') || (b.classList.contains('buy') && !b.classList.contains('confirm'))) {
					window.cartCount++;
					refreshTotal();
				}
			});
		</script></body></html>`)

		fmt.Fprint(w, b.String()) //nolint:errcheck
	})
	return httptest.NewServer(mux)
}
