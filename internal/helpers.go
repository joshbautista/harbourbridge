// Copyright 2022 Google LLC
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

package internal

import (
	"fmt"
	"strconv"

	"github.com/cloudspannerecosystem/harbourbridge/schema"
	"github.com/cloudspannerecosystem/harbourbridge/spanner/ddl"
)

type Counter struct {
	ObjectId string
}

var Cntr Counter

// Contains check string present in list.
func Contains(l []string, str string) bool {
	for _, s := range l {
		if s == str {
			return true
		}
	}
	return false
}

func GenerateIdSuffix() string {

	counter, _ := strconv.Atoi(Cntr.ObjectId)

	counter = counter + 1

	Cntr.ObjectId = strconv.Itoa(counter)
	return Cntr.ObjectId
}

func GenerateId(idPrefix string) string {
	idSuffix := GenerateIdSuffix()
	id := idPrefix + idSuffix
	return id
}

func GenerateTableId() string {
	return GenerateId("t")
}

func GenerateColumnId() string {
	return GenerateId("c")
}

func GenerateForeignkeyId() string {
	return GenerateId("f")
}

func GenerateIndexesId() string {
	return GenerateId("i")
}
func GenerateRuleId() string {
	return GenerateId("r")
}

func GetSrcColNameIdMap(srcs schema.Table) map[string]string {
	if len(srcs.ColNameIdMap) > 0 {
		return srcs.ColNameIdMap
	}
	m := make(map[string]string)
	for _, v := range srcs.ColDefs {
		m[v.Name] = v.Id
	}
	return m
}

func GetColIdFromSrcName(srcColDef map[string]schema.Column, columnName string) (string, error) {
	for _, v := range srcColDef {
		if v.Name == columnName {
			return v.Id, nil
		}
	}
	return "", fmt.Errorf("column id not found for source-db column %s", columnName)
}
func GetTableIdFromSrcName(srcSchema map[string]schema.Table, tableName string) (string, error) {
	for _, v := range srcSchema {
		if v.Name == tableName {
			return v.Id, nil
		}
	}
	return "", fmt.Errorf("table id not found for source-db table %s", tableName)
}

func GetTableIdFromSpName(spSchema ddl.Schema, tableName string) (string, error) {
	for tableId, table := range spSchema {
		if tableName == table.Name {
			return tableId, nil
		}
	}
	return "", fmt.Errorf("table id not found for spanner table %s", tableName)
}

func GetColIdFromSpName(colDefs map[string]ddl.ColumnDef, colName string) (string, error) {
	for colId, col := range colDefs {
		if col.Name == colName {
			return colId, nil
		}
	}
	return "", fmt.Errorf("column id not found for spanner column %s", colName)
}

func GetSrcFkFromId(fks []schema.ForeignKey, fkId string) (schema.ForeignKey, error) {
	for _, v := range fks {
		if v.Id == fkId {
			return v, nil
		}
	}
	return schema.ForeignKey{}, fmt.Errorf("foreign key not found")
}

func GetSrcIndexFromId(indexes []schema.Index, indexId string) (schema.Index, error) {
	for _, v := range indexes {
		if v.Id == indexId {
			return v, nil
		}
	}
	return schema.Index{}, fmt.Errorf("index not found")
}

func ComputeUsedNames(conv *Conv) map[string]bool {
	usedNames := make(map[string]bool)
	for _, table := range conv.SpSchema {
		usedNames[table.Name] = true
		for _, index := range table.Indexes {
			usedNames[index.Name] = true
		}
		for _, fk := range table.ForeignKeys {
			usedNames[fk.Name] = true
		}
	}
	return usedNames
}

func ComputeToSource(conv *Conv) map[string]NameAndCols {
	toSource := make(map[string]NameAndCols)
	for id, spTable := range conv.SpSchema {
		if srcTable, ok := conv.SrcSchema[id]; ok {
			colMap := make(map[string]string)
			for colId, spCol := range spTable.ColDefs {
				if srcCol, ok := srcTable.ColDefs[colId]; ok {
					colMap[spCol.Name] = srcCol.Name
				}
			}
			toSource[spTable.Name] = NameAndCols{Name: srcTable.Name, Cols: colMap}
		}
	}
	return toSource
}

func ComputeToSpanner(conv *Conv) map[string]NameAndCols {
	toSpanner := make(map[string]NameAndCols)
	for id, srcTable := range conv.SrcSchema {
		if spTable, ok := conv.SpSchema[id]; ok {
			colMap := make(map[string]string)
			for colId, srcCol := range srcTable.ColDefs {
				if spCol, ok := spTable.ColDefs[colId]; ok {
					colMap[srcCol.Name] = spCol.Name
				}
			}
			toSpanner[srcTable.Name] = NameAndCols{Name: spTable.Name, Cols: colMap}
		}
	}
	return toSpanner
}

func GetSrcTableByName(srcSchema map[string]schema.Table, name string) (*schema.Table, bool) {
	for _, v := range srcSchema {
		if v.Name == name {
			return &v, true
		}
	}
	return nil, false
}

func ResolveForeignKeyIds(schema map[string]schema.Table) {
	for key, tbl := range schema {
		for idx, fk := range tbl.ForeignKeys {
			colIds := []string{}
			for _, cn := range fk.ColumnNames {
				colIds = append(colIds, tbl.ColNameIdMap[cn])
			}
			schema[key].ForeignKeys[idx].ColIds = colIds
			// ColumnNames is used only to fetch the Ids. The collection is not maintained in the application
			schema[key].ForeignKeys[idx].ColumnNames = nil

			if refTbl, ok := GetSrcTableByName(schema, fk.ReferTableName); ok {
				refColIds := []string{}
				for _, cn := range fk.ReferColumnNames {
					refColIds = append(refColIds, refTbl.ColNameIdMap[cn])
				}
				schema[key].ForeignKeys[idx].ReferTableId = refTbl.Id
				// ReferTableName is used only to fetch the Id. This field is not maintained
				schema[key].ForeignKeys[idx].ReferTableName = ""

				schema[key].ForeignKeys[idx].ReferColumnIds = refColIds
				// ReferColumnNames is used only to fetch the Id. This collection is not maintained
				schema[key].ForeignKeys[idx].ReferColumnNames = nil

			}
		}
	}
}
