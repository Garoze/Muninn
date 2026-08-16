#!/usr/bin/env bash
# Merges main into develop, opening and merging its own PR.
#
# A develop -> main PR only advances main's ref; develop's never moves. Any
# commit made on main - a release cut's tag and changelog, or a fix applied
# there directly - stays outside develop's ancestry until something merges in
# this direction too.
#
# Usage: back-merge.sh <branch-suffix>
# Requires: GH_TOKEN, a full-history checkout, and git user configured.
set -euo pipefail

suffix="${1:?usage: back-merge.sh <branch-suffix>}"

git fetch origin main develop

# Both callers can fire for the same commit - a release cut back-merges at its
# end, and the push that cut made triggers the standalone workflow. Whichever
# runs first does the work; this makes the other a no-op instead of a failure
# on an empty commit.
if git merge-base --is-ancestor origin/main origin/develop; then
    echo "develop already contains main - nothing to back-merge"
    exit 0
fi

branch="back-merge/${suffix}"
git checkout -b "$branch" origin/develop

# CHANGELOG.md always conflicts here by design: develop keeps every alpha
# heading, main consolidates them, so the same content is grouped differently
# on each side. The resolver compares develop's current file against develop's
# own file at the merge-base - anything not present there yet is genuinely new
# since the last sync and is kept; everything else comes from main, which is
# authoritative for already-released history.
merge_base="$(git merge-base origin/develop origin/main)"
git show "$merge_base:CHANGELOG.md" > /tmp/changelog-base.md 2>/dev/null || : > /tmp/changelog-base.md
git show origin/develop:CHANGELOG.md > /tmp/changelog-develop.md
git show origin/main:CHANGELOG.md > /tmp/changelog-main.md

set +e
git merge origin/main --no-commit -m "Merge main into develop"
merge_status=$?
set -e

if [ $merge_status -ne 0 ]; then
    conflicted="$(git diff --name-only --diff-filter=U)"
    for f in $conflicted; do
        case "$f" in
            CHANGELOG.md)
                python3 hack/resolve-changelog-backmerge.py \
                    /tmp/changelog-develop.md /tmp/changelog-base.md /tmp/changelog-main.md CHANGELOG.md
                git add CHANGELOG.md
                ;;
            # develop's copy is authoritative: only develop's release-please
            # runs read it. main's is an inherited byproduct of the last
            # forward merge.
            .release-please-manifest.json)
                git checkout --ours .release-please-manifest.json
                git add .release-please-manifest.json
                ;;
            *)
                echo "unexpected conflict in $f during the back-merge - needs manual resolution" >&2
                exit 1
                ;;
        esac
    done
fi

git commit \
    -m "Merge main into develop" \
    -m "- Brings main's commits into develop's own ancestry. A develop -> main PR only advances main's ref, so anything committed on main is invisible to develop without a merge in this direction."
git push origin "$branch"
gh pr create --base develop --head "$branch" \
    --title "Merge main into develop" \
    --body "Brings main's commits into develop's own ancestry, so nothing released or fixed on main is unreachable from the branch every change enters through. CHANGELOG.md and .release-please-manifest.json conflicts, if any, were resolved automatically - see the workflow run for details."
gh pr merge "$branch" --merge --delete-branch
