#!/bin/bash
set -euo pipefail

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
#
# Note that each test script sets its working directory to `$THIS_REPO/test`.
function run_and_expect {
    cat > "${DIR}/pattern"
    log_debug "Starting autopilot (log output will be copied to ${DIR}/log)"
    as_root env TEST_MODE=1 ../swift-drive-autopilot "${DIR}/config.yaml" \
        | timeout 120s ./logexpect "${DIR}/pattern" > "${DIR}/log"
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

# Standard verification step: Expect a mountpoint at the given path(s).
function expect_mountpoint {
    for MOUNTPOINT in "$@"; do
        if mount | grep -qF " on ${MOUNTPOINT} type xfs "; then true; else
            echo "expected ${MOUNTPOINT} to be a mountpoint with an XFS filesystem, but it isn't" >&2
            return 1
        fi
    done
}

# Standard verification step: Expect no mountpoint at the given path(s).
function expect_no_mountpoint {
    for MOUNTPOINT in "$@"; do
        if mount | grep -qF " on ${MOUNTPOINT} "; then
            echo "expected ${MOUNTPOINT} to not be a mountpoint, but it is" >&2
            return 1
        fi
    done
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

# Standard verification step: Expect that directories exist at the given paths.
function expect_directories {
    for LOCATION in "$@"; do
        if [ ! -d "${LOCATION}" ]; then
            echo "expected directory at ${LOCATION}, but cannot find it" >&2
            exit 1
        fi
    done
}

# Standard verification step: Expect that the given path ($1) contains a regular
# file with the given content ($2).
function expect_file_with_content {
    if [ ! -f "$1" ]; then
        echo "expected file at $1, but cannot find it" >&2
        exit 1
    fi
    if [ "$(cat "$1")" != "$2" ]; then
        echo "expected file at $1 with content \"$2\", but actual content is \"$(cat "$1")\"" >&2
        exit 1
    fi
}

# Standard verification step: Expect that the given path ($1) contains a symlink
# to the given target ($2).
function expect_symlink {
    if [ ! -L "$1" ]; then
        echo "expected symlink at $1, but cannot find it" >&2
        exit 1
    fi
    if [ "$(readlink -f "$1")" != "$2" ]; then
        echo "expected symlink at $1 with target \"$2\", but actual target is \"$(readlink -f "$1")\"" >&2
        exit 1
    fi
}

# Standard verification step: Expect that the given files/symlinks/directories
# were deleted.
function expect_deleted {
    for LOCATION in "$@"; do
        if [ -e "${LOCATION}" ]; then
            echo "expected path $2 to be deleted, but it still exists" >&2
            exit 1
        fi
    done
}

# Standard verification step: Expect that the given files/symlinks/directories
# ($2, ...) belongs to the given user:group ($1).
function expect_ownership {
    local EXPECT STAT
    EXPECT="$1"
    shift
    for LOCATION in "$@"; do
        STAT="$(stat -c '%U:%G' "${LOCATION}")"
        if [ "${STAT}" != "${EXPECT}" ]; then
            echo "expected ${LOCATION} to belong to ${EXPECT}, but belongs to ${STAT}" >&2
            exit 1
        fi
    done
}
