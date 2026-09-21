#!/bin/bash
set -euo pipefail

# Build a data registry next to the project: a Dud project under git whose
# artifacts are pushed to a directory used as an rclone remote. Then import
# two of its artifacts into the project, one pinned to a tag and one tracking
# the default branch.

export GIT_AUTHOR_NAME=dud GIT_AUTHOR_EMAIL=dud@example.com
export GIT_COMMITTER_NAME=dud GIT_COMMITTER_EMAIL=dud@example.com

registry="$(dirname "$PWD")/registry"
remote="$(dirname "$PWD")/fake_remote"
mkdir "$registry" "$remote"

(
    cd "$registry"
    git init -q
    # Let a clone push back into this checkout (see 06_contribute).
    git config receive.denyCurrentBranch updateInstead
    dud init
    echo 'registry data' > data.txt
    mkdir dir
    echo 'a' > dir/a.txt
    echo 'b' > dir/b.txt
    dud stage gen -o data.txt > data.txt.yaml
    dud stage gen -o dir > dir.yaml
    dud stage add data.txt.yaml dir.yaml
    # Only stage files and .dud/{index,config.yaml} belong in git; the data
    # lives on the remote.
    printf 'data.txt\ndir\n' > .gitignore
    dud config set remotes.reg "$remote"
    dud config set remote reg
    dud commit
    dud push
    git add -A
    git commit -q -m 'v1'
    git tag v1
)

dud init

dud import "$registry" data.txt --rev v1

dud import "$registry" dir
