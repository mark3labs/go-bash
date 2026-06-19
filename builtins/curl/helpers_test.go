package curl_test

import (
	"os"
	"sort"
	"strings"
)

// readJSONNames lists *.json basenames (without extension) in dir,
// sorted for deterministic test order. Reused only by the comparison
// subtest; lives in its own file so future fixture-adjacent helpers
// have a home.
func readJSONNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		out = append(out, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Strings(out)
	return out, nil
}
