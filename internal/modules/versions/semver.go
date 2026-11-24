// Package versions contains version comparison rules used by explicit update
// commands. It has no network or filesystem effects.
package versions

import (
	"fmt"
	"regexp"
	"strings"
)

var semanticVersion = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)(?:-([0-9A-Za-z.-]+))?(?:\+([0-9A-Za-z.-]+))?$`)

type parsedVersion struct {
	major, minor, patch string
	pre                 []string
}

// Compare returns -1 when left is older, 0 when equal, and 1 when newer.
func Compare(left, right string) (int, error) {
	a, err := parse(left)
	if err != nil {
		return 0, fmt.Errorf("invalid version %q: %w", left, err)
	}
	b, err := parse(right)
	if err != nil {
		return 0, fmt.Errorf("invalid version %q: %w", right, err)
	}
	for _, pair := range [][2]string{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		if comparison := compareNumeric(pair[0], pair[1]); comparison != 0 {
			return comparison, nil
		}
	}
	if len(a.pre) == 0 && len(b.pre) == 0 {
		return 0, nil
	}
	if len(a.pre) == 0 {
		return 1, nil
	}
	if len(b.pre) == 0 {
		return -1, nil
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		leftID, rightID := a.pre[i], b.pre[i]
		leftNumeric, rightNumeric := allDigits(leftID), allDigits(rightID)
		switch {
		case leftNumeric && rightNumeric:
			if comparison := compareNumeric(leftID, rightID); comparison != 0 {
				return comparison, nil
			}
		case leftNumeric:
			return -1, nil
		case rightNumeric:
			return 1, nil
		default:
			if leftID < rightID {
				return -1, nil
			}
			if leftID > rightID {
				return 1, nil
			}
		}
	}
	if len(a.pre) < len(b.pre) {
		return -1, nil
	}
	if len(a.pre) > len(b.pre) {
		return 1, nil
	}
	return 0, nil
}

func Validate(value string) bool {
	_, err := parse(value)
	return err == nil
}

func parse(value string) (parsedVersion, error) {
	match := semanticVersion.FindStringSubmatch(value)
	if match == nil {
		return parsedVersion{}, fmt.Errorf("expected semantic version")
	}
	parts := match[1:4]
	for _, part := range parts {
		if len(part) > 1 && part[0] == '0' {
			return parsedVersion{}, fmt.Errorf("numeric components cannot contain leading zeroes")
		}
	}
	result := parsedVersion{major: parts[0], minor: parts[1], patch: parts[2]}
	if match[4] != "" {
		result.pre = strings.Split(match[4], ".")
		for _, id := range result.pre {
			if id == "" || (allDigits(id) && len(id) > 1 && id[0] == '0') {
				return parsedVersion{}, fmt.Errorf("invalid prerelease identifier")
			}
		}
	}
	if match[5] != "" {
		for _, id := range strings.Split(match[5], ".") {
			if id == "" {
				return parsedVersion{}, fmt.Errorf("invalid build metadata identifier")
			}
		}
	}
	return result, nil
}

func compareNumeric(left, right string) int {
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func allDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}
