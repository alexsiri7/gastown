+++
name = "upstream-sync"
description = "Detect when fork is behind upstream and alert mayor"
version = 1

[gate]
type = "cooldown"
duration = "24h"

[tracking]
labels = ["plugin:upstream-sync", "rig:gastown", "category:maintenance"]
digest = true

[execution]
timeout = "2m"
notify_on_failure = true
severity = "medium"
+++

# Upstream Sync Check

Detects when `alexsiri7/gastown` (origin) has fallen behind
`steveyegge/gastown` (upstream) and mails the mayor to merge.

## Detection

Fetch upstream and count divergence:

```bash
cd /home/asiri/gt/gastown/mayor/rig
git fetch upstream --quiet 2>/dev/null
BEHIND=$(git rev-list origin/main..upstream/main | wc -l)
echo "Commits behind upstream: $BEHIND"
```

If `BEHIND` is 0, gate closes (nothing to do). Skip.

## Action

If behind by any amount:

1. Get a summary of what's new upstream:

```bash
git log --oneline origin/main..upstream/main | head -20
```

2. Mail the mayor with the count and summary:

```
gt mail send mayor/ -s "Upstream sync: gastown is $BEHIND commits behind" -m "
alexsiri7/gastown is $BEHIND commits behind steveyegge/gastown.

Recent upstream commits:
$(git log --oneline origin/main..upstream/main | head -20)

Action needed:
  cd /home/asiri/gt/gastown/mayor/rig
  git fetch upstream
  git merge upstream/main --no-edit
  # Resolve conflicts if any
  git push origin main
  make install   # rebuild gt binary
"
```

3. Close the bead. Do NOT attempt the merge yourself — merges may have
   conflicts that require mayor judgment.

## Notes

- The `rebuild-gt` plugin handles rebuilding after merge — this plugin
  only detects and alerts.
- Upstream remote must be configured: `git remote get-url upstream` should
  return `https://github.com/steveyegge/gastown.git`.
- Gate cooldown of 6h prevents spamming — one alert per 6 hours is enough.
