# swift-drive-autopilot

This service finds, formats and mounts Swift storage drives, usually from
within a container on a Kubernetes host.

## How it works

Swift expects its drives to be mounted at `/srv/node/$id`, where the `$id`
identifier is referenced in the cluster's **ring files**. The usual method is
to set `$id` equal to the device's name in `/dev`, e.g. `/dev/sdc` becomes
`/srv/node/sdc`, but that mapping is too rigid for some situations.

`swift-drive-autopilot` establishes disk identity by examining a special file
called `swift-id` in the root directory of the disk. In detail, it performs the
following steps:

1. enumerate all storage drives (using a configurable list of globs)

2. create an XFS filesystem on devices that do not have a filesystem yet

2. mount each device below `/run/swift-storage` with a temporary name

3. examine each device's `swift-id` file, and if it is present and unique,
   bind-mount it to `/srv/node/$id`

Support for data-at-rest encryption using dm-crypt/LUKS is coming soon.

## Installation

To build the binary:

```bash
make
```

The binary can also be installed with `go get`:
```bash
go get github.com/sapcc/swift-drive-autopilot
```

To build the Docker container:

```bash
make && docker build .
```

## Usage

Call with a configuration file as single argument. The configuration file is a
YAML like this:

```yaml
drives:             # (required) paths of Swift storage drives, as list of
  - /dev/sd[c-z]    # shell globs

chroot: /coreos     # (optional) if set, then execute cryptsetup/mkfs/mount
                    # inside this chroot; this allows to use the host OS's
                    # utilities instead of those from the container

chown:              # (optional) if set, then mountpoints below /srv/node will
  user: "1000"      # be chown'ed to this user and/or group after mounting (give
  group: "swift"    # the UID/GID or names of the Swift user and group)
```

### In Docker

When used as a container, supply the host's root filesystem as a bind-mount and
set the `chroot` option to its mount point inside the container. Also, the
container has to run in privileged mode to access the host's block devices and
perform mounts in the root mount namespace:

```bash
$ cat > config.yml
drives:
  - /dev/sd[c-z]
chroot: /host
$ docker run --privileged --rm -v $PWD/config.yml:/config.yml -v /:/host sapcc/swift-drive-autopilot:latest /config.yml
```

### In Kubernetes

You will probably want to run this as a daemonset with the `nodeSelector`
matching your Swift storage nodes. Like described for Docker above, make sure
to mount the host's root filesystem into the container (with a `hostPath`
volume) and run the container in privileged mode (by setting
`securityContext.privileged` to `true` in the container spec).
