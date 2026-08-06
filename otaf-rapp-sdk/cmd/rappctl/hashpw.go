// Copyright 2025-2026 coRAN LABS Private Limited
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	rappsdk "github.com/coranlabs/OTAF/otaf-rapp-sdk"
	"github.com/coranlabs/OTAF/otaf-rapp-sdk/auth"
)

func runHashpw(args []string) error {
	var password string

	if len(args) > 0 {
		password = args[0]
	} else {
		// Reading from stdin keeps the password out of the shell history.
		fmt.Fprint(os.Stderr, "password: ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return err
		}
		password = strings.TrimRight(line, "\r\n")
		fmt.Fprintln(os.Stderr)
	}

	if password == "" {
		return fmt.Errorf("password must not be empty")
	}

	hash, err := auth.Hash(password)
	if err != nil {
		return err
	}
	fmt.Println(hash)
	return nil
}

func runVersion() error {
	fmt.Println(rappsdk.UserAgent)
	return nil
}
