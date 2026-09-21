#!/bin/bash
set -euo pipefail

# A file inside a directory artifact can't be imported on its own.
! dud import ../registry dir/b.txt

# An import can't take over a path owned by another stage.
! dud import ../registry data.txt -o copy.txt

# Only import stages can be updated.
! dud update copy.yaml

# A modified artifact is never removed by update.
rm data.txt
echo 'local edits' > data.txt
sed -i 's/^  rev: v2$/  rev: v1/' data.txt.yaml
! dud update data.txt.yaml

# Restore the stage so later steps see a consistent project.
rm data.txt
sed -i 's/^  rev: v1$/  rev: v2/' data.txt.yaml
dud checkout data.txt.yaml
