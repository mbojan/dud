## Concepts

### Artifact

An Artifact is a file or directory that is tracked by Dud. Artifacts are usually
stored in the Cache, but it isn't strictly necessary.

### Stage

A Stage is a group of Artifacts, or an operation that consumes and/or produces
a group of Artifacts. Stages are defined by the user in YAML files and should
be tracked with source control. The Stage YAML file format is described in
[`dud stage --help`]({{<ref "cli/dud_stage">}}).

### Import Stage

An import Stage is a Stage that pins a single Artifact from a data registry
(see below) instead of producing it. It has no command and no inputs; its
`import` block records the registry, the git revision, and the rclone remote
the Artifact is fetched from, and its one output carries the checksum
recorded in the registry. Import Stages are read-only from the project's point
of view: `dud commit` and `dud push` skip them, `dud fetch` downloads them from
the registry's remote, and `dud update` moves them to a newer revision. See
[Data Registries]({{< ref "data_registries.md" >}}).

### Index

The Index is the comprehensive group of Stages in a project. It is stored in
a plain text file at `.dud/index`. The Index forms a dependency graph of Stages,
enabling the user to define data pipelines.

### Cache

The Cache is a local directory where Dud stores and versions the contents of
Artifacts. The Cache is content-addressed, which (among other things)
facilitates storing all versions of all Artifacts without conflicts or
duplication.

### Data Registry

A data registry is an ordinary Dud project tracked with git whose committed
Artifacts have been pushed to a remote. Any project can import Artifacts from
it with `dud import`, pinning them to a git tag, branch or commit. The registry
itself needs nothing special: its Stage files, `.dud/index` and
`.dud/config.yaml` in git, and its data on the remote named there.
