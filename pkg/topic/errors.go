package topic

import "errors"

// Topic validation errors.
var (
	// ErrEmptyTopic indicates the topic name or filter is empty.
	ErrEmptyTopic = errors.New("topic must not be empty")

	// ErrTopicTooLong indicates the topic exceeds 65535 bytes.
	ErrTopicTooLong = errors.New("topic exceeds maximum length")

	// ErrNullCharacter indicates the topic contains a null character.
	ErrNullCharacter = errors.New("topic must not contain null character")

	// ErrWildcardInName indicates wildcards in a topic name (not allowed).
	ErrWildcardInName = errors.New("topic name must not contain wildcards")

	// ErrInvalidMultiWildcard indicates invalid multi-level wildcard usage.
	// The # wildcard must be alone in its level and be the last level.
	ErrInvalidMultiWildcard = errors.New("multi-level wildcard must occupy entire level and be last")

	// ErrInvalidSingleWildcard indicates invalid single-level wildcard usage.
	// The + wildcard must be alone in its level.
	ErrInvalidSingleWildcard = errors.New("single-level wildcard must occupy entire level")

	// ErrInvalidSharedSubscription indicates an invalid shared subscription format.
	ErrInvalidSharedSubscription = errors.New("invalid shared subscription format")
)
