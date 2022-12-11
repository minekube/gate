package suggest

import (
	"sort"
	"strings"

	"github.com/agext/levenshtein"
	"go.minekube.com/brigodier"
)

const DefaultMinimumSimilarityScore = 0.2

// Similar calls SimilarScore with DefaultMinimumSimilarityScore.
func Similar(builder *brigodier.SuggestionsBuilder, candidates []string) *brigodier.SuggestionsBuilder {
	return SimilarScore(builder, candidates, DefaultMinimumSimilarityScore)
}

// SimilarScore sorts and suggests only similar matching candidates based on the current argument input.
//
// It filters and sorts the best matching candidates based
// on the levenshtein similarity score of the current input so far.
//
// A candidate Score below minScore is dropped from the suggestions.
// No candidates are dropped when minScore >= 1, this is useful for using
// this function for sorting candidates by score only.
func SimilarScore(builder *brigodier.SuggestionsBuilder, candidates []string, minScore float64) *brigodier.SuggestionsBuilder {
	input := builder.Input
	if input == "" {
		return builder
	}
	given := input[strings.LastIndex(input, " ")+1:] // TODO not working with quoted arguments; use builder.Remaining?
	var result []suggestion
	for _, text := range candidates {
		score := Score(given, text)
		if score < minScore {
			continue
		}
		result = append(result, suggestion{
			text:  text,
			score: score,
		})
	}
	sortSuggestions(result)
	for _, s := range result {
		builder.Suggest(s.text)
	}
	return builder
}

type suggestion struct {
	text  string
	score float64
}

func sortSuggestions(s []suggestion) {
	sort.Slice(s, func(i, j int) bool {
		return s[i].score > s[j].score
	})
}

// Score calculates the similarity score in the range of 0..1 of two strings.
// A score of 1 means the strings are identical, and 0 means they have nothing in common.
func Score(given, suggestion string) float64 {
	i := len(given)
	if len(suggestion) < i {
		i = len(suggestion)
	}
	return levenshtein.Similarity(given, suggestion[:i], nil)
}
