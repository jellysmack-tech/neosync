package transformer_utils

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
)

/* FLOAT MANIPULATION UTILS */

// Generates a random float64 in the range of the min and max float64 values
func GenerateRandomFloat64WithInclusiveBounds(minValue, maxValue float64) (float64, error) {
	if minValue > maxValue {
		minValue, maxValue = maxValue, minValue
	}

	if minValue == maxValue {
		return minValue, nil
	}

	// generates a rand float64 value from [0.0,1.0)
	//nolint:all
	randValue := rand.Float64()

	// Scale and shift the value to the range
	returnValue := minValue + randValue*(maxValue-minValue)
	return returnValue, nil
}

// GetFloatLength gets the number of digits in a float64
func GetFloat64Length(i float64) int64 {
	// Convert the float64 to a string with a specific format and precision
	// Using 'g' format and a precision of -1 to automatically determine the best format
	str := strconv.FormatFloat(i, 'g', -1, 64)

	// Remove the minus sign if the number is negative
	str = strings.Replace(str, "-", "", 1)

	// Remove the decimal point
	str = strings.Replace(str, ".", "", 1)

	length := int64(len(str))

	return length
}

// Returns the float64 range between the min and max
func GetFloat64Range(minValue, maxValue float64) (float64, error) {
	if minValue > maxValue {
		return 0, fmt.Errorf("min cannot be greater than max")
	}

	if minValue == maxValue {
		return minValue, nil
	}

	return maxValue - minValue, nil
}

func IsNegativeFloat64(val float64) bool {
	if (val * -1) < 0 {
		return false
	} else {
		return true
	}
}

type FloatLength struct {
	DigitsBeforeDecimalLength int64
	DigitsAfterDecimalLength  int64
}

func GetFloatLength(i float64) *FloatLength {
	str := fmt.Sprintf("%g", i)

	parsed := strings.Split(str, ".")

	return &FloatLength{
		DigitsBeforeDecimalLength: int64(len(parsed[0])),
		DigitsAfterDecimalLength:  int64(len(parsed[1])),
	}
}

func ReduceFloat64Precision(precision int, value float64) (float64, error) {
	if precision < 1 {
		return 0, fmt.Errorf("precision cannot be less than 1")
	}

	if precision > 17 {
		return 0, fmt.Errorf("precision cannot be greater than 17")
	}

	res := strconv.FormatFloat(value, 'g', precision, 64)

	f, err := strconv.ParseFloat(res, 64)
	if err != nil {
		return 0, fmt.Errorf("unable to convert string to float64 value: %w", err)
	}
	return f, nil
}
