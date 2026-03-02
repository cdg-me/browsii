#!/bin/bash
set -e

echo "================================================="
echo " Browser CLI: Hacker News JS Evaluation Demo"
echo "================================================="

echo "Building CLI..."
go build -o browsii cmd/browsii/*.go

# Start the daemon
echo "Starting daemon natively (headful)..."
./browsii start --mode headful --port 8000
sleep 2

# Navigate to Hacker News
echo "Navigating to: https://news.ycombinator.com"
./browsii navigate "https://news.ycombinator.com" --port 8000
sleep 2

# Get context on the top story
echo "Extracting context for the top story..."

STORY_CONTEXT_JS=$(cat << 'EOF'
() => {
    const topStory = document.querySelector('.titleline a');
    const commentsLink = document.querySelector("a[href^='item?id=']");
    return {
        title: topStory ? topStory.innerText : 'Unknown Title',
        url: commentsLink ? commentsLink.href : 'Unknown URL'
    };
}
EOF
)

echo ""
echo "--- STORY CONTEXT ---"
./browsii js "$STORY_CONTEXT_JS" --port 8000
echo "---------------------"
echo ""

# Click the first comments link
echo "Clicking the top 'comments' link..."
# Selects the first anchor tag ending with 'item?id=' 
./browsii click "a[href^='item?id=']" --port 8000
sleep 2

# Run JS to scrape comments grouped by user
echo "Executing client-side JavaScript..."

JS_PAYLOAD=$(cat << 'EOF'
() => {
    const comments = document.querySelectorAll('.comtr');
    const userGroups = {};
    
    comments.forEach(c => {
        const userNode = c.querySelector('.hnuser');
        if (!userNode) return;
        
        const username = userNode.innerText;
        if (!userGroups[username]) {
            userGroups[username] = 0;
        }
        userGroups[username]++;
    });
    
    // Sort and return the top 5 most active commenters in this thread
    return Object.entries(userGroups)
        .sort((a, b) => b[1] - a[1])
        .slice(0, 5)
        .map(([user, count]) => ({ user, count }));
}
EOF
)

# Call the JS CLI endpoint
echo ""
echo "--- JS RESULT (Top Commenters) ---"
./browsii js "$JS_PAYLOAD" --port 8000
echo "----------------------------------"

echo ""
echo "Cleaning up..."
./browsii stop --port 8000
echo "Done."
