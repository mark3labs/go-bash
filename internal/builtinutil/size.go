package builtinutil

import (
	"fmt"
)

// HumanSize formats n using IEC suffixes (B, K, M, G, T, P, E) with a
// 1024 divisor, matching coreutils `du -h` / `ls -h` rounding rules:
//
//   - n < 1024 -> "<n>" (no suffix; integer)
//   - otherwise pick the largest unit with quotient >= 1
//   - render with one decimal place if quotient < 10, else integer
//     ("9.8K", "10K", "1023K")
//   - always round UP (ceiling) to match coreutils behavior
func HumanSize(n int64) string {
	if n < 0 {
		return "-" + HumanSize(-n)
	}
	const base = 1024
	if n < base {
		return fmt.Sprintf("%d", n)
	}
	suffixes := []string{"K", "M", "G", "T", "P", "E"}
	v := float64(n)
	i := -1
	for v >= base && i < len(suffixes)-1 {
		v /= base
		i++
	}
	// Ceiling-round.
	if v < 10 {
		// One decimal, ceiling.
		rounded := float64(int64(v*10+0.999999)) / 10
		if rounded >= 10 {
			return fmt.Sprintf("%d%s", int64(rounded+0.5), suffixes[i])
		}
		return fmt.Sprintf("%.1f%s", rounded, suffixes[i])
	}
	rounded := int64(v + 0.999999)
	return fmt.Sprintf("%d%s", rounded, suffixes[i])
}
