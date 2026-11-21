package vect

import (
	"math"
	"testing"
)

const floatEpsilon = 1e-6

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < floatEpsilon
}

func TestFloat32SerializationRoundTrip(t *testing.T) {
	input := []float32{0.1, -0.4, 0.8, 0.0, 3.1415927, -123.456}
	b := Float32ToBytes(input)
	if len(b) != len(input)*4 {
		t.Fatalf("expected byte length %d, got %d", len(input)*4, len(b))
	}

	dim, err := VectorLength(b)
	if err != nil {
		t.Fatalf("VectorLength unexpected error: %v", err)
	}
	if dim != len(input) {
		t.Fatalf("expected dimension %d, got %d", len(input), dim)
	}

	out, err := BytesToFloat32(b)
	if err != nil {
		t.Fatalf("BytesToFloat32 unexpected error: %v", err)
	}
	if len(out) != len(input) {
		t.Fatalf("expected slice length %d, got %d", len(input), len(out))
	}
	for i := range input {
		if out[i] != input[i] {
			t.Errorf("mismatch at index %d: expected %v, got %v", i, input[i], out[i])
		}
	}
}

func TestBytesToFloat32InvalidLength(t *testing.T) {
	for _, badLen := range []int{1, 2, 3, 5, 7, 10} {
		b := make([]byte, badLen)
		if _, err := BytesToFloat32(b); err == nil {
			t.Errorf("expected error for byte slice of length %d, got nil", badLen)
		}
		if _, err := VectorLength(b); err == nil {
			t.Errorf("expected VectorLength error for byte slice of length %d, got nil", badLen)
		}
	}
}

func TestPackF32JSON(t *testing.T) {
	b, err := PackF32JSON("[0.1, -0.4, 0.8]")
	if err != nil {
		t.Fatalf("PackF32JSON unexpected error: %v", err)
	}
	floats, err := BytesToFloat32(b)
	if err != nil {
		t.Fatalf("BytesToFloat32 error: %v", err)
	}
	expected := []float32{0.1, -0.4, 0.8}
	if len(floats) != len(expected) {
		t.Fatalf("expected %d elements, got %d", len(expected), len(floats))
	}
	for i := range expected {
		if floats[i] != expected[i] {
			t.Errorf("element %d mismatch: expected %v, got %v", i, expected[i], floats[i])
		}
	}

	// Empty array
	emptyB, err := PackF32JSON("[]")
	if err != nil {
		t.Fatalf("PackF32JSON empty array error: %v", err)
	}
	if len(emptyB) != 0 {
		t.Fatalf("expected 0 bytes for empty array, got %d", len(emptyB))
	}

	// Malformed JSON
	for _, invalid := range []string{"", "not-json", "[0.1, ", "{\"a\":1}", "[1, \"two\"]"} {
		if _, err := PackF32JSON(invalid); err == nil {
			t.Errorf("expected error for invalid JSON %q, got nil", invalid)
		}
	}
}

func TestCosineSimilarityAndDistance(t *testing.T) {
	tests := []struct {
		name     string
		u        []float32
		v        []float32
		wantSim  float64
		wantDist float64
		wantErr  bool
	}{
		{
			name:     "identical vectors",
			u:        []float32{1.0, 2.0, 3.0, 4.0, 5.0},
			v:        []float32{1.0, 2.0, 3.0, 4.0, 5.0},
			wantSim:  1.0,
			wantDist: 0.0,
		},
		{
			name:     "opposite vectors",
			u:        []float32{1.0, -2.0, 3.0},
			v:        []float32{-1.0, 2.0, -3.0},
			wantSim:  -1.0,
			wantDist: 2.0,
		},
		{
			name:     "orthogonal vectors",
			u:        []float32{1.0, 0.0, 0.0, 0.0},
			v:        []float32{0.0, 1.0, 0.0, 0.0},
			wantSim:  0.0,
			wantDist: 1.0,
		},
		{
			name:     "zero magnitude u",
			u:        []float32{0.0, 0.0, 0.0},
			v:        []float32{1.0, 2.0, 3.0},
			wantSim:  0.0,
			wantDist: 1.0,
		},
		{
			name:     "zero magnitude v",
			u:        []float32{1.0, 2.0, 3.0},
			v:        []float32{0.0, 0.0, 0.0},
			wantSim:  0.0,
			wantDist: 1.0,
		},
		{
			name:     "empty vectors",
			u:        []float32{},
			v:        []float32{},
			wantSim:  0.0,
			wantDist: 1.0,
		},
		{
			name:    "dimension mismatch",
			u:       []float32{1.0, 2.0},
			v:       []float32{1.0, 2.0, 3.0},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sim, err := CosineSimilarity(tc.u, tc.v)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected CosineSimilarity error, got nil")
				}
				if _, err := CosineDistance(tc.u, tc.v); err == nil {
					t.Fatalf("expected CosineDistance error, got nil")
				}
				if _, err := CosineSimilarityBytes(Float32ToBytes(tc.u), Float32ToBytes(tc.v)); err == nil {
					t.Fatalf("expected CosineSimilarityBytes error, got nil")
				}
				if _, err := CosineDistanceBytes(Float32ToBytes(tc.u), Float32ToBytes(tc.v)); err == nil {
					t.Fatalf("expected CosineDistanceBytes error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !almostEqual(sim, tc.wantSim) {
				t.Errorf("CosineSimilarity got %v, want %v", sim, tc.wantSim)
			}

			dist, err := CosineDistance(tc.u, tc.v)
			if err != nil {
				t.Fatalf("CosineDistance unexpected error: %v", err)
			}
			if !almostEqual(dist, tc.wantDist) {
				t.Errorf("CosineDistance got %v, want %v", dist, tc.wantDist)
			}

			// Verify byte-slice fast paths match float32 slice functions
			bu := Float32ToBytes(tc.u)
			bv := Float32ToBytes(tc.v)
			simB, err := CosineSimilarityBytes(bu, bv)
			if err != nil {
				t.Fatalf("CosineSimilarityBytes unexpected error: %v", err)
			}
			if !almostEqual(simB, tc.wantSim) {
				t.Errorf("CosineSimilarityBytes got %v, want %v", simB, tc.wantSim)
			}

			distB, err := CosineDistanceBytes(bu, bv)
			if err != nil {
				t.Fatalf("CosineDistanceBytes unexpected error: %v", err)
			}
			if !almostEqual(distB, tc.wantDist) {
				t.Errorf("CosineDistanceBytes got %v, want %v", distB, tc.wantDist)
			}
		})
	}
}

func TestL2Distance(t *testing.T) {
	u := []float32{1.0, 2.0, 3.0, 4.0, 5.0}
	v := []float32{4.0, 6.0, 3.0, 4.0, 5.0} // diff = (-3, -4, 0, 0, 0) -> sqrt(9+16) = 5.0

	d, err := L2Distance(u, v)
	if err != nil {
		t.Fatalf("L2Distance unexpected error: %v", err)
	}
	if !almostEqual(d, 5.0) {
		t.Errorf("L2Distance got %v, want 5.0", d)
	}

	dBytes, err := L2DistanceBytes(Float32ToBytes(u), Float32ToBytes(v))
	if err != nil {
		t.Fatalf("L2DistanceBytes unexpected error: %v", err)
	}
	if !almostEqual(dBytes, 5.0) {
		t.Errorf("L2DistanceBytes got %v, want 5.0", dBytes)
	}

	// Dimension mismatch
	if _, err := L2Distance([]float32{1.0}, []float32{1.0, 2.0}); err == nil {
		t.Errorf("expected L2Distance dimension mismatch error")
	}
	if _, err := L2DistanceBytes(Float32ToBytes([]float32{1.0}), Float32ToBytes([]float32{1.0, 2.0})); err == nil {
		t.Errorf("expected L2DistanceBytes dimension mismatch error")
	}

	// Corrupt byte slice lengths
	if _, err := CosineSimilarityBytes([]byte{1, 2, 3}, []byte{1, 2, 3, 4}); err == nil {
		t.Errorf("expected CosineSimilarityBytes invalid byte length error")
	}
	if _, err := CosineSimilarityBytes([]byte{1, 2, 3, 4}, []byte{1, 2, 3}); err == nil {
		t.Errorf("expected CosineSimilarityBytes invalid second byte length error")
	}
	if _, err := L2DistanceBytes([]byte{1, 2, 3}, []byte{1, 2, 3, 4}); err == nil {
		t.Errorf("expected L2DistanceBytes invalid byte length error")
	}
	if _, err := L2DistanceBytes([]byte{1, 2, 3, 4}, []byte{1, 2, 3}); err == nil {
		t.Errorf("expected L2DistanceBytes invalid second byte length error")
	}
}
