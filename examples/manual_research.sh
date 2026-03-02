#!/bin/bash
set -e

# ============================================================
#  Browser CLI: Manual Research Script
#  Usage: ./examples/manual_research.sh "your research topic"
# ============================================================

TOPIC="${1:?Usage: $0 \"research topic\"}"
PORT=9010
DATE=$(date +%Y-%m-%d_%H%M%S)
SAFE_TOPIC=$(echo "$TOPIC" | tr ' ' '-' | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9-]//g')
OUTPUT_DIR="script-results/${SAFE_TOPIC}-${DATE}"
CLI="./browsii"

mkdir -p "$OUTPUT_DIR"

echo "============================================================"
echo " Deep Research: $TOPIC"
echo " Output: $OUTPUT_DIR"
echo "============================================================"

# --- Build ---
echo "[0/6] Building CLI..."
go build -o "$CLI" cmd/browsii/*.go

# --- Start daemon ---
echo "[1/6] Starting browser daemon (headful)..."
$CLI start --mode user-headful --port $PORT
sleep 3

cleanup() {
  echo ""
  echo "[cleanup] Shutting down daemon..."
  $CLI stop --port $PORT 2>/dev/null || true
}
trap cleanup EXIT

# --- Phase 1: DuckDuckGo Search (headful Chrome bypasses bot detection) ---
echo "[2/6] Searching DuckDuckGo for: $TOPIC"
ENCODED_QUERY=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$TOPIC'))")
$CLI navigate "https://duckduckgo.com/?q=${ENCODED_QUERY}" --port $PORT
sleep 4

# Extract search result URLs via JS
echo "[3/6] Extracting search result links..."
LINKS_JSON=$($CLI js "() => {
  const links = [];
  // DuckDuckGo uses data-testid='result-title-a' for result links
  const selectors = ['a[data-testid=result-title-a]', 'a.result__a', 'article a[href]'];
  let resultLinks = [];
  for (const sel of selectors) {
    resultLinks = document.querySelectorAll(sel);
    if (resultLinks.length > 0) break;
  }
  resultLinks.forEach(a => {
    const href = a.href;
    const text = a.innerText.trim().substring(0, 120);
    if (href && text.length > 3 && href.startsWith('http') && !href.includes('duckduckgo.com')) {
      links.push({url: href, title: text});
    }
  });
  // Deduplicate by URL
  const seen = new Set();
  return links.filter(l => {
    if (seen.has(l.url)) return false;
    seen.add(l.url);
    return true;
  }).slice(0, 10);
}" --port $PORT)

echo "$LINKS_JSON" | python3 -m json.tool > "$OUTPUT_DIR/00_search_results.json" 2>/dev/null || echo "$LINKS_JSON" > "$OUTPUT_DIR/00_search_results.json"

# Count how many links we got
NUM_LINKS=$(echo "$LINKS_JSON" | python3 -c "import json,sys; data=json.load(sys.stdin); print(len(data))" 2>/dev/null || echo "0")
echo "   Found $NUM_LINKS result links"

if [ "$NUM_LINKS" = "0" ]; then
  echo "   WARNING: No links extracted. Saving raw page for debugging..."
  $CLI scrape --port $PORT > "$OUTPUT_DIR/debug_search_raw.html" 2>/dev/null || true
fi

# --- Phase 2: Wikipedia ---
echo "[4/6] Fetching Wikipedia article..."
WIKI_QUERY=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$TOPIC'))")
$CLI tab new "https://en.wikipedia.org/w/index.php?search=${WIKI_QUERY}" --port $PORT
sleep 3

# Switch to the Wikipedia tab (should be index 1)
TABS_JSON=$($CLI tab list --port $PORT 2>/dev/null || echo "[]")
WIKI_TAB_IDX=$(echo "$TABS_JSON" | python3 -c "
import json,sys
tabs = json.load(sys.stdin) if sys.stdin.readable() else []
for t in tabs:
  if 'wikipedia' in t.get('url','').lower():
    print(t['index'])
    break
" 2>/dev/null || echo "1")

if [ -n "$WIKI_TAB_IDX" ]; then
  $CLI tab switch "$WIKI_TAB_IDX" --port $PORT 2>/dev/null || true
  sleep 1

  WIKI_CONTENT=$($CLI js "() => {
    const article = document.querySelector('#mw-content-text .mw-parser-output');
    if (!article) return {found: false, content: 'No article found'};
    
    // Extract a structured summary
    const sections = [];
    let currentSection = {heading: 'Introduction', paragraphs: []};
    
    for (const el of article.children) {
      if (el.tagName === 'H2' || el.tagName === 'H3') {
        if (currentSection.paragraphs.length > 0) {
          sections.push(currentSection);
        }
        const headingText = el.querySelector('.mw-headline')?.innerText || el.innerText;
        currentSection = {heading: headingText.trim(), paragraphs: []};
      } else if (el.tagName === 'P') {
        const text = el.innerText.trim();
        if (text.length > 20) {
          currentSection.paragraphs.push(text);
        }
      }
    }
    if (currentSection.paragraphs.length > 0) {
      sections.push(currentSection);
    }
    
    return {
      found: true,
      title: document.querySelector('#firstHeading')?.innerText || '',
      url: window.location.href,
      sections: sections.slice(0, 15)
    };
  }" --port $PORT)

  echo "$WIKI_CONTENT" | python3 -m json.tool > "$OUTPUT_DIR/01_wikipedia.json" 2>/dev/null || echo "$WIKI_CONTENT" > "$OUTPUT_DIR/01_wikipedia.json"
  echo "   Wikipedia: saved"
fi

# --- Phase 3: Open and scrape each Google result ---
echo "[5/6] Scraping individual sources..."

# Parse links and open each in a new tab
URLS=$(echo "$LINKS_JSON" | python3 -c "
import json, sys
try:
  data = json.load(sys.stdin)
  for item in data:
    print(item['url'])
except:
  pass
" 2>/dev/null)

SOURCE_IDX=0
while IFS= read -r url; do
  [ -z "$url" ] && continue
  SOURCE_IDX=$((SOURCE_IDX + 1))
  
  echo "   [$SOURCE_IDX] $url"
  
  # Open in new tab (daemon auto-switches to it)
  $CLI tab new "$url" --port $PORT 2>/dev/null || continue
  sleep 4
  
  # Extract article content via JS (runs on the newly active tab)
  CONTENT=$($CLI js "() => {
    // Try common article selectors
    const selectors = ['article', '[role=main]', 'main', '.post-content', '.entry-content', '.article-body', '#content', '.content'];
    let target = null;
    for (const sel of selectors) {
      target = document.querySelector(sel);
      if (target && target.innerText.trim().length > 200) break;
      target = null;
    }
    if (!target) target = document.body;
    
    // Extract text content, limiting to reasonable size
    const paragraphs = [];
    target.querySelectorAll('p, h1, h2, h3, li').forEach(el => {
      const text = el.innerText.trim();
      if (text.length > 20) {
        const tag = el.tagName.toLowerCase();
        paragraphs.push({tag: tag, text: text.substring(0, 500)});
      }
    });
    
    return {
      url: window.location.href,
      title: document.title,
      content: paragraphs.slice(0, 50)
    };
  }" --port $PORT 2>/dev/null || echo '{"error": "failed to scrape"}')
  
  PADDED_IDX=$(printf "%02d" $((SOURCE_IDX + 1)))
  echo "$CONTENT" | python3 -m json.tool > "$OUTPUT_DIR/${PADDED_IDX}_source.json" 2>/dev/null || echo "$CONTENT" > "$OUTPUT_DIR/${PADDED_IDX}_source.json"
  
done <<< "$URLS"

# --- Phase 4: Aggregate into final report ---
echo "[6/6] Aggregating research into final report..."

python3 -c "
import json, os, glob

output_dir = '$OUTPUT_DIR'
topic = '$TOPIC'

report_lines = []
report_lines.append(f'# Deep Research Report: {topic}')
report_lines.append(f'')
report_lines.append(f'Generated: $(date)')
report_lines.append(f'')
report_lines.append('---')
report_lines.append('')

# Wikipedia
wiki_path = os.path.join(output_dir, '01_wikipedia.json')
if os.path.exists(wiki_path):
    try:
        with open(wiki_path) as f:
            wiki = json.load(f)
        if wiki.get('found'):
            report_lines.append(f'## Wikipedia: {wiki.get(\"title\", \"Unknown\")}')
            report_lines.append(f'Source: {wiki.get(\"url\", \"\")}')
            report_lines.append('')
            for section in wiki.get('sections', []):
                report_lines.append(f'### {section[\"heading\"]}')
                for p in section['paragraphs'][:3]:
                    report_lines.append(p)
                    report_lines.append('')
            report_lines.append('---')
            report_lines.append('')
    except Exception as e:
        report_lines.append(f'Wikipedia: Error parsing - {e}')
        report_lines.append('')

# Search results overview
search_path = os.path.join(output_dir, '00_search_results.json')
if os.path.exists(search_path):
    try:
        with open(search_path) as f:
            results = json.load(f)
        report_lines.append('## Search Results Overview')
        report_lines.append('')
        for i, r in enumerate(results, 1):
            report_lines.append(f'{i}. [{r[\"title\"]}]({r[\"url\"]})')
        report_lines.append('')
        report_lines.append('---')
        report_lines.append('')
    except:
        pass

# Individual sources
source_files = sorted(glob.glob(os.path.join(output_dir, '*_source.json')))
for sf in source_files:
    try:
        with open(sf) as f:
            src = json.load(f)
        if src.get('error'):
            continue
        report_lines.append(f'## {src.get(\"title\", \"Unknown Source\")}')
        report_lines.append(f'Source: {src.get(\"url\", \"\")}')
        report_lines.append('')
        for item in src.get('content', [])[:20]:
            tag = item.get('tag', 'p')
            text = item.get('text', '')
            if tag in ('h1','h2','h3'):
                report_lines.append(f'### {text}')
            elif tag == 'li':
                report_lines.append(f'- {text}')
            else:
                report_lines.append(text)
            report_lines.append('')
        report_lines.append('---')
        report_lines.append('')
    except:
        continue

# Write final report
report_path = os.path.join(output_dir, 'REPORT.md')
with open(report_path, 'w') as f:
    f.write('\n'.join(report_lines))

print(f'Report written to: {report_path}')
print(f'Total sources: {len(source_files)}')
print(f'Report length: {len(report_lines)} lines')
"

# Take a final screenshot
$CLI tab switch 0 --port $PORT 2>/dev/null || true
$CLI screenshot "$OUTPUT_DIR/final_screenshot.png" --port $PORT 2>/dev/null || true

echo ""
echo "============================================================"
echo " Research Complete!"
echo " Results: $OUTPUT_DIR/"
echo "   - REPORT.md          (aggregated markdown report)"
echo "   - 00_search_results.json  (raw Google result links)"
echo "   - 01_wikipedia.json       (structured Wikipedia data)"
echo "   - XX_source.json          (individual source extracts)"
echo "   - final_screenshot.png    (browser state snapshot)"
echo "============================================================"
