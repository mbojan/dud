---
weight: 3
title: Data Registries
---
# Data Registries

A data registry is a Dud project that exists to publish datasets, models, or
any other Artifacts for other projects to use. It is nothing more than an
ordinary Dud project tracked with git: the Stage files, `.dud/index` and
`.dud/config.yaml` live in git, and the data lives on the rclone remote named
in the config. Other projects then *import* Artifacts from the registry,
pinned to a git revision, with `dud import`, and move the pin with
`dud update`.

This page walks through setting up a registry, consuming it, and contributing
to it. It assumes you have read [Getting Started]({{< ref "getting_started.md" >}})
and have git and [rclone](https://rclone.org) installed.

## Setting up a registry

Create a Dud project, add and commit some data, and push it to a remote. For
the sake of the demo the "remote" is just another directory, as in Getting
Started; any rclone remote works the same way.

```
$ mkdir registry && cd registry
$ git init
$ dud init
$ cp ~/somewhere/raw.csv data/raw.csv
$ dud stage gen -o data/raw.csv > data/raw.csv.yaml
$ dud stage add data/raw.csv.yaml
$ dud config set remotes.storage /srv/dud-registry-remote
$ dud config set remote storage
$ dud commit
$ dud push
```

Only the Stage files and `.dud/` metadata belong in git. Ignore the Artifacts
themselves (they are links into the local cache after `dud commit`), then
commit and tag:

```
$ echo 'data/raw.csv' >> .gitignore
$ git add -A
$ git commit -m 'Add raw.csv'
$ git tag v1
$ git push origin HEAD v1
```

The `dud init` template already ignores `.dud/cache` and `.dud/lock`. The
project's rclone config (`.dud/rclone.conf`, if you use one) is what maps the
remote's name to real credentials; whether to commit it is up to you and the
kind of remote.

## Importing from a registry

In another Dud project, import the Artifact by naming the registry (a git URL
or a local path) and the Artifact's path *inside the registry*:

```
$ cd ../analysis
$ dud init
$ dud import git@github.com:org/registry.git data/raw.csv --rev v1
added import stage raw.csv.yaml
fetching stage raw.csv.yaml
checking out stage raw.csv.yaml
```

`dud import` writes an import Stage next to the Artifact (by default the
Artifact's base name in the current directory; use `-o` to pick another path),
fetches the Artifact from the registry's remote into this project's cache, and
checks it out. The Stage file records everything needed to reproduce that:

```yaml
checksum: 7f1e…
import:
  repo: git@github.com:org/registry.git
  rev: v1
  rev-lock: 6c73875a5f5b522f90b5afa9ab12585f64327ca7
  path: data/raw.csv
  remote: /srv/dud-registry-remote
outputs:
  raw.csv:
    checksum: 8e4c7c1b…
```

`rev` is what you asked for (a tag, branch or commit; leave it out to track
the registry's default branch), `rev-lock` is the commit it resolved to, and
`remote` is the rclone remote path the registry's config named at that
commit. Because fetching goes through *your* rclone configuration, the remote
name used by the registry must be defined in it as well (a plain directory
path, as here, needs nothing).

From here on the imported Artifact is like any other. It can be an input to
your own Stages:

```
$ dud stage gen -i raw.csv -o clean.csv -- python clean.py > clean.yaml
$ dud stage add clean.yaml
$ dud run
$ dud commit
committing stage clean.yaml
skipping import stage raw.csv.yaml (imports are read-only)
```

`dud commit` and `dud push` leave import Stages alone: the registry is the
source of truth for their contents. `dud fetch` and `dud pull` know to
download them from the registry's remote, so collaborators cloning your
project get the data with the usual `dud pull`.

A hand-written import Stage with just `repo`, `path` and optionally `rev` is
valid too; `dud update` fills in the rest.

## Updating imports

`dud update` re-resolves import Stages against their registries. With no
arguments it updates every import Stage in the index:

```
$ dud update
stage raw.csv.yaml is up to date
```

An import that tracks a branch (no `rev`) follows new commits on it:

```
$ dud update
updated stage raw.csv.yaml
fetching stage raw.csv.yaml
checking out stage raw.csv.yaml
```

To move a pinned import to another tag, edit `rev` in the Stage file and run
`dud update` on it:

```
$ sed -i 's/rev: v1/rev: v2/' raw.csv.yaml
$ dud update raw.csv.yaml
```

Before checking out the new version, `dud update` removes the old one from the
workspace, but only if it is unmodified. If you have changed the Artifact
locally, update refuses and asks you to move it aside first; Dud never
discards your changes implicitly. Use `--no-fetch` to update the Stage files
only.

## Contributing to a registry

Because the registry is a plain Dud project, contributing an Artifact that
another Dud project produced takes no special commands. Say the `analysis`
project's pipeline produced `model.bin` and committed it:

```
$ cd ../analysis
$ dud run
$ dud commit
```

Clone the registry, copy the Artifact in, and commit and push it there.
`cp -L` matters: after `dud commit`, the workspace copy is a link into the
producer's cache, and the registry should get a real file.

```
$ git clone git@github.com:org/registry.git ../registry-clone
$ cd ../registry-clone
$ cp -L ../analysis/model.bin models/model.bin
$ dud stage gen -o models/model.bin > models/model.bin.yaml
$ dud stage add models/model.bin.yaml
$ echo 'models/model.bin' >> .gitignore
$ dud commit models/model.bin.yaml
$ dud push models/model.bin.yaml
```

Name the Stage in `dud commit` and `dud push`: the clone has no workspace
copies of the registry's other Artifacts, so an unqualified `dud commit` would
complain about them. The Artifact is re-hashed here, but since the cache is
content-addressed the checksum is the same one the producer computed.

Finally, version the change in git:

```
$ git add -A
$ git commit -m 'Add model.bin'
$ git tag v2
$ git push origin HEAD v2
```

Consumers pick it up with `dud import … models/model.bin --rev v2`, or, for
imports tracking the default branch, with `dud update`. To publish a new
version of an existing Artifact, overwrite the file in the clone and commit,
push and tag as above; the Stage already exists.

Registries can also generate their own data: give a registry Stage a command
(a download script, say) and contributing becomes editing that command and
running `dud run`, `dud commit` and `dud push` in the registry. Both kinds of
registry import identically.

A project should not import an Artifact that it also contributes to the
registry: the import Stage would own the same path as the producing Stage,
which `dud stage add` and `dud import` reject.
