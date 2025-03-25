package neuprintneo4j

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
)

// TestJsonNumberConversion tests direct conversion of json.Number to int64
func TestJsonNumberConversion(t *testing.T) {
	// 2^55 + 1 = 36028797018963969 (exceeds JavaScript's safe integer range of 2^53-1)
	largeInt := int64(36028797018963969)
	
	// Create a json.Number from our large integer
	jsonNumber := json.Number(fmt.Sprintf("%d", largeInt))
	
	// Verify we can correctly convert it to int64
	intValue, err := jsonNumber.Int64()
	if err != nil {
		t.Fatalf("Failed to convert json.Number to int64: %v", err)
	}
	
	if intValue != largeInt {
		t.Errorf("Expected %d, got %d", largeInt, intValue)
	}
	
	// Demonstrate precision loss when using float64
	floatValue := float64(largeInt)
	convertedBack := int64(floatValue)
	
	if convertedBack == largeInt {
		t.Errorf("Expected precision loss but got matching values: %d", convertedBack)
	}
	
	// Calculate and print the difference
	diff := largeInt - convertedBack
	t.Logf("Precision loss: %d", diff)
}

// TestJsonDecodeNumber tests that json.Decoder.UseNumber() preserves integer precision
func TestJsonDecodeNumber(t *testing.T) {
	// 2^55 + 1 = 36028797018963969 (exceeds JavaScript's safe integer range of 2^53-1)
	largeInt := int64(36028797018963969)
	
	// Create a JSON string containing our large integer
	jsonStr := fmt.Sprintf(`{"value": %d}`, largeInt)
	
	// 1. First, test standard json.Unmarshal behavior (will use float64)
	var resultUnmarshal struct {
		Value interface{} `json:"value"`
	}
	
	err := json.Unmarshal([]byte(jsonStr), &resultUnmarshal)
	if err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	
	// By default, json.Unmarshal should use float64 for numbers
	_, isFloat := resultUnmarshal.Value.(float64)
	if !isFloat {
		t.Errorf("Expected float64 from Unmarshal, got %T", resultUnmarshal.Value)
	}
	
	// 2. Then test json.Decoder with UseNumber()
	var resultDecoder struct {
		Value interface{} `json:"value"`
	}
	
	decoder := json.NewDecoder(bytes.NewReader([]byte(jsonStr)))
	decoder.UseNumber() // This ensures numbers are stored as json.Number
	
	err = decoder.Decode(&resultDecoder)
	if err != nil {
		t.Fatalf("json.Decoder failed: %v", err)
	}
	
	// With UseNumber(), this should be json.Number
	jsonNum, isJsonNumber := resultDecoder.Value.(json.Number)
	if !isJsonNumber {
		t.Fatalf("Expected json.Number from Decoder with UseNumber, got %T", resultDecoder.Value)
	}
	
	// Convert json.Number to int64
	intValue, err := jsonNum.Int64()
	if err != nil {
		t.Fatalf("Failed to convert json.Number to int64: %v", err)
	}
	
	// Verify we get the exact integer
	if intValue != largeInt {
		t.Errorf("Expected %d, got %d", largeInt, intValue)
	}
}