# backlogger

A local-first, directory-scoped task tracker.  
One index file (`backlog.md`) is derived from one detail file per ticket under `.tickets/`.

- Correct under concurrency: every mutation runs inside one file lock with an
  atomic read/verify/write cycle.
- Git-friendly: plain Markdown, byte-stable derived index, no server or daemon.
- Single static Go binary.

## Table of contents

- [Install](#install)
- [Usage](#usage)
- [License](#license)

## Install

From a release (downloads the latest tagged archive, needs `curl` + `jq`):

```sh
bin/download.sh     # installs bl into ~/.local/bin
```

From source (builds, then installs):

```sh
bin/install.sh      # build + copy to ~/.local/bin
make install        # same result
```

## Usage

```sh
bl add "Add login flow" -p 1
bl list
bl available
bl start <id>
bl done <id>
bl doctor
```

Run `bl help` for the full command surface. Distribution is via GitHub
Releases.

## License

Apache License 2.0. Copyright 2026 Yury Antonov. See [LICENSE](LICENSE).
