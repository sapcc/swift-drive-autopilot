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
YAML and the following options are supported:

```yaml
drives:
  - /dev/sd[c-z]
```

The only required field, `drives`, contains the paths of the Swift storage
drives, as a list of shell globs.

```yaml
chroot: /coreos
```

If `chroot` is set, commands like cryptsetup/mkfs/mount will be executed inside
the chroot. This allows to use the host OS's utilities instead of those from
the container.

```yaml
chown:
  user: "1000"
  group: "swift"
```

If `chown` is set, mountpoints below `/srv/node` will be chown'ed to this user
and/or group after mounting. Give the UID/GID or names of the Swift user and
group here.

```yaml
keys:
  - secret: "bzQoG5HN4onnEis5bhDmnYqqacoLNCSmDbFEAb3VDztmBtGobH"
  - secret: "Nr8LHATRJF4kPI51KY6pgsUCbAXwHN9LPNjaMknTWK4u44EAme"
```

If `keys` is set, automatic disk encryption handling is activated. LUKS
containers on the drives will be decrypted automatically, and empty drives will
be encrypted with LUKS before a filesystem is created.

When decrypting, each of the keys is tried until one works, but only the first
one is used when creating new LUKS containers.

Currently, the `secret` will be used as encryption key directly. Other key
derivation schemes may be supported in the future.

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
