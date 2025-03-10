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

/*
Package common creates an outline for common functionality across the multiple
source databases we support.
While adding new methods or code here
 1. Ensure that the changes do not adversely impact any source that uses the
    common code
 2. Test cases might not sufficiently cover all cases, so integration and
    manual testing should be done ensure no functionality is breaking. Most of
    the test cases that cover the code in this package will lie in the
    implementing source databases, so it might not be required to have unit
    tests for each method here.
 3. Any functions added here should be used by two or more databases
 4. If it looks like the code is getting more complex due to refactoring,
    it is probably better off leaving the functionality out of common
*/
package common

import (
	"fmt"
	"strconv"
	"unicode"

	"github.com/cloudspannerecosystem/harbourbridge/internal"
	"github.com/cloudspannerecosystem/harbourbridge/schema"
	"github.com/cloudspannerecosystem/harbourbridge/spanner/ddl"
)

// ToDdl interface is meant to be implemented by all sources. When support for a
// new target database is added, please add a new method here with the output
// type expected. In case a particular source to target transoformation is not
// supported, an error is to be returned by the corresponding method.
type ToDdl interface {
	ToSpannerType(conv *internal.Conv, spType string, srcType schema.Type) (ddl.Type, []internal.SchemaIssue)
}

// SchemaToSpannerDDL performs schema conversion from the source DB schema to
// Spanner. It uses the source schema in conv.SrcSchema, and writes
// the Spanner schema to conv.SpSchema.
func SchemaToSpannerDDL(conv *internal.Conv, toddl ToDdl) error {
	tableIds := GetSortedTableIdsBySrcName(conv.SrcSchema)
	for _, tableId := range tableIds {
		srcTable := conv.SrcSchema[tableId]
		SchemaToSpannerDDLHelper(conv, toddl, srcTable, false)
	}
	internal.ResolveRefs(conv)
	return nil
}

func SchemaToSpannerDDLHelper(conv *internal.Conv, toddl ToDdl, srcTable schema.Table, isRestore bool) error {
	spTableName, err := internal.GetSpannerTable(conv, srcTable.Id)
	if err != nil {
		conv.Unexpected(fmt.Sprintf("Couldn't map source table %s to Spanner: %s", srcTable.Name, err))
		return err
	}
	var spColIds []string
	spColDef := make(map[string]ddl.ColumnDef)
	conv.SchemaIssues[srcTable.Id] = make(map[string][]internal.SchemaIssue)
	// Iterate over columns using ColNames order.
	for _, srcColId := range srcTable.ColIds {
		srcCol := srcTable.ColDefs[srcColId]
		colName, err := internal.GetSpannerCol(conv, srcTable.Id, srcCol.Id, spColDef)
		if err != nil {
			conv.Unexpected(fmt.Sprintf("Couldn't map source column %s of table %s to Spanner: %s", srcTable.Name, srcCol.Name, err))
			continue
		}
		spColIds = append(spColIds, srcColId)
		ty, issues := toddl.ToSpannerType(conv, "", srcCol.Type)

		// TODO(hengfeng): add issues for all elements of srcCol.Ignored.
		if srcCol.Ignored.ForeignKey {
			issues = append(issues, internal.ForeignKey)
		}
		_, isChanged := internal.FixName(srcCol.Name)
		if isChanged && (srcCol.Name != colName) {
			issues = append(issues, internal.IllegalName)
		}
		if srcCol.Ignored.Default {
			issues = append(issues, internal.DefaultValue)
		}
		if srcCol.Ignored.AutoIncrement { //TODO(adibh) - check why this is not there in postgres
			issues = append(issues, internal.AutoIncrement)
		}
		if len(issues) > 0 {
			conv.SchemaIssues[srcTable.Id][srcColId] = issues
		}
		spColDef[srcColId] = ddl.ColumnDef{
			Name:    colName,
			T:       ty,
			NotNull: srcCol.NotNull,
			Comment: "From: " + quoteIfNeeded(srcCol.Name) + " " + srcCol.Type.Print(),
			Id:      srcColId,
		}
	}
	comment := "Spanner schema for source table " + quoteIfNeeded(srcTable.Name)
	conv.SpSchema[srcTable.Id] = ddl.CreateTable{
		Name:        spTableName,
		ColIds:      spColIds,
		ColDefs:     spColDef,
		PrimaryKeys: cvtPrimaryKeys(conv, srcTable.Id, srcTable.PrimaryKeys),
		ForeignKeys: cvtForeignKeys(conv, spTableName, srcTable.Id, srcTable.ForeignKeys, isRestore),
		Indexes:     cvtIndexes(conv, srcTable.Id, srcTable.Indexes, spColIds),
		Comment:     comment,
		Id:          srcTable.Id}
	return nil
}

func quoteIfNeeded(s string) string {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsPunct(r) {
			continue
		}
		return strconv.Quote(s)
	}
	return s
}

func cvtPrimaryKeys(conv *internal.Conv, srcTableId string, srcKeys []schema.Key) []ddl.IndexKey {
	var spKeys []ddl.IndexKey
	for _, k := range srcKeys {
		spKeys = append(spKeys, ddl.IndexKey{ColId: k.ColId, Desc: k.Desc, Order: k.Order})
	}
	return spKeys
}

func cvtForeignKeys(conv *internal.Conv, spTableName string, srcTableId string, srcKeys []schema.ForeignKey, isRestore bool) []ddl.Foreignkey {
	var spKeys []ddl.Foreignkey
	for _, key := range srcKeys {
		spKey, err := CvtForeignKeysHelper(conv, spTableName, srcTableId, key, isRestore)
		if err != nil {
			continue
		}
		spKeys = append(spKeys, spKey)
	}
	return spKeys
}

func CvtForeignKeysHelper(conv *internal.Conv, spTableName string, srcTableId string, srcKey schema.ForeignKey, isRestore bool) (ddl.Foreignkey, error) {
	if len(srcKey.ColIds) != len(srcKey.ReferColumnIds) {
		conv.Unexpected(fmt.Sprintf("ConvertForeignKeys: ColIds and referColumns don't have the same lengths: len(columns)=%d, len(referColumns)=%d for source tableId: %s, referenced table: %s", len(srcKey.ColIds), len(srcKey.ReferColumnIds), srcTableId, srcKey.ReferTableId))
		return ddl.Foreignkey{}, fmt.Errorf("ConvertForeignKeys: columns and referColumns don't have the same lengths")
	}

	// check whether spanner refer table exist or not.
	_, isPresent := conv.SpSchema[srcKey.ReferTableId]
	if !isPresent && isRestore {
		return ddl.Foreignkey{}, nil
	}

	// check whether source refer table exist or not.
	_, isPresent = conv.SrcSchema[srcKey.ReferTableId]
	if !isPresent {
		conv.Unexpected(fmt.Sprintf("Can't map foreign key for source tableId: %s, referenced tableId: %s", srcTableId, srcKey.ReferTableId))
		return ddl.Foreignkey{}, fmt.Errorf("reference table not found")
	}
	var spColIds, spReferColIds []string
	for i, colId := range srcKey.ColIds {
		spColIds = append(spColIds, colId)
		spReferColIds = append(spReferColIds, srcKey.ReferColumnIds[i])
	}
	spKeyName := internal.ToSpannerForeignKey(conv, srcKey.Name)

	spKey := ddl.Foreignkey{
		Name:           spKeyName,
		ColIds:         spColIds,
		ReferTableId:   srcKey.ReferTableId,
		ReferColumnIds: spReferColIds,
		Id:             srcKey.Id,
	}
	return spKey, nil
}

func cvtIndexes(conv *internal.Conv, tableId string, srcIndexes []schema.Index, spColIds []string) []ddl.CreateIndex {
	var spIndexes []ddl.CreateIndex
	for _, srcIndex := range srcIndexes {
		spIndex := CvtIndexHelper(conv, tableId, srcIndex, spColIds)
		spIndexes = append(spIndexes, spIndex)
	}
	return spIndexes
}

func SrcTableToSpannerDDL(conv *internal.Conv, toddl ToDdl, srcTable schema.Table) error {
	err := SchemaToSpannerDDLHelper(conv, toddl, srcTable, true)
	if err != nil {
		return err
	}
	for tableId, sourceTable := range conv.SrcSchema {
		if _, isPresent := conv.SpSchema[tableId]; !isPresent {
			continue
		}
		spTable := conv.SpSchema[tableId]
		if tableId != srcTable.Id {
			spTable.ForeignKeys = cvtForeignKeysForAReferenceTable(conv, tableId, srcTable.Id, sourceTable.ForeignKeys, spTable.ForeignKeys)
			conv.SpSchema[tableId] = spTable
		}
	}
	internal.ResolveRefs(conv)
	return nil
}

func cvtForeignKeysForAReferenceTable(conv *internal.Conv, tableId string, referTableId string, srcKeys []schema.ForeignKey, spKeys []ddl.Foreignkey) []ddl.Foreignkey {
	for _, key := range srcKeys {
		if key.ReferTableId == referTableId {
			spKey, err := CvtForeignKeysHelper(conv, conv.SpSchema[tableId].Name, tableId, key, true)
			if err != nil {
				continue
			}
			spKey.Id = key.Id
			spKeys = append(spKeys, spKey)
		}
	}
	return spKeys
}

func CvtIndexHelper(conv *internal.Conv, tableId string, srcIndex schema.Index, spColIds []string) ddl.CreateIndex {
	var spKeys []ddl.IndexKey
	var spStoredColIds []string

	for _, k := range srcIndex.Keys {
		isPresent := false
		for _, v := range spColIds {
			if v == k.ColId {
				isPresent = true
				break
			}
		}
		if !isPresent {
			conv.Unexpected(fmt.Sprintf("Can't map index key column for tableId %s columnId %s", tableId, k.ColId))
			continue
		}
		spKeys = append(spKeys, ddl.IndexKey{ColId: k.ColId, Desc: k.Desc, Order: k.Order})
	}
	for _, colId := range srcIndex.StoredColumnIds {
		isPresent := false
		for _, v := range spColIds {
			if v == colId {
				isPresent = true
				break
			}
		}
		if !isPresent {
			conv.Unexpected(fmt.Sprintf("Can't map index column for tableId %s columnId %s", tableId, colId))
			continue
		}
		spStoredColIds = append(spStoredColIds, colId)
	}
	if srcIndex.Name == "" {
		// Generate a name if index name is empty in MySQL.
		// Collision of index name will be handled by ToSpannerIndexName.
		srcIndex.Name = fmt.Sprintf("Index_%s", conv.SrcSchema[tableId].Name)
	}
	spIndexName := internal.ToSpannerIndexName(conv, srcIndex.Name)
	spIndex := ddl.CreateIndex{
		Name:            spIndexName,
		TableId:         tableId,
		Unique:          srcIndex.Unique,
		Keys:            spKeys,
		StoredColumnIds: spStoredColIds,
		Id:              srcIndex.Id,
	}
	return spIndex
}
