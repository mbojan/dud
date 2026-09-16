#!/bin/bash
set -euo pipefail

# A leading argument naming a configured remote selects that remote.

dud push bar

# An unknown name given via the flag must fail rather than be passed to
# rclone as a path.
! dud push --remote nope
