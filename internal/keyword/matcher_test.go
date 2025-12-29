// Package keyword provides keyword matching functionality.
package keyword

import (
	"strings"
	"sync"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// genKeywordEntry generates a random keyword entry.
func genKeywordEntry() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString(),
	).Map(func(values []interface{}) Entry {
		return Entry{
			Keyword: values[0].(string),
			Reply:   values[1].(string),
		}
	})
}

// genKeywordList generates a list of keyword entries.
func genKeywordList() gopter.Gen {
	return gen.SliceOfN(5, genKeywordEntry()).SuchThat(func(entries []Entry) bool {
		// Ensure all keywords are unique (case-insensitive)
		seen := make(map[string]bool)
		for _, e := range entries {
			lower := strings.ToLower(e.Keyword)
			if seen[lower] {
				return false
			}
			seen[lower] = true
		}
		return true
	})
}

// genNonEmptyAlphaString generates a non-empty alphabetic string.
func genNonEmptyAlphaString() gopter.Gen {
	return gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 })
}

// TestProperty2_KeywordMatchingCorrectness tests Property 2: Keyword Matching Correctness
// **Feature: go-telegram-intent-bot, Property 2: Keyword Matching Correctness**
// **Validates: Requirements 2.1, 2.2, 2.3**
//
// For any keyword list and message text:
// - If the text contains a keyword, Match SHALL return that keyword
// - If the text contains multiple keywords, Match SHALL return the first one in configuration order
// - If the text contains no keywords, Match SHALL return nil
func TestProperty2_KeywordMatchingCorrectness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 2a: If text contains a keyword, Match returns that keyword (Requirement 2.1)
	properties.Property("text containing keyword returns match", prop.ForAll(
		func(keywords []Entry, prefix string, suffix string) bool {
			if len(keywords) == 0 {
				return true // Skip empty keyword lists
			}
			matcher := NewMatcher(keywords)
			// Pick a random keyword and embed it in text
			keyword := keywords[0]
			text := prefix + keyword.Keyword + suffix
			result := matcher.Match(text)
			if result == nil {
				return false
			}
			// The matched keyword should be contained in the text (case-insensitive)
			return strings.Contains(strings.ToLower(text), strings.ToLower(result.Keyword))
		},
		genKeywordList().SuchThat(func(k []Entry) bool { return len(k) > 0 }),
		gen.AlphaString(),
		gen.AlphaString(),
	))

	// Property 2b: If text contains multiple keywords, Match returns the first one (Requirement 2.2)
	properties.Property("multiple keywords returns first in order", prop.ForAll(
		func(keywords []Entry) bool {
			if len(keywords) < 2 {
				return true // Need at least 2 keywords
			}
			matcher := NewMatcher(keywords)
			// Create text containing all keywords
			var textParts []string
			for _, k := range keywords {
				textParts = append(textParts, k.Keyword)
			}
			text := strings.Join(textParts, " ")
			result := matcher.Match(text)
			if result == nil {
				return false
			}
			// Should return the first keyword
			return strings.EqualFold(result.Keyword, keywords[0].Keyword)
		},
		genKeywordList().SuchThat(func(k []Entry) bool { return len(k) >= 2 }),
	))

	// Property 2c: If text contains no keywords, Match returns nil (Requirement 2.3)
	properties.Property("text without keywords returns nil", prop.ForAll(
		func(keywords []Entry, text string) bool {
			matcher := NewMatcher(keywords)
			// Ensure text doesn't contain any keyword
			lowerText := strings.ToLower(text)
			for _, k := range keywords {
				if strings.Contains(lowerText, strings.ToLower(k.Keyword)) {
					return true // Skip this case - text accidentally contains a keyword
				}
			}
			result := matcher.Match(text)
			return result == nil
		},
		genKeywordList(),
		genNonEmptyAlphaString(),
	))

	properties.TestingRun(t)
}

// TestProperty3_KeywordHotUpdate tests Property 3: Keyword Hot Update
// **Feature: go-telegram-intent-bot, Property 3: Keyword Hot Update**
// **Validates: Requirements 2.4**
//
// For any keyword matcher, after calling UpdateKeywords with a new list,
// subsequent Match calls SHALL use the new keyword list.
func TestProperty3_KeywordHotUpdate(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 3: After UpdateKeywords, Match uses the new list
	properties.Property("UpdateKeywords changes matching behavior", prop.ForAll(
		func(oldKeywords []Entry, newKeywords []Entry) bool {
			if len(newKeywords) == 0 {
				return true // Skip empty new keyword lists
			}
			
			// Create matcher with old keywords
			matcher := NewMatcher(oldKeywords)
			
			// Update to new keywords
			matcher.UpdateKeywords(newKeywords)
			
			// Create text containing the first new keyword
			text := "prefix " + newKeywords[0].Keyword + " suffix"
			result := matcher.Match(text)
			
			// Should match the new keyword
			if result == nil {
				return false
			}
			return strings.EqualFold(result.Keyword, newKeywords[0].Keyword)
		},
		genKeywordList(),
		genKeywordList().SuchThat(func(k []Entry) bool { return len(k) > 0 }),
	))

	// Property 3b: Old keywords no longer match after update
	properties.Property("old keywords no longer match after update", prop.ForAll(
		func(oldKeyword Entry, newKeywords []Entry) bool {
			// Ensure old keyword is not in new keywords
			for _, k := range newKeywords {
				if strings.EqualFold(k.Keyword, oldKeyword.Keyword) {
					return true // Skip - old keyword is in new list
				}
			}
			
			// Create matcher with old keyword
			matcher := NewMatcher([]Entry{oldKeyword})
			
			// Verify old keyword matches before update
			text := "test " + oldKeyword.Keyword + " text"
			if matcher.Match(text) == nil {
				return false // Old keyword should match
			}
			
			// Update to new keywords (without old keyword)
			matcher.UpdateKeywords(newKeywords)
			
			// Old keyword should no longer match (unless accidentally in text)
			result := matcher.Match(text)
			if result == nil {
				return true // Correctly no match
			}
			// If there's a match, it should be from new keywords, not old
			for _, k := range newKeywords {
				if strings.EqualFold(result.Keyword, k.Keyword) {
					return true // Match is from new keywords
				}
			}
			return false // Match is from old keyword - error
		},
		genKeywordEntry(),
		genKeywordList(),
	))

	properties.TestingRun(t)
}

// TestMatcherConcurrency tests that the matcher is thread-safe
func TestMatcherConcurrency(t *testing.T) {
	keywords := []Entry{
		{Keyword: "hello", Reply: "world"},
		{Keyword: "foo", Reply: "bar"},
	}
	matcher := NewMatcher(keywords)

	var wg sync.WaitGroup
	const numGoroutines = 100

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			matcher.Match("hello world")
		}()
	}

	// Concurrent updates
	for i := 0; i < numGoroutines/10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			newKeywords := []Entry{
				{Keyword: "test", Reply: "reply"},
			}
			matcher.UpdateKeywords(newKeywords)
		}(i)
	}

	wg.Wait()
}

// TestMatchCaseInsensitive tests case-insensitive matching
func TestMatchCaseInsensitive(t *testing.T) {
	keywords := []Entry{
		{Keyword: "Hello", Reply: "world"},
	}
	matcher := NewMatcher(keywords)

	testCases := []struct {
		text     string
		expected bool
	}{
		{"hello", true},
		{"HELLO", true},
		{"HeLLo", true},
		{"say hello there", true},
		{"goodbye", false},
	}

	for _, tc := range testCases {
		result := matcher.Match(tc.text)
		if tc.expected && result == nil {
			t.Errorf("expected match for '%s', got nil", tc.text)
		}
		if !tc.expected && result != nil {
			t.Errorf("expected no match for '%s', got %v", tc.text, result)
		}
	}
}

// TestMatchEmptyKeywords tests matching with empty keyword list
func TestMatchEmptyKeywords(t *testing.T) {
	matcher := NewMatcher([]Entry{})
	result := matcher.Match("any text")
	if result != nil {
		t.Errorf("expected nil for empty keywords, got %v", result)
	}
}

// TestMatchEmptyText tests matching with empty text
func TestMatchEmptyText(t *testing.T) {
	keywords := []Entry{
		{Keyword: "hello", Reply: "world"},
	}
	matcher := NewMatcher(keywords)
	result := matcher.Match("")
	if result != nil {
		t.Errorf("expected nil for empty text, got %v", result)
	}
}
