// SPDX-FileCopyrightText: 2018 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package os

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sapcc/go-bits/logg"

	"github.com/sapcc/swift-drive-autopilot/pkg/command"
)

// ReadSwiftID implements the Interface interface.
func (l *Linux) ReadSwiftID(mountPath string) (string, error) {
	buf, err := os.ReadFile(swiftIDPathIn(mountPath))
	switch {
	case err == nil:
		return strings.TrimSpace(string(buf)), nil
	case os.IsNotExist(err): // not an error
		return "", nil
	default:
		return "", err
	}
}

// WriteSwiftID implements the Interface interface.
func (l *Linux) WriteSwiftID(mountPath, swiftID string) error {
	return os.WriteFile(swiftIDPathIn(mountPath), []byte(swiftID+"\n"), 0644)
}

func swiftIDPathIn(mountPath string) string {
	path := filepath.Join(mountPath, "swift-id")
	// make path relative to working directory to account for chrootPath
	return strings.TrimPrefix(path, "/")
}

// Chown implements the Interface interface.
func (l *Linux) Chown(path, user, group string) {
	var (
		cmd string
		arg string
	)

	if path == "" {
		logg.Fatal("Cannot chown empty path")
	}

	// set only those things which were given
	if user == "" {
		if group == "" {
			return // nothing to do
		}
		cmd, arg = "chgrp", group
	} else {
		cmd, arg = "chown", user
		if group != "" {
			arg += ":" + group
		}
	}

	logg.Debug("%s %s to %s", cmd, path, arg)
	command.Run(cmd, arg, path)
}
