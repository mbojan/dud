#!/bin/bash
set -euo pipefail

# Declare two named remotes and push to the default one only.

dud init

echo 'bar' > foo.txt

dud stage gen -o foo.txt > foo.yaml

dud stage add foo.yaml

dud commit

mkdir remote_foo remote_bar

dud config set remotes.foo remote_foo
dud config set remotes.bar remote_bar
dud config set remote foo

dud push
