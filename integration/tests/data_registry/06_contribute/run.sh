#!/bin/bash
set -euo pipefail

# Contribute an artifact produced by another Dud project to the registry with
# nothing but the usual commands: copy it into a clone of the registry (the
# workspace copy is a link into the producer's cache, hence -L), commit and
# push it there, and version the registry in git.

export GIT_AUTHOR_NAME=dud GIT_AUTHOR_EMAIL=dud@example.com
export GIT_COMMITTER_NAME=dud GIT_COMMITTER_EMAIL=dud@example.com

producer="$(dirname "$PWD")/producer"
mkdir "$producer"
(
    cd "$producer"
    dud init
    dud stage gen -o model.bin -- 'echo model > model.bin' > model.yaml
    dud stage add model.yaml
    dud run
    dud commit
)

git clone -q ../registry ../registry_clone
(
    cd ../registry_clone
    cp -L "$producer/model.bin" model.bin
    dud stage gen -o model.bin > model.yaml
    dud stage add model.yaml
    echo model.bin >> .gitignore
    # The clone has no workspace copies of the other artifacts, so commit
    # only the new stage.
    dud commit model.yaml
    dud push model.yaml
    git add -A
    git commit -q -m 'v3: add model.bin'
    git tag v3
    git push -q origin HEAD v3
)

dud import ../registry model.bin --rev v3

cat model.bin
