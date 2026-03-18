+++
name = "daily-report"
description = "Generate daily changelog as a GitHub Release with prose summary of activity across active rigs"
version = 1

[gate]
type = "cron"
schedule = "0 9 * * *"

[tracking]
labels = ["plugin:daily-report", "category:reporting"]
digest = true

[execution]
timeout = "5m"
notify_on_failure = true
severity = "low"
+++

# Daily Report

Generates a daily changelog summarizing activity across all active rigs and
publishes it as a GitHub Release. This is a **dog plugin** — you (the Claude
agent) read raw data and write a human-readable narrative summary.

Also supports manual trigger via the deacon.

## Opt-in Check

Only run for rigs that have opted in via rig config:

```bash
echo "=== Daily Report: Checking rig opt-in ==="

TOWN_ROOT="$HOME/gt"
RIGS_JSON_PATH="${TOWN_ROOT}/mayor/rigs.json"

if [ ! -f "$RIGS_JSON_PATH" ]; then
  echo "SKIP: rigs.json not found"
  exit 0
fi

RIGS_FILE=$(cat "$RIGS_JSON_PATH" 2>/dev/null)
RIG_NAMES=$(echo "$RIGS_FILE" | jq -r '.rigs | keys[]' 2>/dev/null)

if [ -z "$RIG_NAMES" ]; then
  echo "SKIP: no rigs found in rigs.json"
  exit 0
fi

TODAY=$(date -u +%Y-%m-%d)
YESTERDAY=$(date -u -d '24 hours ago' +%Y-%m-%d 2>/dev/null || date -u -v-1d +%Y-%m-%d)
ACTIVE_RIGS=()

for RIG in $RIG_NAMES; do
  # Check opt-in: gt rig config show <rig> and look for daily_report = true
  DR_ENABLED=$(gt rig config show "$RIG" 2>/dev/null \
    | grep -E '^daily_report\s' | awk '{print $2}')

  if [ "$DR_ENABLED" != "true" ]; then
    echo "  $RIG: daily_report not enabled, skipping"
    continue
  fi

  ACTIVE_RIGS+=("$RIG")
done

if [ ${#ACTIVE_RIGS[@]} -eq 0 ]; then
  echo "No rigs have daily_report enabled. Nothing to do."
  exit 0
fi

echo "Active rigs for daily report: ${ACTIVE_RIGS[*]}"
```

## Step 1: Gather raw data per rig

For each opted-in rig, collect commits, merged PRs, closed beads, convoy
progress, and open issues.

```bash
ALL_DATA=""

for RIG in "${ACTIVE_RIGS[@]}"; do
  echo ""
  echo "=== Gathering data for rig: $RIG ==="

  RIG_ROOT="$TOWN_ROOT/$RIG/mayor/rig"
  GIT_URL=$(echo "$RIGS_FILE" | jq -r --arg r "$RIG" '.rigs[$r].git_url // empty')
  BEADS_PREFIX=$(echo "$RIGS_FILE" | jq -r --arg r "$RIG" '.rigs[$r].beads.prefix // empty')

  # Derive GitHub repo from git URL
  REPO=$(echo "$GIT_URL" | sed -E 's|.*github\.com[:/]||; s|\.git$||')

  RIG_DATA="### $RIG"
  RIG_DATA="$RIG_DATA\nrepo: $REPO"

  # 1. Recent commits (last 24h)
  COMMITS=""
  if [ -d "$RIG_ROOT/.git" ] || [ -d "$RIG_ROOT" ]; then
    COMMITS=$(git -C "$RIG_ROOT" log --oneline --since='24 hours ago' 2>/dev/null || echo "")
  fi
  COMMIT_COUNT=$(echo "$COMMITS" | grep -c . 2>/dev/null || echo 0)
  RIG_DATA="$RIG_DATA\n\nCommits (last 24h): $COMMIT_COUNT"
  if [ -n "$COMMITS" ]; then
    RIG_DATA="$RIG_DATA\n$COMMITS"
  fi

  # 2. Merged PRs (last 24h)
  MERGED_PRS=""
  if [ -n "$REPO" ]; then
    MERGED_PRS=$(gh pr list --repo "$REPO" --state merged \
      --search "merged:>$YESTERDAY" \
      --json number,title,author \
      --limit 50 2>/dev/null || echo "[]")
  fi
  MERGED_COUNT=$(echo "$MERGED_PRS" | jq 'length' 2>/dev/null || echo 0)
  RIG_DATA="$RIG_DATA\n\nMerged PRs: $MERGED_COUNT"
  if [ "$MERGED_COUNT" -gt 0 ]; then
    RIG_DATA="$RIG_DATA\n$(echo "$MERGED_PRS" | jq -r '.[] | "  PR #\(.number): \(.title) (by \(.author.login))"' 2>/dev/null)"
  fi

  # 3. Closed beads (last 24h)
  CLOSED_BEADS=""
  if [ -n "$BEADS_PREFIX" ]; then
    CLOSED_BEADS=$(bd list --status=closed --json 2>/dev/null \
      | jq --arg y "$YESTERDAY" '[.[] | select(.updated_at >= $y)]' 2>/dev/null || echo "[]")
  fi
  CLOSED_COUNT=$(echo "$CLOSED_BEADS" | jq 'length' 2>/dev/null || echo 0)
  RIG_DATA="$RIG_DATA\n\nClosed beads: $CLOSED_COUNT"
  if [ "$CLOSED_COUNT" -gt 0 ]; then
    RIG_DATA="$RIG_DATA\n$(echo "$CLOSED_BEADS" | jq -r '.[] | "  \(.id): \(.title)"' 2>/dev/null)"
  fi

  # 4. Convoy progress
  CONVOYS=$(gt convoy list 2>/dev/null || echo "")
  if [ -n "$CONVOYS" ]; then
    RIG_DATA="$RIG_DATA\n\nConvoys:\n$CONVOYS"
  fi

  # 5. Open GitHub issues count
  if [ -n "$REPO" ]; then
    OPEN_ISSUES=$(gh issue list --repo "$REPO" --state open \
      --json number --limit 200 2>/dev/null \
      | jq 'length' 2>/dev/null || echo "?")
    RIG_DATA="$RIG_DATA\n\nOpen GitHub issues: $OPEN_ISSUES"
  fi

  ALL_DATA="$ALL_DATA\n\n$RIG_DATA"
done

echo ""
echo "=== Raw data collection complete ==="
echo -e "$ALL_DATA"
```

## Step 2: Write prose summary

**You (the dog agent) read all the raw data above and write a concise, readable
report.** This is NOT a mechanical dump of PR titles — it's a narrative.

Cover these sections:

1. **What shipped** — Features, fixes, and improvements in plain English. Group
   related changes. Don't just list PR titles; describe what they accomplish.
2. **What's in progress** — Open convoys, active polecats, work that's underway.
3. **What needs attention** — Blockers, failing CI, stale PRs, anything concerning.
4. **Key metrics** — PRs merged, beads closed, commits, open issues remaining.

**Tone:** Professional but concise. Like a good daily standup email. No filler,
no celebration for routine work. Highlight what matters.

**Format the output as markdown** suitable for a GitHub Release body.

## Step 3: Publish as GitHub Release

For each rig that had activity, create a GitHub Release with the prose summary.

```bash
for RIG in "${ACTIVE_RIGS[@]}"; do
  GIT_URL=$(echo "$RIGS_FILE" | jq -r --arg r "$RIG" '.rigs[$r].git_url // empty')
  REPO=$(echo "$GIT_URL" | sed -E 's|.*github\.com[:/]||; s|\.git$||')

  if [ -z "$REPO" ]; then
    echo "  $RIG: no repo URL, skipping release"
    continue
  fi

  # Check if a release for today already exists
  EXISTING=$(gh release view "daily/$TODAY" --repo "$REPO" 2>/dev/null && echo "exists" || echo "")
  if [ -n "$EXISTING" ]; then
    echo "  $RIG: release daily/$TODAY already exists, skipping"
    continue
  fi

  # The REPORT variable should be set by the dog agent in Step 2
  # with the prose summary for this rig
  gh release create "daily/$TODAY" --repo "$REPO" \
    --title "Daily Report: $TODAY" \
    --notes "$REPORT" \
    --latest=false

  if [ $? -eq 0 ]; then
    echo "  $RIG: published daily/$TODAY release"
  else
    echo "  $RIG: FAILED to publish release"
  fi
done
```

The `--latest=false` flag prevents daily reports from appearing as the "latest
release" — that designation is reserved for actual version releases.

## Record Result

```bash
SUMMARY="Daily report: ${#ACTIVE_RIGS[@]} rig(s) processed for $TODAY"
echo "=== $SUMMARY ==="
```

On success:
```bash
bd create "daily-report: $SUMMARY" -t chore --ephemeral \
  -l type:plugin-run,plugin:daily-report,result:success \
  -d "$SUMMARY" --silent 2>/dev/null || true
```

On failure:
```bash
bd create "daily-report: FAILED" -t chore --ephemeral \
  -l type:plugin-run,plugin:daily-report,result:failure \
  -d "Daily report failed: $ERROR" --silent 2>/dev/null || true

gt escalate "Plugin FAILED: daily-report" \
  --severity low \
  --reason "$ERROR"
```
