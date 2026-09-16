#!/bin/bash
set -euo pipefail

# Exercise the rclone_config key: put the rclone config outside the project
# entirely and delete the default .dud/rclone.conf, so Dud has to pick up the
# external file via the new setting.

external_conf="$PWD/../external_rclone.conf"
remote_dir="$PWD/../fake_remote"

dud init
rm .dud/rclone.conf

mkdir "$remote_dir"

cat > "$external_conf" <<EOF
[fake_remote]
type = local
EOF

dud config set rclone_config "$external_conf"
dud config set remote "fake_remote:$remote_dir"

echo 'bar' > foo.txt

dud stage gen -o foo.txt > foo.yaml

dud stage add foo.yaml

dud commit

dud push

rm foo.txt
rm -rf .dud/cache/*

dud pull
