#!/bin/bash

if [ "${DEBUG:-0}" == 1 ]; then
    set -x
    function log_debug {
        echo ">> $@"
    }
else
    function log_debug {
        return 0
    }
fi

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
    as_root env TEST_MODE=1 "${THISDIR}/../swift-drive-autopilot" "${DIR}/config.yaml" \
        | timeout 120s "${THISDIR}/logexpect" "${DIR}/pattern" > "${DIR}/log"
    log_debug "Success!"
}

# Standard execution step: Find and remove the broken flag for the given drive.
function reinstate_drive {
    for BROKEN_LINK in /run/swift-storage/broken/*; do
        if [ "$1" = "$(readlink -f "${BROKEN_LINK}")" ]; then
            as_root rm "${BROKEN_LINK}"
            return 0
        fi
    done
    echo "cannot reinstate $1: no link found in /run/swift-storage/broken" >&2
    return 1
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

# Standard verification step: Count the number of LUKS containers currently
# open and compare to the given number.
function expect_open_luks_count {
    ACTUAL="$(as_root dmsetup ls --target=crypt | grep -cE '^[0-9a-f]{32}\s' || true)"
    if [ "${ACTUAL}" -ne "$1" ]; then
        echo "expected $1 open LUKS containers, but found ${ACTUAL}:" >&2
        as_root dmsetup ls --target=crypt | grep -E '^[0-9a-f]{32}\s'
        exit 1
    fi
}

# Standard execution step: As the subshell with given index, report success.
#
# This is necessary because a subshell spawned as `( COMMAND... )&` does not
# report its exit code in a way that `set -e` could see. But each subshell has
# `set -e`, too, so if it calls report_subshell_success at the end, that proves
# that all commands before that executed successfully.
function report_subshell_success {
    touch "${DIR}/subshell-${1:-1}-success"
}

# Standard verification step: Expect that the subshell with given index completed successfully.
function expect_subshell_success {
    if [ ! -f "${DIR}/subshell-${1:-1}-success" ]; then
        echo "subshell ${1:-1} exited prematurely; see error message above" >&2
        exit 1
    fi
}
