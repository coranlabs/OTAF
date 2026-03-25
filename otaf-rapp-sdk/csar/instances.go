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

package csar

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
)

func UUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// refreshElementIDs gives every automation composition element a new
// identifier.
//
// The platform remembers element ids. A row survives a failed deploy and, in
// some cases, a teardown, and creating an instance whose element id it has
// seen before is refused as a duplicate. Since the committed instance file
// carries whatever id was minted when the repository was created, shipping it
// unchanged means the second onboard of a project fails and the reason is
// nowhere near the cause.
//
// The id is local to the instance: the element is tied to its definition by
// name and version, and nothing refers to it by id. Minting a fresh one on
// every build is therefore free, and makes a package re-onboardable.
func refreshElementIDs(data []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse composition instance: %w", err)
	}

	elements, ok := doc["elements"].(map[string]any)
	if !ok || len(elements) == 0 {
		return data, nil
	}

	refreshed := make(map[string]any, len(elements))
	for _, value := range elements {
		id, err := UUID()
		if err != nil {
			return nil, fmt.Errorf("generate element id: %w", err)
		}
		if element, ok := value.(map[string]any); ok {
			element["id"] = id
		}
		refreshed[id] = value
	}
	doc["elements"] = refreshed

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode composition instance: %w", err)
	}
	return append(out, '\n'), nil
}
