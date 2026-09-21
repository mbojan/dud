#!/bin/bash
set -euo pipefail

# Move the registry's default branch; the import that tracks it (dir.yaml)
# follows, the one pinned to v1 (data.txt.yaml) stays put.

export GIT_AUTHOR_NAME=dud GIT_AUTHOR_EMAIL=dud@example.com
export GIT_COMMITTER_NAME=dud GIT_COMMITTER_EMAIL=dud@example.com

(
    cd ../registry
    rm dir/a.txt
    echo 'a2' > dir/a.txt
    rm data.txt
    echo 'registry data v2' > data.txt
    dud commit
    dud push
    git commit -q -am 'v2'
    git tag v2
)

dud update

cat dir/a.txt data.txt
