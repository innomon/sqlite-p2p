package vect

import (
	"database/sql/driver"
	"fmt"
	"sync"

	"modernc.org/sqlite"
)

var registerOnce sync.Once

func init() {
	RegisterExtension()
}

// RegisterExtension idempotently registers all pure Go SQLite vector search
// deterministic scalar functions on the modernc.org/sqlite driver:
//   - vec_cosine_similarity(v1 BLOB, v2 BLOB) -> REAL
//   - vec_cosine_distance(v1 BLOB, v2 BLOB) -> REAL
//   - vec_l2_distance(v1 BLOB, v2 BLOB) -> REAL
//   - vec_length(v BLOB) -> INTEGER
//   - vec_pack_f32(text TEXT) -> BLOB
func RegisterExtension() {
	registerOnce.Do(func() {
		sqlite.MustRegisterDeterministicScalarFunction(
			"vec_cosine_similarity",
			2,
			scalarCosineSimilarity,
		)
		sqlite.MustRegisterDeterministicScalarFunction(
			"vec_cosine_distance",
			2,
			scalarCosineDistance,
		)
		sqlite.MustRegisterDeterministicScalarFunction(
			"vec_l2_distance",
			2,
			scalarL2Distance,
		)
		sqlite.MustRegisterDeterministicScalarFunction(
			"vec_length",
			1,
			scalarVecLength,
		)
		sqlite.MustRegisterDeterministicScalarFunction(
			"vec_pack_f32",
			1,
			scalarVecPackF32,
		)
	})
}

func extractBlobArg(arg driver.Value, pos int) ([]byte, bool, error) {
	if arg == nil {
		return nil, true, nil
	}
	b, ok := arg.([]byte)
	if !ok {
		return nil, false, fmt.Errorf("argument %d must be a BLOB, got %T", pos, arg)
	}
	return b, false, nil
}

func scalarCosineSimilarity(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	b1, isNull1, err := extractBlobArg(args[0], 1)
	if err != nil {
		return nil, err
	}
	b2, isNull2, err := extractBlobArg(args[1], 2)
	if err != nil {
		return nil, err
	}
	if isNull1 || isNull2 {
		return nil, nil
	}
	return CosineSimilarityBytes(b1, b2)
}

func scalarCosineDistance(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	b1, isNull1, err := extractBlobArg(args[0], 1)
	if err != nil {
		return nil, err
	}
	b2, isNull2, err := extractBlobArg(args[1], 2)
	if err != nil {
		return nil, err
	}
	if isNull1 || isNull2 {
		return nil, nil
	}
	return CosineDistanceBytes(b1, b2)
}

func scalarL2Distance(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	b1, isNull1, err := extractBlobArg(args[0], 1)
	if err != nil {
		return nil, err
	}
	b2, isNull2, err := extractBlobArg(args[1], 2)
	if err != nil {
		return nil, err
	}
	if isNull1 || isNull2 {
		return nil, nil
	}
	return L2DistanceBytes(b1, b2)
}

func scalarVecLength(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	b, isNull, err := extractBlobArg(args[0], 1)
	if err != nil {
		return nil, err
	}
	if isNull {
		return nil, nil
	}
	dim, err := VectorLength(b)
	if err != nil {
		return nil, err
	}
	return int64(dim), nil
}

func scalarVecPackF32(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if args[0] == nil {
		return nil, nil
	}
	switch v := args[0].(type) {
	case string:
		return PackF32JSON(v)
	case []byte:
		return PackF32JSON(string(v))
	default:
		return nil, fmt.Errorf("vec_pack_f32 argument must be TEXT or BLOB, got %T", args[0])
	}
}
