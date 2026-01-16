//go:build coverage

package ui

// Select is a stub for coverage builds.
// The real implementation is in selector_nocov.go.
func Select[T any](opts SelectorOptions[T]) (T, error) {
	var zero T
	return zero, ErrNoSelection
}

// SelectFromList is a stub for coverage builds.
func SelectFromList[T any](items []T, renderer ItemRenderer[T]) (T, error) {
	var zero T
	return zero, ErrNoSelection
}

// SelectFromListWithAction is a stub for coverage builds.
func SelectFromListWithAction[T any](items []T, renderer ItemRenderer[T], customAction CustomAction[T], actionKey string, onOpen CustomAction[T], filterFunc func(T, bool) bool, onSelect CustomAction[T], customActionSecond CustomAction[T], actionKeySecond string) (T, error) {
	var zero T
	return zero, ErrNoSelection
}
