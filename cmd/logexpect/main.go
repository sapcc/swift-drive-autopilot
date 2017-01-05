/*******************************************************************************
*
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
*******************************************************************************/

package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <pattern-file>", os.Args[0])
		os.Exit(1)
	}

	pattern, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	patterns := filterEmptyPatterns(strings.Split(string(pattern), "\n"))

	err = matchPatterns(os.Stdin, os.Stdout, patterns)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func filterEmptyPatterns(patterns []string) []string {
	result := make([]string, 0, len(patterns))
	for _, str := range patterns {
		str = strings.TrimSpace(str)
		if str != "" {
			result = append(result, str)
		}
	}
	return result
}

func matchPatterns(input io.Reader, output io.Writer, patterns []string) error {
	vars := make(map[string]string)
	reader := bufio.NewReader(input)
	eof := false

	for len(patterns) > 0 && !eof {
		//get next input line
		inputLine, err := reader.ReadString('\n')
		eof = err == io.EOF
		if err != nil && !eof {
			return err
		}

		//skip empty input lines
		if strings.TrimSpace(inputLine) == "" {
			continue
		}

		//echo input line onto output writer if requested
		if output != nil {
			output.Write([]byte(inputLine))
		}

		//consume next pattern and compare
		pattern := patterns[0]
		patterns = patterns[1:]
		err = matchPattern(inputLine, pattern, vars)
		if err != nil {
			return err
		}
	}

	//check if pattern was exhausted
	if len(patterns) > 0 {
		return fmt.Errorf("unexpected EOF\nexpected: %s", patterns[0])
	}
	return nil
}

//the backslashes in variableRx are necessary because this regexp works on QuoteMeta(pattern)
var variableRx = regexp.MustCompile(`\\{\\{[a-zA-Z][a-zA-Z0-9_]+\\}\\}`)
var timestampRx = regexp.MustCompile(`^\d{4}/\d{2}/\d{2}\s*\d{2}:\d{2}:\d{2}\s*`)
var whitespaceRx = regexp.MustCompile(`\s+`)

func matchPattern(line string, pattern string, vars map[string]string) error {
	//everybody hates whitespace
	line = whitespaceRx.ReplaceAllString(strings.TrimSpace(line), " ")
	pattern = whitespaceRx.ReplaceAllString(strings.TrimSpace(pattern), " ")

	//remove timestamp from input, if any
	line = timestampRx.ReplaceAllString(line, "")

	//compile pattern into a regex
	var captures []string
	patternRxStr := variableRx.ReplaceAllStringFunc(
		regexp.QuoteMeta(pattern),
		func(match string) string {
			//get variable name from match
			name := strings.TrimPrefix(strings.TrimSuffix(match, `\}\}`), `\{\{`)

			//if value of variable is known from previous pattern match, use value
			value, ok := vars[name]
			if ok {
				return regexp.QuoteMeta(value)
			}

			//value of variable is not yet known - catch its value
			captures = append(captures, name)
			return `(.+)`
		},
	)

	//check if line matches pattern
	match := regexp.MustCompile("^" + patternRxStr + "$").FindStringSubmatch(line)
	if match == nil {
		return fmt.Errorf("log line does not match expectation\nexpected: %s\ncompiled: %s\n  actual: %s", pattern, patternRxStr, line)
	}

	//if new variables were introduced in this pattern, remember their values
	for idx, name := range captures {
		//if this variable catched eariler in the same match, ensure that both values are identical
		value, ok := vars[name]
		if ok {
			if value != match[idx+1] {
				return fmt.Errorf("log line does not match expectation\nexpected: %s\ncompiled: %s\n  actual: %s", pattern, patternRxStr, line)
			}
		} else {
			vars[name] = match[idx+1]
		}

	}

	return nil
}
