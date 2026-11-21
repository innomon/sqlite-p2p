// Package vect provides pure Go SQLite vector similarity search functions
// and Little-Endian IEEE 754 float32 binary serialization helpers.
package vect

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
)

// VectorLength validates that b is a packed float32 byte slice (multiple of 4 bytes)
// and returns the number of float32 dimensions.
func VectorLength(b []byte) (int, error) {
	if len(b)%4 != 0 {
		return 0, fmt.Errorf("invalid blob length %d: must be a multiple of 4", len(b))
	}
	return len(b) / 4, nil
}

// BytesToFloat32 converts a Little-Endian byte slice into []float32.
func BytesToFloat32(b []byte) ([]float32, error) {
	dim, err := VectorLength(b)
	if err != nil {
		return nil, err
	}
	floats := make([]float32, dim)
	for i := 0; i < dim; i++ {
		bits := binary.LittleEndian.Uint32(b[i*4:])
		floats[i] = math.Float32frombits(bits)
	}
	return floats, nil
}

// Float32ToBytes converts []float32 into a Little-Endian byte slice.
func Float32ToBytes(floats []float32) []byte {
	b := make([]byte, len(floats)*4)
	for i, f := range floats {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

// PackF32JSON parses a JSON string array of floating-point numbers (e.g. "[0.1, -0.4, 0.8]")
// and returns a packed Little-Endian float32 byte slice.
func PackF32JSON(text string) ([]byte, error) {
	var floats []float32
	if err := json.Unmarshal([]byte(text), &floats); err != nil {
		return nil, fmt.Errorf("invalid float32 JSON array: %w", err)
	}
	if floats == nil {
		return nil, fmt.Errorf("invalid float32 JSON array: null value")
	}
	return Float32ToBytes(floats), nil
}

// CosineSimilarity computes the cosine similarity between two float32 slices in [-1.0, 1.0].
// Returns 0.0 if either vector has zero magnitude.
func CosineSimilarity(u, v []float32) (float64, error) {
	if len(u) != len(v) {
		return 0, fmt.Errorf("dimension mismatch: %d vs %d", len(u), len(v))
	}

	var dot0, dot1, dot2, dot3 float64
	var normU0, normU1, normU2, normU3 float64
	var normV0, normV1, normV2, normV3 float64

	n := len(u)
	i := 0
	for ; i <= n-4; i += 4 {
		u0 := float64(u[i])
		u1 := float64(u[i+1])
		u2 := float64(u[i+2])
		u3 := float64(u[i+3])

		v0 := float64(v[i])
		v1 := float64(v[i+1])
		v2 := float64(v[i+2])
		v3 := float64(v[i+3])

		dot0 += u0 * v0
		dot1 += u1 * v1
		dot2 += u2 * v2
		dot3 += u3 * v3

		normU0 += u0 * u0
		normU1 += u1 * u1
		normU2 += u2 * u2
		normU3 += u3 * u3

		normV0 += v0 * v0
		normV1 += v1 * v1
		normV2 += v2 * v2
		normV3 += v3 * v3
	}

	dot := (dot0 + dot1) + (dot2 + dot3)
	normU := (normU0 + normU1) + (normU2 + normU3)
	normV := (normV0 + normV1) + (normV2 + normV3)

	for ; i < n; i++ {
		ui := float64(u[i])
		vi := float64(v[i])
		dot += ui * vi
		normU += ui * ui
		normV += vi * vi
	}

	if normU == 0 || normV == 0 {
		return 0.0, nil
	}

	return dot / math.Sqrt(normU*normV), nil
}

// CosineDistance computes 1.0 - CosineSimilarity(u, v) in [0.0, 2.0].
func CosineDistance(u, v []float32) (float64, error) {
	sim, err := CosineSimilarity(u, v)
	if err != nil {
		return 0, err
	}
	return 1.0 - sim, nil
}

// L2Distance computes the Euclidean (L2) distance between two float32 slices.
func L2Distance(u, v []float32) (float64, error) {
	if len(u) != len(v) {
		return 0, fmt.Errorf("dimension mismatch: %d vs %d", len(u), len(v))
	}

	var sum0, sum1, sum2, sum3 float64
	n := len(u)
	i := 0
	for ; i <= n-4; i += 4 {
		d0 := float64(u[i]) - float64(v[i])
		d1 := float64(u[i+1]) - float64(v[i+1])
		d2 := float64(u[i+2]) - float64(v[i+2])
		d3 := float64(u[i+3]) - float64(v[i+3])

		sum0 += d0 * d0
		sum1 += d1 * d1
		sum2 += d2 * d2
		sum3 += d3 * d3
	}

	sum := (sum0 + sum1) + (sum2 + sum3)
	for ; i < n; i++ {
		d := float64(u[i]) - float64(v[i])
		sum += d * d
	}

	return math.Sqrt(sum), nil
}

// CosineSimilarityBytes computes cosine similarity directly over two Little-Endian
// float32 byte slices without allocating intermediate []float32 slices.
func CosineSimilarityBytes(b1, b2 []byte) (float64, error) {
	if len(b1)%4 != 0 {
		return 0, fmt.Errorf("invalid first blob length %d: must be a multiple of 4", len(b1))
	}
	if len(b2)%4 != 0 {
		return 0, fmt.Errorf("invalid second blob length %d: must be a multiple of 4", len(b2))
	}
	if len(b1) != len(b2) {
		return 0, fmt.Errorf("dimension mismatch: %d vs %d", len(b1)/4, len(b2)/4)
	}

	var dot0, dot1, dot2, dot3 float64
	var normU0, normU1, normU2, normU3 float64
	var normV0, normV1, normV2, normV3 float64

	n := len(b1)
	i := 0
	for ; i <= n-16; i += 16 {
		u0 := float64(math.Float32frombits(binary.LittleEndian.Uint32(b1[i:])))
		u1 := float64(math.Float32frombits(binary.LittleEndian.Uint32(b1[i+4:])))
		u2 := float64(math.Float32frombits(binary.LittleEndian.Uint32(b1[i+8:])))
		u3 := float64(math.Float32frombits(binary.LittleEndian.Uint32(b1[i+12:])))

		v0 := float64(math.Float32frombits(binary.LittleEndian.Uint32(b2[i:])))
		v1 := float64(math.Float32frombits(binary.LittleEndian.Uint32(b2[i+4:])))
		v2 := float64(math.Float32frombits(binary.LittleEndian.Uint32(b2[i+8:])))
		v3 := float64(math.Float32frombits(binary.LittleEndian.Uint32(b2[i+12:])))

		dot0 += u0 * v0
		dot1 += u1 * v1
		dot2 += u2 * v2
		dot3 += u3 * v3

		normU0 += u0 * u0
		normU1 += u1 * u1
		normU2 += u2 * u2
		normU3 += u3 * u3

		normV0 += v0 * v0
		normV1 += v1 * v1
		normV2 += v2 * v2
		normV3 += v3 * v3
	}

	dot := (dot0 + dot1) + (dot2 + dot3)
	normU := (normU0 + normU1) + (normU2 + normU3)
	normV := (normV0 + normV1) + (normV2 + normV3)

	for ; i < n; i += 4 {
		ui := float64(math.Float32frombits(binary.LittleEndian.Uint32(b1[i:])))
		vi := float64(math.Float32frombits(binary.LittleEndian.Uint32(b2[i:])))
		dot += ui * vi
		normU += ui * ui
		normV += vi * vi
	}

	if normU == 0 || normV == 0 {
		return 0.0, nil
	}

	return dot / math.Sqrt(normU*normV), nil
}

// CosineDistanceBytes computes 1.0 - CosineSimilarityBytes(b1, b2) directly over Little-Endian byte slices.
func CosineDistanceBytes(b1, b2 []byte) (float64, error) {
	sim, err := CosineSimilarityBytes(b1, b2)
	if err != nil {
		return 0, err
	}
	return 1.0 - sim, nil
}

// L2DistanceBytes computes Euclidean distance directly over two Little-Endian float32 byte slices.
func L2DistanceBytes(b1, b2 []byte) (float64, error) {
	if len(b1)%4 != 0 {
		return 0, fmt.Errorf("invalid first blob length %d: must be a multiple of 4", len(b1))
	}
	if len(b2)%4 != 0 {
		return 0, fmt.Errorf("invalid second blob length %d: must be a multiple of 4", len(b2))
	}
	if len(b1) != len(b2) {
		return 0, fmt.Errorf("dimension mismatch: %d vs %d", len(b1)/4, len(b2)/4)
	}

	var sum0, sum1, sum2, sum3 float64
	n := len(b1)
	i := 0
	for ; i <= n-16; i += 16 {
		d0 := float64(math.Float32frombits(binary.LittleEndian.Uint32(b1[i:]))) - float64(math.Float32frombits(binary.LittleEndian.Uint32(b2[i:])))
		d1 := float64(math.Float32frombits(binary.LittleEndian.Uint32(b1[i+4:]))) - float64(math.Float32frombits(binary.LittleEndian.Uint32(b2[i+4:])))
		d2 := float64(math.Float32frombits(binary.LittleEndian.Uint32(b1[i+8:]))) - float64(math.Float32frombits(binary.LittleEndian.Uint32(b2[i+8:])))
		d3 := float64(math.Float32frombits(binary.LittleEndian.Uint32(b1[i+12:]))) - float64(math.Float32frombits(binary.LittleEndian.Uint32(b2[i+12:])))

		sum0 += d0 * d0
		sum1 += d1 * d1
		sum2 += d2 * d2
		sum3 += d3 * d3
	}

	sum := (sum0 + sum1) + (sum2 + sum3)
	for ; i < n; i += 4 {
		d := float64(math.Float32frombits(binary.LittleEndian.Uint32(b1[i:]))) - float64(math.Float32frombits(binary.LittleEndian.Uint32(b2[i:])))
		sum += d * d
	}

	return math.Sqrt(sum), nil
}
