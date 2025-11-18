package main

import (
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
)

func main() {
	// Test the volume conversion fix
	val := 155601234.0
	valStr := strconv.FormatFloat(val, 'f', 10, 64)
	fmt.Printf("Value: %f\n", val)
	fmt.Printf("String: %s\n", valStr)

	var num pgtype.Numeric
	err := num.Scan(valStr)
	fmt.Printf("Scan error: %v\n", err)

	// Test with scientific notation (should fail)
	valStrSci := strconv.FormatFloat(val, 'g', -1, 64)
	fmt.Printf("Scientific notation: %s\n", valStrSci)
	err2 := num.Scan(valStrSci)
	fmt.Printf("Scientific scan error: %v\n", err2)
}
