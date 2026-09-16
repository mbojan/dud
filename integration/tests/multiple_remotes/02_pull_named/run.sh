#!/bin/bash
set -euo pipefail

# Wipe the default remote so the pull can only succeed via 'bar'. Pull is
# given both a remote name and a stage path to make sure the remote name is
# not forwarded to checkout as a stage.

rm foo.txt
rm -rf .dud/cache/* remote_foo

dud pull bar foo.yaml

