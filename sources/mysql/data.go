// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mysql

import (
	"fmt"
	"math/big"
	"math/bits"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
	"github.com/cloudspannerecosystem/harbourbridge/common/constants"
	"github.com/cloudspannerecosystem/harbourbridge/internal"
	"github.com/cloudspannerecosystem/harbourbridge/schema"
	"github.com/cloudspannerecosystem/harbourbridge/spanner/ddl"
)

// ProcessDataRow converts a row of data and writes it out to Spanner.
// srcTable and srcCols are the source table and columns respectively,
// and vals contains string data to be converted to appropriate types
// to send to Spanner. ProcessDataRow is only called in DataMode.
func ProcessDataRow(conv *internal.Conv, tableId string, colIds []string, srcSchema schema.Table, spSchema ddl.CreateTable, vals []string) {
	srcTableName := srcSchema.Name
	srcCols := []string{}
	for _, colId := range colIds {
		srcCols = append(srcCols, srcSchema.ColDefs[colId].Name)
	}
	spTableName, cvtCols, cvtVals, err := ConvertData(conv, tableId, colIds, srcSchema, spSchema, vals)
	if err != nil {
		conv.Unexpected(fmt.Sprintf("Error while converting data: %s\n", err))
		conv.StatsAddBadRow(srcTableName, conv.DataMode())
		conv.CollectBadRow(srcTableName, srcCols, vals)
	} else {
		conv.WriteRow(srcTableName, spTableName, cvtCols, cvtVals)
	}
}

// ConvertData maps the source DB data in vals into Spanner data,
// based on the Spanner and source DB schemas. Note that since entries
// in vals may be empty, we also return the list of columns (empty
// cols are dropped).
func ConvertData(conv *internal.Conv, tableId string, colIds []string, srcSchema schema.Table, spSchema ddl.CreateTable, vals []string) (string, []string, []interface{}, error) {
	var c []string
	var v []interface{}
	if len(colIds) != len(vals) {
		return "", []string{}, []interface{}{}, fmt.Errorf("ConvertData: colIds and vals don't all have the same lengths: len(colIds)=%d, len(vals)=%d", len(colIds), len(vals))
	}
	for i, colId := range colIds {
		// Skip columns with 'NULL' values. When processing data rows from mysqldump, these values
		// are represented as nil (by pingcap/tidb/types/parser_driver's ValueExpr), which is
		// converted to the string '<nil>'. When processing data rows obtained from the MySQL driver,
		// 'NULL' values are represented as "NULL" (because we retrieve the values as strings).
		if vals[i] == "<nil>" || vals[i] == "NULL" {
			continue
		}

		spColDef, ok1 := spSchema.ColDefs[colId]
		srcColDef, ok2 := srcSchema.ColDefs[colId]
		if !ok1 || !ok2 {
			return "", []string{}, []interface{}{}, fmt.Errorf("can't find Spanner and source-db schema for colId %s", colId)
		}
		spCol := spColDef.Name

		var x interface{}
		var err error
		if spColDef.T.IsArray {
			x, err = convArray(spColDef.T, srcColDef.Type.Name, vals[i])
		} else {
			x, err = convScalar(conv, spColDef.T, srcColDef.Type.Name, conv.TimezoneOffset, vals[i])
		}
		if err != nil {
			return "", []string{}, []interface{}{}, err
		}
		v = append(v, x)
		c = append(c, spCol)
	}
	if aux, ok := conv.SyntheticPKeys[tableId]; ok {
		c = append(c, conv.SpSchema[tableId].ColDefs[aux.ColId].Name)
		v = append(v, fmt.Sprintf("%d", int64(bits.Reverse64(uint64(aux.Sequence)))))
		aux.Sequence++
		conv.SyntheticPKeys[tableId] = aux
	}
	return conv.SpSchema[tableId].Name, c, v, nil
}

// convScalar converts a source database string value to an
// appropriate Spanner value. It is the caller's responsibility to
// detect and handle NULL values: convScalar will return error if a
// NULL value is passed.
func convScalar(conv *internal.Conv, spannerType ddl.Type, srcTypeName string, TimezoneOffset string, val string) (interface{}, error) {
	// Whitespace within the val string is considered part of the data value.
	// Note that many of the underlying conversions functions we use (like
	// strconv.ParseFloat and strconv.ParseInt) return "invalid syntax"
	// errors if whitespace were to appear at the start or end of a string.
	// We do not expect mysqldump to generate such output.
	switch spannerType.Name {
	case ddl.Bool:
		return convBool(conv, val)
	case ddl.Bytes:
		return convBytes(val)
	case ddl.Date:
		return convDate(val)
	case ddl.Float64:
		return convFloat64(val)
	case ddl.Int64:
		return convInt64(val)
	case ddl.Numeric:
		return convNumeric(conv, val)
	case ddl.String:
		return val, nil
	case ddl.Timestamp:
		return convTimestamp(srcTypeName, TimezoneOffset, val)
	case ddl.JSON:
		return val, nil
	default:
		return val, fmt.Errorf("data conversion not implemented for type %v", spannerType.Name)
	}
}

func convBool(conv *internal.Conv, val string) (bool, error) {
	b, err := strconv.ParseBool(val)
	if err != nil {
		// MySQL uses TINYINT(1) to implement BOOL/BOOLEAN, and does not
		// enforce/validate boolean values i.e. any value that can be stored
		// in a TINYINT (-128 to 127) can be stored in BOOL/BOOLEAN.
		// If ParseBool(val) fails, this is very likely the cause.
		// To handle this, re-parse as INT64 and treat as true if value is non-zero.
		// Note: if ParseBool(val) fails, then val is probably a non-zero number.
		i, err2 := convInt64(val)
		if err2 == nil && i >= -128 && i <= 127 {
			b = i != 0
			conv.Unexpected(fmt.Sprintf("Expected boolean value, but found integer value %v; mapping it to %v\n", val, b))
			return b, err2
		}
		return b, fmt.Errorf("can't convert to bool: %w", err)
	}
	return b, err
}

func convBytes(val string) ([]byte, error) {
	// convert a string to a byte slice.
	b := []byte(val)
	return b, nil
}

func convDate(val string) (civil.Date, error) {
	d, err := civil.ParseDate(val)
	if err != nil {
		return d, fmt.Errorf("can't convert to date: %w", err)
	}
	return d, err
}

func convFloat64(val string) (float64, error) {
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return f, fmt.Errorf("can't convert to float64: %w", err)
	}
	return f, err
}

func convInt64(val string) (int64, error) {
	i, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return i, fmt.Errorf("can't convert to int64: %w", err)
	}
	return i, err
}

// convNumeric maps a source database string value (representing a numeric)
// into a string representing a valid Spanner numeric.
func convNumeric(conv *internal.Conv, val string) (interface{}, error) {
	if conv.SpDialect == constants.DIALECT_POSTGRESQL {
		return spanner.PGNumeric{Numeric: val, Valid: true}, nil
	} else {
		r := new(big.Rat)
		if _, ok := r.SetString(val); !ok {
			return "", fmt.Errorf("can't convert %q to big.Rat", val)
		}
		return r, nil
	}
}

// convTimestamp maps a source DB timestamp into a go Time Spanner timestamp
// It handles both datetime and timestamp conversions.
func convTimestamp(srcTypeName string, TimezoneOffset string, val string) (t time.Time, err error) {
	// mysqldump outputs timestamps as ISO 8601, except
	// it uses space instead of T.
	if srcTypeName == "timestamp" {
		// We consider timezone for timestamp datatype.
		// If timezone is not specified in mysqldump, we consider UTC time.
		if TimezoneOffset == "" {
			TimezoneOffset = "+00:00"
		}
		// convert timestamp from format "2006-01-02 15:04:05" to
		// "2006-01-02T15:04:05+00:00".
		timeNew := strings.Split(val, " ")
		timeJoined := strings.Join(timeNew, "T")
		timeJoined = timeJoined + TimezoneOffset
		t, err = time.Parse(time.RFC3339, timeJoined)
	} else {
		// datetime: data should just consist of date and time.
		// timestamp conversion should ignore timezone. We mimic this using Parse
		// i.e. treat it as UTC, so it will be stored 'as-is' in Spanner.
		t, err = time.Parse("2006-01-02 15:04:05", val)
	}
	if err != nil {
		return t, fmt.Errorf("can't convert to timestamp (mysql type: %s)", srcTypeName)
	}
	return t, err
}

// convArray converts a source database string value (representing an
// array) to an appropriate Spanner array value. It is the caller's
// responsibility to detect and handle the case where the entire array
// is NULL. However, convArray does handle the case where individual
// array elements are NULL. In other words, convArray handles "{1,
// NULL, 2}", but it does not handle "NULL" (it returns error).
// NOTE : convArray would only be called when MySQL 'SET' datatype is encountered.
func convArray(spannerType ddl.Type, srcTypeName string, v string) (interface{}, error) {
	v = strings.TrimSpace(v)
	// Handle empty array. Note that we use an empty NullString array
	// for all Spanner array types since this will be converted to the
	// appropriate type by the Spanner client.
	if v == "" {
		return []spanner.NullString{}, nil
	}

	a := strings.Split(v, ",")

	// The Spanner client for go does not accept []interface{} for arrays.
	// Instead it only accepts slices of a specific type eg: []string
	// Hence we have to do the following case analysis.
	// NOTE: MySQL only supports SET of string which will be translated
	// to spanner array<string>.
	switch spannerType.Name {
	case ddl.String:
		var r []spanner.NullString
		for _, s := range a {
			if s == "NULL" {
				r = append(r, spanner.NullString{Valid: false})
				continue
			}
			s, err := processQuote(s)
			if err != nil {
				return []spanner.NullString{}, err
			}
			r = append(r, spanner.NullString{StringVal: s, Valid: true})
		}
		return r, nil
	}
	return []interface{}{}, fmt.Errorf("array type conversion not implemented for type %v", spannerType.Name)
}

// processQuote returns the unquoted version of s.
// Note: The element values of a MySQL array ('SET' datatype) may have double
// quotes around them. The array output routine will put double
// quotes around element values if they are empty strings, contain
// curly braces, delimiter characters, double quotes, backslashes, or
// white space, or match the word NULL. Double quotes and backslashes
// embedded in element values will be backslash-escaped.
func processQuote(s string) (string, error) {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return strconv.Unquote(s)
	}
	return s, nil
}
