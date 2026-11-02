package handler

import (
	"encoding/json"
	"testing"
)

func TestSafeSlice_NilStringSlice(t *testing.T) {
	var input []string
	result := safeSlice(input)
	if result == nil {
		t.Fatal("safeSlice(nil []string) returned nil, want non-nil empty slice")
	}
	if len(result) != 0 {
		t.Errorf("safeSlice(nil []string) length = %d, want 0", len(result))
	}
}

func TestSafeSlice_NilIntSlice(t *testing.T) {
	var input []int
	result := safeSlice(input)
	if result == nil {
		t.Fatal("safeSlice(nil []int) returned nil, want non-nil empty slice")
	}
	if len(result) != 0 {
		t.Errorf("safeSlice(nil []int) length = %d, want 0", len(result))
	}
}

func TestSafeSlice_NonNilEmptySlice(t *testing.T) {
	input := []string{}
	result := safeSlice(input)
	if result == nil {
		t.Fatal("safeSlice(empty []string) returned nil, want non-nil")
	}
	if len(result) != 0 {
		t.Errorf("safeSlice(empty []string) length = %d, want 0", len(result))
	}
}

func TestSafeSlice_PopulatedSlice(t *testing.T) {
	input := []string{"a", "b", "c"}
	result := safeSlice(input)
	if len(result) != 3 {
		t.Fatalf("safeSlice(populated) length = %d, want 3", len(result))
	}
	for i, v := range input {
		if result[i] != v {
			t.Errorf("result[%d] = %q, want %q", i, result[i], v)
		}
	}
}

func TestSafeSlice_PopulatedIntSlice(t *testing.T) {
	input := []int{1, 2, 3}
	result := safeSlice(input)
	if len(result) != 3 {
		t.Fatalf("safeSlice(populated []int) length = %d, want 3", len(result))
	}
	for i, v := range input {
		if result[i] != v {
			t.Errorf("result[%d] = %d, want %d", i, result[i], v)
		}
	}
}

func TestSafeSlice_JSONEncoding_NilBecomesEmptyArray(t *testing.T) {
	var nilSlice []string
	result := safeSlice(nilSlice)

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if string(data) != "[]" {
		t.Errorf("JSON encoding of safeSlice(nil) = %s, want []", string(data))
	}
}

func TestSafeSlice_JSONEncoding_NilWouldBeNull(t *testing.T) {
	// Verify the problem safeSlice solves: nil slices encode as "null".
	var nilSlice []string
	data, err := json.Marshal(nilSlice)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if string(data) != "null" {
		t.Errorf("JSON encoding of nil slice = %s, want null (confirming the problem)", string(data))
	}

	// And safeSlice fixes it.
	fixed := safeSlice(nilSlice)
	data, err = json.Marshal(fixed)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if string(data) != "[]" {
		t.Errorf("JSON encoding of safeSlice(nil) = %s, want []", string(data))
	}
}

type structWithSlice struct {
	Items []string `json:"items"`
}

func TestSafeSlice_JSONEncoding_InStruct(t *testing.T) {
	s := structWithSlice{Items: safeSlice[string](nil)}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	expected := `{"items":[]}`
	if string(data) != expected {
		t.Errorf("JSON = %s, want %s", string(data), expected)
	}
}
