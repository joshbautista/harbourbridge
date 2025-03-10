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

package table

import (
	"github.com/cloudspannerecosystem/harbourbridge/internal"
)

// reviewRenameColumn review  renaming of Columnname in schmema.
func reviewRenameColumn(newName, tableId, colId string, conv *internal.Conv, interleaveTableSchema []InterleaveTableSchema) []InterleaveTableSchema {

	sp := conv.SpSchema[tableId]

	// review column name update for interleaved child.
	isParent, childTableId := IsParent(tableId)

	if isParent {
		childColId, err := getColIdFromSpannerName(conv, childTableId, sp.ColDefs[colId].Name)
		if err == nil {
			oldColName := conv.SpSchema[childTableId].ColDefs[childColId].Name
			reviewRenameColumnNameTableSchema(conv, childTableId, childColId, newName)
			childTableName := conv.SpSchema[childTableId].Name
			colType := conv.SpSchema[childTableId].ColDefs[childColId].T.Name
			interleaveTableSchema = renameInterleaveTableSchema(interleaveTableSchema, childTableName, childColId, oldColName, newName, colType)

		}
	}

	// review column name update for interleaved parent.
	parentTableId := conv.SpSchema[tableId].ParentId

	if parentTableId != "" {
		parentColId, err := getColIdFromSpannerName(conv, parentTableId, sp.ColDefs[colId].Name)
		if err == nil {
			oldColName := conv.SpSchema[parentTableId].ColDefs[parentColId].Name
			reviewRenameColumnNameTableSchema(conv, parentTableId, parentColId, newName)
			parentTableName := conv.SpSchema[parentTableId].Name
			colType := conv.SpSchema[parentTableId].ColDefs[parentColId].T.Name
			interleaveTableSchema = renameInterleaveTableSchema(interleaveTableSchema, parentTableName, parentColId, oldColName, newName, colType)

		}
	}

	oldColName := conv.SpSchema[tableId].ColDefs[colId].Name
	colType := conv.SpSchema[tableId].ColDefs[colId].T.Name
	reviewRenameColumnNameTableSchema(conv, tableId, colId, newName)
	if childTableId != "" || parentTableId != "" {
		interleaveTableSchema = renameInterleaveTableSchema(interleaveTableSchema, sp.Name, colId, oldColName, newName, colType)
	}

	return interleaveTableSchema
}

// reviewRenameColumnNameTableSchema review  renaming of column-name in Table Schema.
func reviewRenameColumnNameTableSchema(conv *internal.Conv, tableId, colId, newName string) {
	sp := conv.SpSchema[tableId]

	column, ok := sp.ColDefs[colId]

	if ok {
		column.Name = newName

		sp.ColDefs[colId] = column
		conv.SpSchema[tableId] = sp

	}
}
