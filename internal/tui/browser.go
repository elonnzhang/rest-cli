package tui

import (
	"github.com/sahilm/fuzzy"
)

// FuzzyFilter returns the subset of files matching query, ranked by score.
// If query is empty, all files are returned unchanged.
func FuzzyFilter(query string, files []string) []string {
	if query == "" {
		result := make([]string, len(files))
		copy(result, files)
		return result
	}

	matches := fuzzy.Find(query, files)
	result := make([]string, len(matches))
	for i, m := range matches {
		result[i] = files[m.Index]
	}
	return result
}

// FuzzyMatchIndices returns the indices into labels that match query, ranked by score.
// If query is empty, all indices are returned in order.
func FuzzyMatchIndices(query string, labels []string) []int {
	if query == "" {
		idx := make([]int, len(labels))
		for i := range labels {
			idx[i] = i
		}
		return idx
	}
	matches := fuzzy.Find(query, labels)
	result := make([]int, len(matches))
	for i, m := range matches {
		result[i] = m.Index
	}
	return result
}
