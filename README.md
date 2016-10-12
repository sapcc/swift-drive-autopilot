# swift-storage-boot

This service finds, formats and mounts Swift storage drives, usually from
within a container on a Kubernetes host.

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
