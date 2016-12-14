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

2. (optional) create a LUKS encryption container on fresh devices, or unlock an
   existing one

3. create an XFS filesystem on devices that do not have a filesystem yet

4. mount each device below `/run/swift-storage` with a temporary name

5. examine each device's `swift-id` file, and if it is present and unique,
   bind-mount it to `/srv/node/$id`

The autopilot then continues to run and will react to various types of events:

1. A new device file appears. It will be decrypted and mounted (and formatted
   if necessary).

2. A device file disappears. Any active mounts or mappings will be cleaned up.
   (This is especially helpful with hot-swappable hard drives.)

3. The kernel log contains a line like `error on /dev/sda`. The offending
   device will be marked as unhealthy and unmounted from `/srv/node`. The
   other mappings and mounts are left intact for the administrator to inspect.

   This means that you do not need `swift-drive-audit` if you're using the
   autopilot.

4. Mounts of managed devices disappear unexpectedly. The offending device will
   be marked as unhealthy (see previous point).

Internally, events are collected by *collector* threads, and handled by the
single *converger* thread.

### Operational considerations

`swift-drive-autopilot` runs under the assumption that a few disks are better
than no disks. If some operation relating to a single disk fails, the autopilot
will log an error and keep going. This means that it is absolutely crucial that
you have proper alerting in place for log messages with the `ERROR` label.

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
  - /dev/sd[a-z]
```

The only required field, `drives`, contains the paths of the Swift storage
drives, as a list of shell globs.

As a special rule, the autopilot will ignore all drives that contain valid
partition tables. This rule allows one to use a very general glob, like
`/dev/sd[a-z]`, without knowing the actual disk layout in advance. The system
installation will usually reside on a partitioned disk (because of the need for
special partitions such as boot and swap partition), so it will be ignored by
the autopilot. Any other disks can be used for non-Swift purposes as long as
they are partitioned into at least one partition.

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

If `chown` is set, mountpoints below `/srv/node` and `/var/cache/swift` will be chown'ed to this user
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

```yaml
swift-id-pool: [ "swift1", "swift2", "swift3", "swift4", "swift5", "swift6" ]
```

If `swift-id-pool` is set, when a new drive is formatted, it will be assigned an
unused `swift-id` from this pool. This allows a new node to go from unformatted
drives to a fully operational Swift drive setup without any human intervention.

Automatic assignment will only happen during the initial formatting (i.e. when
no LUKS container or filesystem or active mount is found on the drive).
Automatic assignment will *not* happen if there is any broken drive (since the
autopilot cannot check the broken drive's `swift-id`, any automatic assignment
could result in a duplicate `swift-id`).

### Runtime interface

The autopilot advertises its state by writing the following files and
directories:
`swift-drive-autopilot` maintains the directory `/run/swift-storage/state` to
store and advertise state information. (If a chroot is configured, then this
path refers to inside the chroot.) Currently, the following files will be
written:

* `/run/swift-storage/state/flag-ready` is an empty file whose existence marks
  that the autopilot has handled each available drive at least once. This flag
  can be used to delay the startup of Swift services until storage is available.

* `/run/swift-storage/state/unmount-propagation` is a directory containing a
  symlink for each drive that was unmounted by the autopilot. The intention
  of this mechanism is to propagate unmounting of broken drives to Swift
  services running in separate mount namespaces. For example, if the other
  service sees `/run/swift-storage/state/unmount-propagation/foo`, it shall
  unmount `/srv/node/foo` from its local mount namespace.

  `/run/swift-storage/state/unmount-propagation` can be ignored unless you have
  Swift services running in multiple private mount namespaces, typically
  because of containers and because your orchestrator cannot setup shared or
  slave mount namespaces (e.g.  Kubernetes). In plain Docker, pass `/srv/node`
  to the Swift service with the `slave` or `shared` option, and mounts/unmounts
  made by the autopilot will propagate automatically.

* `/run/swift-storage/broken` is a directory containing symlinks to all drives
  deemed broken by the autopilot. When the autopilot finds a broken device, its
  log will explain why the device is considered broken, and how to reinstate the
  device into the cluster after resolving the issue.

* Since the autopilot also does the job of `swift-drive-audit`, it honors its
  interface and writes `/var/cache/swift/drive.recon`. Drive errors detected by
  the autopilot will thus show up in `swift-recon --driveaudit`.

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

Any other Swift containers should have access to the host's
`/run/swift-storage/state` directory (using a `hostPath` volume) and wait for
the file `flag-ready` to appear before starting up.
