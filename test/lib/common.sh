#!/bin/bash

function log_debug {
    if [ "${DEBUG:-0}" == 1 ]; then
        echo ">> $@"
    fi
}

# choose temporary directory (this is the same for all test-runs, so that a
# subsequent run can cleanup the assets from the previous one)
DIR="${TMPDIR:-/tmp}/swift-drive-autopilot-test"
log_debug "Test assets will be placed below ${DIR}"
mkdir -p "${DIR}"

################################################################################
# library of helper functions

function as_root {
    if [ "${EUID}" == 0 ]; then
        env --unset=DEBUG "$@"
    else
        sudo "$@"
    fi
}

# Standard preparation step: Create disk image files.
function make_disk_images {
    log_debug "Allocating disk images"
    for idx in "$@"; do
        fallocate -l 100M "${DIR}/image${idx}"
    done
}

# Standard preparation step: Make loop devices out of the given disk image files.
function make_loop_devices {
    log_debug "Preparing loop devices"
    for idx in "$@"; do
        ln -sfT "$(as_root losetup --find --show "${DIR}/image${idx}")" "${DIR}/loop${idx}"
    done
}

# Standard preparation step: Send the function's stdin to the config.yaml for this test run.
function with_config {
    log_debug "Preparing autopilot config"
    cat > "${DIR}/config.yaml"
}

# Standard execution step: Send the function's stdin to the pattern file for
# this test run, then execute swift-drive-autopilot and logexpect.
function run_and_expect {
    cat > "${DIR}/pattern"
    log_debug "Starting autopilot (log output will be copied to ${DIR}/log)"
    as_root env TEST_MODE=1 "${THISDIR}/../swift-drive-autopilot" "${DIR}/config.yaml" 2>&1 \
        | "${THISDIR}/logexpect" "${DIR}/pattern" > "${DIR}/log"
    log_debug "Success!"
}

# Standard verification step: Expect a mountpoint at the given path.
function expect_mountpoint {
    if mount | grep -qF " on $1 type xfs "; then true; else
        echo "expected $1 to be a mountpoint with an XFS filesystem, but it isn't" >&2
        return 1
    fi
}

# Standard verification step: Expect no mountpoint at the given path.
function expect_no_mountpoint {
    if mount | grep -qF " on $1 "; then
        echo "expected $1 to not be a mountpoint, but it is" >&2
        return 1
    fi
}

# Standard verification step: Expect no mounts below /srv/node.
function expect_no_mounts {
    if mount | grep -qF 'on /srv/node'; then
        echo "expected no mountpoints below /srv/node, but there are some:" >&2
        mount | grep -F 'on /srv/node'
        exit 1
    fi
}
