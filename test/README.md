<!--
SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company
SPDX-License-Identifier: Apache-2.0
-->

# Integration tests for swift-drive-autopilot

These tests check that the `swift-drive-autopilot` performs correctly against
an actual system. To run them, say

```bash
make check
```

in the repository's root directory. Note that root or `sudo` privileges are
required for mounting and unmounting, talking to device-mapper etc. If in
doubt, run this in a virtual machine.

## Structure

Individual testcases are shell scripts directly in this directory. The names of
the testcases start with two digits to provide some ordering from basic to
advanced tests. The Makefile target `check` calls [`run.sh`](./run.sh), which
runs all testcases in order.

Shared code for all testcases resides in the [`lib`](./lib) subdirectory:

* `common.sh` contains shared initialization logic and a library of functions.
* `cleanup.sh` cleans up resources from a previous testcase. It is idempotent;
  if nothing needs to be cleaned up, it will not do anything.

The testcases are ordered and also roughly organized into blocks by their
initial number:

* Tests 01, 02, etc. test drive discovery and initial setup.
* Tests 11, 12, etc. test reaction to events after the initial discovery and
  setup (drive failures, reinstatements, etc.).
