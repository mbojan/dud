#!/bin/bash
set -euo pipefail

# Imported artifacts are ordinary inputs for the project's own stages. Commit
# and status leave the import stages alone. The stage is named explicitly so
# the output order (upstream first) is deterministic.

dud stage gen -i data.txt -o copy.txt -- cp data.txt copy.txt > copy.yaml

dud stage add copy.yaml

dud run copy.yaml

dud commit copy.yaml

dud status copy.yaml
