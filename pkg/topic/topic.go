// Package topic provides MQTT topic name and filter handling.
// It implements topic validation and wildcard matching according to
// MQTT 3.1.1 Section 4.7 and MQTT 5.0 Section 4.7.
package topic

import (
	"strings"
)

const (
	// Separator is the topic level separator.
	Separator = '/'

	// MultiWildcard matches any number of levels (must be last).
	MultiWildcard = '#'

	// SingleWildcard matches exactly one level.
	SingleWildcard = '+'

	// SysPrefix is the prefix for system topics.
	SysPrefix = '$'
)

// ValidateName validates a topic name (no wildcards allowed).
// Returns nil if valid, otherwise returns an error describing the issue.
func ValidateName(name string) error {
	if len(name) == 0 {
		return ErrEmptyTopic
	}

	if len(name) > 65535 {
		return ErrTopicTooLong
	}

	// Topic names must not contain wildcards
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == MultiWildcard || c == SingleWildcard {
			return ErrWildcardInName
		}
		if c == 0 {
			return ErrNullCharacter
		}
	}

	return nil
}

// ValidateFilter validates a topic filter (wildcards allowed).
// Returns nil if valid, otherwise returns an error describing the issue.
func ValidateFilter(filter string) error {
	if len(filter) == 0 {
		return ErrEmptyTopic
	}

	if len(filter) > 65535 {
		return ErrTopicTooLong
	}

	levels := strings.Split(filter, string(Separator))

	for i, level := range levels {
		// Check for null character
		for j := 0; j < len(level); j++ {
			if level[j] == 0 {
				return ErrNullCharacter
			}
		}

		// Check multi-level wildcard
		if strings.Contains(level, string(MultiWildcard)) {
			// # must be alone in its level
			if level != string(MultiWildcard) {
				return ErrInvalidMultiWildcard
			}
			// # must be the last level
			if i != len(levels)-1 {
				return ErrInvalidMultiWildcard
			}
		}

		// Check single-level wildcard
		if strings.Contains(level, string(SingleWildcard)) {
			// + must be alone in its level
			if level != string(SingleWildcard) {
				return ErrInvalidSingleWildcard
			}
		}
	}

	return nil
}

// Match checks if a topic name matches a topic filter.
// The filter may contain wildcards (+ and #).
// Returns true if the name matches the filter.
func Match(filter, name string) bool {
	// Empty filter or name never matches
	if len(filter) == 0 || len(name) == 0 {
		return false
	}

	// $-prefixed topics don't match filters starting with wildcard
	if name[0] == SysPrefix {
		if filter[0] == MultiWildcard || filter[0] == SingleWildcard {
			return false
		}
	}

	filterLevels := strings.Split(filter, string(Separator))
	nameLevels := strings.Split(name, string(Separator))

	return matchLevels(filterLevels, nameLevels)
}

// matchLevels recursively matches topic levels.
func matchLevels(filterLevels, nameLevels []string) bool {
	for i := range filterLevels {
		filterLevel := filterLevels[i]

		// Multi-level wildcard matches everything remaining
		if filterLevel == string(MultiWildcard) {
			return true
		}

		// No more name levels but filter has more
		if i >= len(nameLevels) {
			return false
		}

		nameLevel := nameLevels[i]

		// Single-level wildcard matches any single level
		if filterLevel == string(SingleWildcard) {
			continue
		}

		// Exact match required
		if filterLevel != nameLevel {
			return false
		}
	}

	// Filter exhausted - name must also be exhausted for a match
	return len(filterLevels) == len(nameLevels)
}

// IsShared checks if a topic filter is a shared subscription.
// Shared subscriptions have the format: $share/{ShareName}/{filter}
// Returns the share name and actual filter if shared, or empty strings if not.
func IsShared(filter string) (shareName, actualFilter string, ok bool) {
	const prefix = "$share/"
	if !strings.HasPrefix(filter, prefix) {
		return "", "", false
	}

	// Find the share name
	rest := filter[len(prefix):]
	idx := strings.Index(rest, string(Separator))
	if idx <= 0 {
		return "", "", false
	}

	shareName = rest[:idx]
	actualFilter = rest[idx+1:]

	if len(actualFilter) == 0 {
		return "", "", false
	}

	return shareName, actualFilter, true
}

// ParseShared parses a shared subscription filter.
// Returns the share name, actual filter, and whether it's a shared subscription.
func ParseShared(filter string) (shareName, actualFilter string, isShared bool) {
	return IsShared(filter)
}

// Levels splits a topic into its constituent levels.
func Levels(topic string) []string {
	return strings.Split(topic, string(Separator))
}

// HasWildcard returns true if the filter contains any wildcard characters.
func HasWildcard(filter string) bool {
	return strings.ContainsAny(filter, string(MultiWildcard)+string(SingleWildcard))
}

// IsSysTopic returns true if the topic name starts with $.
func IsSysTopic(name string) bool {
	return len(name) > 0 && name[0] == SysPrefix
}
