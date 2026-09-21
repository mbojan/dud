#!/bin/bash
set -euo pipefail

# Switching an import to another tag is done by editing 'rev' in the stage
# file and running update on it.

sed -i 's/^  rev: v1$/  rev: v2/' data.txt.yaml

dud update data.txt.yaml

cat data.txt
