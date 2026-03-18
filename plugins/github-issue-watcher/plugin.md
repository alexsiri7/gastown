+++
name = "github-issue-watcher"
description = "Poll GitHub issues on opt-in rigs and mail mayor for triage"
version = 1

[gate]
type = "cooldown"
duration = "10m"

[tracking]
labels = ["plugin:github-issue-watcher", "category:issue-monitoring"]
digest = true

[execution]
timeout = "2m"
notify_on_failure = true
severity = "low"
+++

# GitHub Issue Watcher

Polls GitHub for new issues on rigs that have opted in via
`github_issue_watcher` rig config, and mails the mayor for each new issue
so it can be triaged (beads created, polecats slung, etc.).

Requires: `gh` CLI installed and authenticated (`gh auth status`).

## Detection

Verify `gh` is available and authenticated:

```bash
gh auth status 2>/dev/null
if [ $? -ne 0 ]; then
  echo "SKIP: gh CLI not authenticated"
  exit 0
fi
```

## Action

### Step 1: Discover opted-in rigs

List all rigs and check which ones have `github_issue_watcher` enabled:

```bash
RIGS_JSON=$(gt rig list --json 2>/dev/null)
RIG_NAMES=$(echo "$RIGS_JSON" | jq -r '.[].name')

OPTED_IN=()
for RIG in $RIG_NAMES; do
  CONFIG=$(gt rig config show "$RIG" 2>/dev/null)
  if echo "$CONFIG" | grep -q '^github_issue_watcher.*true'; then
    OPTED_IN+=("$RIG")
  fi
done

if [ ${#OPTED_IN[@]} -eq 0 ]; then
  echo "No rigs have github_issue_watcher enabled. Skipping."
  exit 0
fi

echo "Opted-in rigs: ${OPTED_IN[*]}"
```

### Step 2: Poll issues for each opted-in rig

For each opted-in rig, detect the GitHub repo from the rig's git remote
and fetch open issues:

```bash
TOTAL_NEW=0
TOTAL_SEEN=0
GT_ROOT="${GT_ROOT:-$HOME/gt}"

for RIG in "${OPTED_IN[@]}"; do
  # Detect repo from rig config.json git_url
  RIG_ROOT="${GT_ROOT}/${RIG}"
  CONFIG_FILE="${RIG_ROOT}/config.json"
  if [ ! -f "$CONFIG_FILE" ]; then
    echo "WARN: no config.json for rig $RIG, skipping"
    continue
  fi

  GIT_URL=$(jq -r '.git_url // empty' "$CONFIG_FILE")
  REPO=$(echo "$GIT_URL" | sed -E 's|.*github\.com[:/]||; s|\.git$||')

  if [ -z "$REPO" ]; then
    echo "WARN: could not detect GitHub repo for rig $RIG, skipping"
    continue
  fi

  echo "Checking $REPO for rig $RIG..."

  # Fetch open issues (exclude PRs — gh issue list already filters them out)
  ISSUES=$(gh issue list --repo "$REPO" --state open \
    --json number,title,body,author,labels,url \
    --limit 100 2>/dev/null)

  ISSUE_COUNT=$(echo "$ISSUES" | jq 'length')
  if [ "$ISSUE_COUNT" -eq 0 ]; then
    echo "  No open issues for $REPO"
    continue
  fi

  # Load existing gh-issue tracking beads for dedup
  EXISTING=$(bd list --label-pattern "gh-issue:*" --rig "$RIG" --json 2>/dev/null || echo "[]")

  while IFS= read -r ISSUE_JSON; do
    [ -z "$ISSUE_JSON" ] && continue

    NUM=$(echo "$ISSUE_JSON" | jq -r '.number')
    TITLE=$(echo "$ISSUE_JSON" | jq -r '.title')
    AUTHOR=$(echo "$ISSUE_JSON" | jq -r '.author.login')
    URL=$(echo "$ISSUE_JSON" | jq -r '.url')
    BODY=$(echo "$ISSUE_JSON" | jq -r '.body // "(no body)"' | head -50)
    LABELS=$(echo "$ISSUE_JSON" | jq -r '[.labels[].name] | join(", ")')

    # Dedup: check if we already have a bead with label gh-issue:<number>
    DEDUP_LABEL="gh-issue:${NUM}"
    if echo "$EXISTING" | jq -e --arg l "$DEDUP_LABEL" '.[] | .labels // [] | map(select(. == $l)) | length > 0' > /dev/null 2>&1; then
      TOTAL_SEEN=$((TOTAL_SEEN + 1))
      continue
    fi

    # New issue — mail mayor for triage
    MAIL_SUBJECT="GH_ISSUE_OPENED #${NUM}: ${TITLE}"
    MAIL_BODY="New GitHub issue opened on ${REPO}:

Title: ${TITLE}
Author: ${AUTHOR}
URL: ${URL}
Labels: ${LABELS:-none}

---
${BODY}
---

Rig: ${RIG}
Action: Triage — create beads, sling polecats, or respond as needed."

    gt mail send mayor/ -s "$MAIL_SUBJECT" --stdin <<MAILEOF
$MAIL_BODY
MAILEOF

    # Create ephemeral bead for dedup tracking
    bd create "gh-issue #${NUM}: ${TITLE}" -t chore --ephemeral \
      --rig "$RIG" \
      -l "gh-issue:${NUM},plugin:github-issue-watcher" \
      -d "Tracking bead for GitHub issue #${NUM} on ${REPO}. URL: ${URL}" \
      --silent 2>/dev/null || true

    TOTAL_NEW=$((TOTAL_NEW + 1))
    echo "  NEW: #${NUM} ${TITLE} (by ${AUTHOR}) — mailed mayor"
  done < <(echo "$ISSUES" | jq -c '.[]')
done
```

## Record Result

```bash
SUMMARY="github-issue-watcher: ${#OPTED_IN[@]} rig(s) checked, $TOTAL_NEW new issue(s) mailed to mayor, $TOTAL_SEEN already tracked"
echo "$SUMMARY"
```

On success:
```bash
bd create "github-issue-watcher: $SUMMARY" -t chore --ephemeral \
  -l type:plugin-run,plugin:github-issue-watcher,result:success \
  -d "$SUMMARY" --silent 2>/dev/null || true
```

On failure:
```bash
bd create "github-issue-watcher: FAILED" -t chore --ephemeral \
  -l type:plugin-run,plugin:github-issue-watcher,result:failure \
  -d "GitHub issue watcher failed: $ERROR" --silent 2>/dev/null || true

gt escalate "Plugin FAILED: github-issue-watcher" \
  --severity low \
  --reason "$ERROR"
```
