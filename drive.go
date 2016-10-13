/*
 * Copyright 2016 SAP SE
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You should have received a copy of the License along with this
 * program. If not, you may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package main

import "path/filepath"

//ListDrives returns the list of all Swift storage drives, by expanding the
//shell globs in Config.DriveGlobs.
func ListDrives() ([]string, error) {
	var result []string

	for _, pattern := range Config.DriveGlobs {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		result = append(result, matches...)
	}

	return result, nil
}
