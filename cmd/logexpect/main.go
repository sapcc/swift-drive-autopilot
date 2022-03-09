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
	"os"
	"os/exec"
	"regexp"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <command-file>", os.Args[0])
		os.Exit(1)
	}

	commandBytes, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	commands := filterEmptyCommands(strings.Split(string(commandBytes), "\n"))

	//echo all reads from stdin onto stdout
	in := io.TeeReader(os.Stdin, os.Stdout)

	err = matchCommands(in, commands)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func filterEmptyCommands(commands []string) []string {
	result := make([]string, 0, len(commands))
	for _, str := range commands {
		str = strings.TrimSpace(str)
		if str != "" {
			result = append(result, str)
		}
	}
	return result
}

func matchCommands(input io.Reader, commands []string) error {
	vars := make(map[string]string)
	reader := bufio.NewReader(input)
	eof := false

	for len(commands) > 0 && !eof {
		//fetch next command
		command := commands[0]
		commands = commands[1:]

		//if it's a pattern, match with input
		if strings.HasPrefix(command, ">") {
			pattern := strings.TrimPrefix(command, ">")
			err := matchPattern(reader, pattern, vars)
			eof = err == io.EOF
			if err != nil && !eof {
				return err
			}
			continue
		}

		//if it's a command, execute
		if strings.HasPrefix(command, "$") {
			err := executeScript(strings.TrimPrefix(command, "$"), vars)
			if err != nil {
				return err
			}
			continue
		}

		//what is this?
		return fmt.Errorf("malformed command (neither a pattern nor a script): %s", command)
	}

	//check if command list was exhausted
	if len(commands) > 0 {
		return fmt.Errorf("unexpected EOF while executing command: %s", commands[0])
	}
	return nil
}

//the backslashes in variableRxQuoted are necessary because this regexp works on QuoteMeta(pattern)
var variableRxQuoted = regexp.MustCompile(`\\{\\{[a-zA-Z][a-zA-Z0-9_]+\\}\\}`)
var variableRx = regexp.MustCompile(`{{[a-zA-Z][a-zA-Z0-9_]+}}`)
var timestampRx = regexp.MustCompile(`^\d{4}/\d{2}/\d{2}\s*\d{2}:\d{2}:\d{2}\s*`)
var whitespaceRx = regexp.MustCompile(`\s+`)

func matchPattern(reader *bufio.Reader, pattern string, vars map[string]string) error {
	//get next input line
	line, err := reader.ReadString('\n')
	eof := err == io.EOF
	if err != nil && !eof {
		return err
	}

	//skip empty input lines
	if strings.TrimSpace(line) == "" {
		if eof {
			return io.EOF
		}
		return matchPattern(reader, pattern, vars)
	}

	//everybody hates whitespace
	line = whitespaceRx.ReplaceAllString(strings.TrimSpace(line), " ")
	pattern = whitespaceRx.ReplaceAllString(strings.TrimSpace(pattern), " ")

	//remove timestamp from input, if any
	line = timestampRx.ReplaceAllString(line, "")

	//compile pattern into a regex
	var captures []string
	patternRxStr := variableRxQuoted.ReplaceAllStringFunc(
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

	if eof {
		return io.EOF
	}
	return nil
}

func executeScript(script string, vars map[string]string) error {
	script = strings.TrimSpace(script)

	//if script contains any variable references like {{foo}}, replace them with the actual values
	var err error
	compiledScript := variableRx.ReplaceAllStringFunc(script, func(match string) string {
		//get variable name from match
		name := strings.TrimPrefix(strings.TrimSuffix(match, "}}"), "{{")

		value, ok := vars[name]
		if ok {
			return value
		}
		err = fmt.Errorf("unknown variable \"%s\" in shell script input: %s", name, script)
		return ""
	})
	if err != nil {
		return err
	}

	cmd := exec.Command("/bin/bash", "-c", compiledScript)
	cmd.Stdin = nil
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
