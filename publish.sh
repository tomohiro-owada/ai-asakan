#!/usr/bin/env bash
# Publish one AI朝刊 issue: put the (commented) digest in issues/, validate, commit, push.
#
#   ./publish.sh [DATE] [--from FILE] [--allow-uncommented] [--no-push]
#
# DATE defaults to today in Asia/Manila (YYYY-MM-DD).
# Source of the digest JSON, first match wins:
#   1. --from FILE                                   (any path; already-commented JSON)
#   2. issues/digest-DATE.json                       (already in the repo dir, e.g. written by the routine)
#   3. /workspace/asakan-comments/digest-DATE.json   (commented digest)
#   4. /workspace/newspaper/digest-DATE.json         (Toto's raw digest, only with --allow-uncommented)
# An existing issues/digest-DATE.json is NEVER overwritten (--from/copy is refused if it differs).
# Exit 0 with a message if there is nothing new to publish; non-zero on validation/build/push errors.
set -euo pipefail
cd "$(dirname "$0")"

DATE=""; FROM=""; ALLOW=0; PUSH=1
while [ $# -gt 0 ]; do
  case "$1" in
    --from) FROM="${2:?--from needs a file}"; shift 2 ;;
    --allow-uncommented) ALLOW=1; shift ;;
    --no-push) PUSH=0; shift ;;
    -h|--help) sed -n '2,15p' "$0"; exit 0 ;;
    [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]) DATE="$1"; shift ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
DATE="${DATE:-$(TZ=Asia/Manila date +%F)}"
DEST="issues/digest-$DATE.json"

if [ -n "$FROM" ]; then SRC="$FROM"
elif [ -f "$DEST" ]; then SRC="$DEST"
elif [ -f "/workspace/asakan-comments/digest-$DATE.json" ]; then SRC="/workspace/asakan-comments/digest-$DATE.json"
elif [ -f "/workspace/newspaper/digest-$DATE.json" ]; then
  if [ "$ALLOW" = 1 ]; then SRC="/workspace/newspaper/digest-$DATE.json"
  else echo "Only the uncommented digest exists for $DATE (/workspace/newspaper). Add the commented JSON or pass --allow-uncommented. Nothing published."; exit 0; fi
else echo "No digest for $DATE found. Nothing to publish."; exit 0; fi
[ -f "$SRC" ] || { echo "error: $SRC not found" >&2; exit 1; }

# validate JSON + that its date matches
node -e 'const d=JSON.parse(require("fs").readFileSync(process.argv[1],"utf8"));if(d.date!==process.argv[2]){console.error("date field "+d.date+" != "+process.argv[2]);process.exit(1)}' "$SRC" "$DATE" \
  || { echo "error: $SRC is not valid JSON for $DATE" >&2; exit 1; }

if [ "$SRC" != "$DEST" ]; then
  if [ -e "$DEST" ]; then
    if cmp -s "$SRC" "$DEST"; then :; else echo "error: $DEST already exists and differs; refusing to overwrite (edit it directly if intended)." >&2; exit 1; fi
  else cp "$SRC" "$DEST"; fi
fi

# the generator must build the whole site from issues/
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
go run . -site issues -out "$TMP/site" >/dev/null || { echo "error: generator failed; not committing" >&2; [ "$SRC" != "$DEST" ] && git status --porcelain "$DEST" | grep -q '^??' && rm -f "$DEST"; exit 1; }

# optional OGP image (needs node + playwright; skipped if unavailable)
if [ ! -f "og/$DATE.jpg" ]; then
  go run . -in "$DEST" -out "$TMP" -og "og/$DATE.jpg" >/dev/null 2>&1 || echo "note: OGP image not rendered (falls back to og/default.jpg)"
fi

git add "$DEST"; [ -f "og/$DATE.jpg" ] && git add "og/$DATE.jpg"
if git diff --cached --quiet; then echo "Issue $DATE is already published. Nothing to do."; exit 0; fi

NOW="$(TZ=Asia/Manila date +'%Y-%m-%dT%H:%M:%S+08:00')"
export GIT_AUTHOR_NAME="Tomohiro Owada" GIT_AUTHOR_EMAIL="info@tomoporo.net" GIT_COMMITTER_NAME="Tomohiro Owada" GIT_COMMITTER_EMAIL="info@tomoporo.net"
export GIT_AUTHOR_DATE="$NOW" GIT_COMMITTER_DATE="$NOW"
HL="$(node -e 'console.log(JSON.parse(require("fs").readFileSync(process.argv[1],"utf8")).lead.headline.replace(/\n/g,""))' "$DEST")"
git commit -q -m "Publish $DATE issue: $HL"
if [ "$PUSH" = 1 ]; then git push -q origin main; echo "Published $DATE and pushed. Site: https://news.abalol.xyz/$(echo "$DATE" | tr - /)/";
else echo "Committed $DATE (not pushed)."; fi
