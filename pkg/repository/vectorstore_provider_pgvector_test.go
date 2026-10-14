package repository

import (
	"math"
	"testing"
)

func TestPGVectorContentHashCompatibility(t *testing.T) {
	cases := []struct{ content, hash string }{
		{"", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"abc", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
		{"café🙂", "acef77f84ef633e211246c7fcabb48eb8eb13b49271ec9a381f598ab4620fea3"},
	}
	for _, tc := range cases {
		if got := pgVectorContentHash(tc.content); got != tc.hash {
			t.Fatalf("content SHA256 changed: got %s want %s", got, tc.hash)
		}
	}
	if pgVectorContentHash("é") == pgVectorContentHash("e\u0301") {
		t.Fatal("content hash must retain raw UTF-8 normalization distinctions")
	}
	if pgVectorContentHash("line\r\nline") == pgVectorContentHash("line\nline") {
		t.Fatal("content hash must retain line endings")
	}
}

func TestPGVectorSerializationPreservesFloat32(t *testing.T) {
	vector := []float32{0, -1.2345679, 1.9e-8, math.SmallestNonzeroFloat32, math.MaxFloat32}
	encoded, err := serializePGVector(vector)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := (&PGVectorCustomStore{}).parsePGVectorString(encoded)
	if err != nil || len(decoded) != len(vector) {
		t.Fatal("vector serialization failed", err)
	}
	for i, expected := range vector {
		if math.Float32bits(decoded[i]) != math.Float32bits(expected) {
			t.Fatalf("component %d changed: got %g want %g", i, decoded[i], expected)
		}
	}
	for _, invalid := range [][]float32{nil, {}, {float32(math.NaN())}, {float32(math.Inf(1))}, {float32(math.Inf(-1))}} {
		if _, err := serializePGVector(invalid); err == nil {
			t.Fatal("invalid pgvector component accepted")
		}
	}
}
