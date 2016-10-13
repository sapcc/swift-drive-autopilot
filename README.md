# swift-storage-boot

This service finds, formats and mounts Swift storage drives, usually from
within a container on a Kubernetes host.

## Compiling

To build the binary:

```bash
make
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
$ docker run -d -v $PWD/config.yml:/config.yml -v /:/host sapcc/swift-storage-boot:latest /config.yml
```

### In Kubernetes

You will probably want to run this as a daemonset with the `nodeSelector`
matching your Swift storage nodes. Like described for Docker above, make sure
to mount the host's root filesystem into the container (with a `hostPath`
volume) and run the container in privileged mode (by setting
`securityContext.privileged` to `true` in the container spec).
