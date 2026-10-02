package config

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

var sizePattern = regexp.MustCompile(`(?i)^([0-9]+(?:\.[0-9]+)?)(b|kb|mb|gb|tb|kib|mib|gib|tib)?$`)

// ParseSize accepts positive exact bytes with optional decimal or binary units.
func ParseSize(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if len(value) > 128 {
		return 0, fmt.Errorf("size value is too long")
	}
	match := sizePattern.FindStringSubmatch(value)
	if match == nil {
		return 0, fmt.Errorf("invalid size %q: use positive bytes or B/KB/MB/GB/TB/KiB/MiB/GiB/TiB", value)
	}
	units := map[string]int64{
		"": 1, "b": 1, "kb": 1000, "mb": 1000000, "gb": 1000000000, "tb": 1000000000000,
		"kib": 1 << 10, "mib": 1 << 20, "gib": 1 << 30, "tib": 1 << 40,
	}
	n, ok := new(big.Rat).SetString(match[1])
	if !ok {
		return 0, fmt.Errorf("invalid size %q", value)
	}
	n.Mul(n, new(big.Rat).SetInt64(units[strings.ToLower(match[2])]))
	if !n.IsInt() || !n.Num().IsInt64() || n.Sign() <= 0 {
		return 0, fmt.Errorf("invalid size %q: byte count must be a positive int64 whole number", value)
	}
	return n.Num().Int64(), nil
}
