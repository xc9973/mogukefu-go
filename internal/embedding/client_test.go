// Package embedding provides the embedding client for text-to-vector conversion.
package embedding

import (
	"math"
	"reflect"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// genNonZeroVector generates a non-zero vector of the specified dimension.
func genNonZeroVector(dim int) gopter.Gen {
	return gen.SliceOfN(dim, gen.Float64Range(-100.0, 100.0)).SuchThat(func(v []float64) bool {
		// Ensure at least one non-zero element
		for _, x := range v {
			if x != 0 {
				return true
			}
		}
		return false
	})
}

// genZeroVector generates a zero vector of the specified dimension.
func genZeroVector(dim int) gopter.Gen {
	return gen.Const(make([]float64, dim))
}

// genVectorPair generates two vectors of the same dimension.
func genVectorPair() gopter.Gen {
	return gen.IntRange(1, 100).FlatMap(func(dim interface{}) gopter.Gen {
		d := dim.(int)
		return gopter.CombineGens(
			genNonZeroVector(d),
			genNonZeroVector(d),
		).Map(func(values []interface{}) [2][]float64 {
			return [2][]float64{values[0].([]float64), values[1].([]float64)}
		})
	}, reflect.TypeOf([2][]float64{}))
}

// dotProduct calculates the dot product of two vectors.
func dotProduct(a, b []float64) float64 {
	var sum float64
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// norm calculates the Euclidean norm of a vector.
func norm(v []float64) float64 {
	var sum float64
	for _, x := range v {
		sum += x * x
	}
	return math.Sqrt(sum)
}

// expectedCosineSimilarity calculates the expected cosine similarity.
func expectedCosineSimilarity(a, b []float64) float64 {
	normA := norm(a)
	normB := norm(b)
	if normA == 0 || normB == 0 {
		return 0
	}
	return dotProduct(a, b) / (normA * normB)
}

// TestProperty5_CosineSimilarityCalculation tests Property 5: Cosine Similarity Calculation
// **Feature: go-telegram-intent-bot, Property 5: Cosine Similarity Calculation**
// **Validates: Requirements 3.6**
//
// For any two vectors A and B, the calculated cosine similarity SHALL equal
// dot(A,B) / (norm(A) * norm(B)), with the result in range [-1, 1].
func TestProperty5_CosineSimilarityCalculation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 5a: Cosine similarity equals dot(A,B) / (norm(A) * norm(B))
	properties.Property("cosine similarity equals expected formula", prop.ForAll(
		func(pair [2][]float64) bool {
			a, b := pair[0], pair[1]
			actual := CosineSimilarity(a, b)
			expected := expectedCosineSimilarity(a, b)
			// Allow small floating point tolerance
			return math.Abs(actual-expected) < 1e-10
		},
		genVectorPair(),
	))

	// Property 5b: Result is in range [-1, 1]
	properties.Property("result is in range [-1, 1]", prop.ForAll(
		func(pair [2][]float64) bool {
			a, b := pair[0], pair[1]
			result := CosineSimilarity(a, b)
			return result >= -1.0 && result <= 1.0
		},
		genVectorPair(),
	))

	// Property 5c: Cosine similarity is symmetric
	properties.Property("cosine similarity is symmetric", prop.ForAll(
		func(pair [2][]float64) bool {
			a, b := pair[0], pair[1]
			return CosineSimilarity(a, b) == CosineSimilarity(b, a)
		},
		genVectorPair(),
	))

	// Property 5d: Identical vectors have similarity 1
	properties.Property("identical vectors have similarity 1", prop.ForAll(
		func(v []float64) bool {
			result := CosineSimilarity(v, v)
			return math.Abs(result-1.0) < 1e-10
		},
		gen.IntRange(1, 100).FlatMap(func(dim interface{}) gopter.Gen {
			return genNonZeroVector(dim.(int))
		}, reflect.TypeOf([]float64{})),
	))

	// Property 5e: Zero vectors return 0
	properties.Property("zero vector returns 0", prop.ForAll(
		func(dim int) bool {
			zero := make([]float64, dim)
			nonZero := make([]float64, dim)
			nonZero[0] = 1.0 // Make it non-zero
			
			// Zero with zero
			if CosineSimilarity(zero, zero) != 0 {
				return false
			}
			// Zero with non-zero
			if CosineSimilarity(zero, nonZero) != 0 {
				return false
			}
			// Non-zero with zero
			if CosineSimilarity(nonZero, zero) != 0 {
				return false
			}
			return true
		},
		gen.IntRange(1, 100),
	))

	// Property 5f: Mismatched dimensions return 0
	properties.Property("mismatched dimensions return 0", prop.ForAll(
		func(dim1, dim2 int) bool {
			if dim1 == dim2 {
				return true // Skip when dimensions match
			}
			a := make([]float64, dim1)
			b := make([]float64, dim2)
			a[0] = 1.0
			b[0] = 1.0
			return CosineSimilarity(a, b) == 0
		},
		gen.IntRange(1, 50),
		gen.IntRange(1, 50),
	))

	// Property 5g: Empty vectors return 0
	properties.Property("empty vectors return 0", prop.ForAll(
		func(_ int) bool {
			empty := []float64{}
			return CosineSimilarity(empty, empty) == 0
		},
		gen.IntRange(1, 10), // Just need something to generate
	))

	properties.TestingRun(t)
}
