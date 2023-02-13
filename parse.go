// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.  All rights reserved.
// ------------------------------------------------------------

package testproxy

import (
	"os"
	"strings"
)

func Load(root string) error {
	envFile, err := os.ReadFile(root)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(envFile), "\n") {
		splits := strings.Split(line, " ")
		if len(splits) != 2 {
			continue
		}

		os.Setenv(splits[0], strings.TrimSuffix(splits[1],"\r"))
	}

	return nil
}
