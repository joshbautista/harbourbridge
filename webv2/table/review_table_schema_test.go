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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudspannerecosystem/harbourbridge/common/constants"
	"github.com/cloudspannerecosystem/harbourbridge/internal"
	"github.com/cloudspannerecosystem/harbourbridge/proto/migration"
	"github.com/cloudspannerecosystem/harbourbridge/schema"
	"github.com/cloudspannerecosystem/harbourbridge/spanner/ddl"
	"github.com/cloudspannerecosystem/harbourbridge/webv2/session"
	"github.com/stretchr/testify/assert"
)

func TestReviewTableSchema(t *testing.T) {

	tc := []struct {
		name         string
		tableId      string
		payload      string
		statusCode   int64
		conv         *internal.Conv
		expectedConv *internal.Conv
	}{
		{
			name:    "Test change type success",
			tableId: "t1",
			payload: `
		{
		  "UpdateCols":{
			"c1": { "ToType": "STRING" },
			"c2": { "ToType": "BYTES" }
		}
		}`,
			statusCode: http.StatusOK,
			conv: &internal.Conv{
				SpSchema: map[string]ddl.CreateTable{
					"t1": {
						Name:   "t1",
						ColIds: []string{"c1", "c2"},
						ColDefs: map[string]ddl.ColumnDef{
							"c1": {Name: "a", Id: "c1", T: ddl.Type{Name: ddl.Int64}},
							"c2": {Name: "b", Id: "c2", T: ddl.Type{Name: ddl.String, Len: 6}},
						},
						PrimaryKeys: []ddl.IndexKey{{ColId: "c1"}},
					}},
				SrcSchema: map[string]schema.Table{
					"t1": {
						Name:   "t1",
						ColIds: []string{"c1", "c2"},
						ColDefs: map[string]schema.Column{
							"c1": {Name: "a", Id: "c1", Type: schema.Type{Name: "bigint", Mods: []int64{}}},
							"c2": {Name: "b", Id: "c2", Type: schema.Type{Name: "varchar", Mods: []int64{6}}},
						},
						PrimaryKeys: []schema.Key{{ColId: "c1"}},
					}},
				SchemaIssues: map[string]map[string][]internal.SchemaIssue{
					"t1": {},
				},
				Audit: internal.Audit{
					MigrationType: migration.MigrationData_MIGRATION_TYPE_UNSPECIFIED.Enum(),
				},
			},
			expectedConv: &internal.Conv{
				SpSchema: map[string]ddl.CreateTable{
					"t1": {
						Name:   "t1",
						ColIds: []string{"c1", "c2"},
						ColDefs: map[string]ddl.ColumnDef{
							"c1": {Name: "a", Id: "c1", T: ddl.Type{Name: ddl.String, Len: ddl.MaxLength}},
							"c2": {Name: "b", Id: "c2", T: ddl.Type{Name: ddl.Bytes, Len: 6}},
						},
						PrimaryKeys: []ddl.IndexKey{{ColId: "c1"}},
					}},
				SrcSchema: map[string]schema.Table{
					"t1": {
						Name:   "t1",
						ColIds: []string{"c1", "c2"},
						ColDefs: map[string]schema.Column{
							"c1": {Name: "a", Id: "c1", Type: schema.Type{Name: "bigint", Mods: []int64{}}},
							"c2": {Name: "b", Id: "c2", Type: schema.Type{Name: "varchar", Mods: []int64{6}}},
						},
						PrimaryKeys: []schema.Key{{ColId: "c1"}},
					}},
				SchemaIssues: map[string]map[string][]internal.SchemaIssue{
					"t1": {
						"c1": {internal.Widened},
					},
				},
				Audit: internal.Audit{
					MigrationType: migration.MigrationData_MIGRATION_TYPE_UNSPECIFIED.Enum(),
				},
			},
		},
		{
			name:    "Test Add success",
			tableId: "t1",
			payload: `
		{
		  "UpdateCols":{
			"c3": { "Add": true, "ToType": "STRING"}
		}
		}`,
			statusCode: http.StatusOK,
			conv: &internal.Conv{
				SpSchema: map[string]ddl.CreateTable{
					"t1": {
						Name:   "t1",
						Id:     "t1",
						ColIds: []string{"c1", "c2"},
						ColDefs: map[string]ddl.ColumnDef{
							"c1": {Id: "c1", Name: "a", T: ddl.Type{Name: ddl.String, Len: ddl.MaxLength}},
							"c2": {Id: "c2", Name: "b", T: ddl.Type{Name: ddl.String, Len: ddl.MaxLength}},
						},
						PrimaryKeys: []ddl.IndexKey{{ColId: "c1"}},
					}},
				SrcSchema: map[string]schema.Table{
					"t1": {
						Id:     "t1",
						Name:   "t1",
						ColIds: []string{"c1", "c2", "c3"},
						ColDefs: map[string]schema.Column{
							"c1": {Id: "c1", Name: "a", Type: schema.Type{Name: "varchar", Mods: []int64{6}}},
							"c2": {Id: "c2", Name: "b", Type: schema.Type{Name: "varchar", Mods: []int64{6}}},
							"c3": {Id: "c3", Name: "c", Type: schema.Type{Name: "varchar", Mods: []int64{}}},
						},
						PrimaryKeys: []schema.Key{{ColId: "c1"}},
					}},

				Audit: internal.Audit{
					MigrationType: migration.MigrationData_MIGRATION_TYPE_UNSPECIFIED.Enum(),
				},
			},
			expectedConv: &internal.Conv{
				SpSchema: map[string]ddl.CreateTable{
					"t1": {
						Id:     "t1",
						Name:   "t1",
						ColIds: []string{"c1", "c2", "c3"},
						ColDefs: map[string]ddl.ColumnDef{
							"c1": {Id: "c1", Name: "a", T: ddl.Type{Name: ddl.String, Len: ddl.MaxLength}},
							"c2": {Id: "c2", Name: "b", T: ddl.Type{Name: ddl.String, Len: ddl.MaxLength}},
							"c3": {Id: "c3", Name: "c", T: ddl.Type{Name: ddl.String, Len: ddl.MaxLength}},
						},
						PrimaryKeys: []ddl.IndexKey{{ColId: "c1"}},
					}},
				SrcSchema: map[string]schema.Table{
					"t1": {
						Id:     "t1",
						Name:   "t1",
						ColIds: []string{"c1", "c2", "c3"},
						ColDefs: map[string]schema.Column{
							"c1": {Id: "c1", Name: "a", Type: schema.Type{Name: "varchar", Mods: []int64{6}}},
							"c2": {Id: "c2", Name: "b", Type: schema.Type{Name: "varchar", Mods: []int64{6}}},
							"c3": {Id: "c3", Name: "c", Type: schema.Type{Name: "varchar", Mods: []int64{}}},
						},
						PrimaryKeys: []schema.Key{{ColId: "c1"}},
					}},

				Audit: internal.Audit{
					MigrationType: migration.MigrationData_MIGRATION_TYPE_UNSPECIFIED.Enum(),
				},
			},
		},
		{
			name:    "Test remove success",
			tableId: "t1",
			payload: `
		{
		  "UpdateCols":{
			"c3": { "Removed": true }
		}
		}`,
			statusCode: http.StatusOK,
			conv: &internal.Conv{
				SpSchema: map[string]ddl.CreateTable{
					"t1": {
						Name:   "t1",
						ColIds: []string{"c1", "c2", "c3"},
						ColDefs: map[string]ddl.ColumnDef{
							"c1": {Name: "a", Id: "c1", T: ddl.Type{Name: ddl.String, Len: ddl.MaxLength}},
							"c2": {Name: "b", Id: "c2", T: ddl.Type{Name: ddl.String, Len: ddl.MaxLength}},
							"c3": {Name: "c", Id: "c3", T: ddl.Type{Name: ddl.Int64}},
						},
						PrimaryKeys: []ddl.IndexKey{{ColId: "c1"}},
					}},
				SchemaIssues: map[string]map[string][]internal.SchemaIssue{
					"t1": {
						"c3": {internal.Widened},
					},
				},
				Audit: internal.Audit{
					MigrationType: migration.MigrationData_MIGRATION_TYPE_UNSPECIFIED.Enum(),
				},
			},
			expectedConv: &internal.Conv{
				SpSchema: map[string]ddl.CreateTable{
					"t1": {
						Name:   "t1",
						ColIds: []string{"c1", "c2"},
						ColDefs: map[string]ddl.ColumnDef{
							"c1": {Name: "a", Id: "c1", T: ddl.Type{Name: ddl.String, Len: ddl.MaxLength}},
							"c2": {Name: "b", Id: "c2", T: ddl.Type{Name: ddl.String, Len: ddl.MaxLength}},
						},
						PrimaryKeys: []ddl.IndexKey{{ColId: "c1"}},
					}},
				SchemaIssues: map[string]map[string][]internal.SchemaIssue{
					"t1": {},
				},
				Audit: internal.Audit{
					MigrationType: migration.MigrationData_MIGRATION_TYPE_UNSPECIFIED.Enum(),
				},
			},
		},

		{
			name:    "Test rename success",
			tableId: "t1",
			payload: `
		{
		  "UpdateCols":{
			"c1": { "Rename": "aa" }
		}
		}`,
			statusCode: http.StatusOK,
			conv: &internal.Conv{
				SpSchema: map[string]ddl.CreateTable{
					"t1": {
						Name:   "t1",
						ColIds: []string{"c1", "c2", "c3"},
						ColDefs: map[string]ddl.ColumnDef{
							"c1": {Name: "a", Id: "c1", T: ddl.Type{Name: ddl.String, Len: ddl.MaxLength}},
							"c2": {Name: "b", Id: "c2", T: ddl.Type{Name: ddl.String, Len: ddl.MaxLength}},
							"c3": {Name: "c", Id: "c3", T: ddl.Type{Name: ddl.Int64}},
						},
						PrimaryKeys: []ddl.IndexKey{{ColId: "c1"}},
					}},
				Audit: internal.Audit{
					MigrationType: migration.MigrationData_MIGRATION_TYPE_UNSPECIFIED.Enum(),
				},
			},
			expectedConv: &internal.Conv{
				SpSchema: map[string]ddl.CreateTable{
					"t1": {
						Name:   "t1",
						ColIds: []string{"c1", "c2", "c3"},
						ColDefs: map[string]ddl.ColumnDef{
							"c1": {Name: "aa", Id: "c1", T: ddl.Type{Name: ddl.String, Len: ddl.MaxLength}},
							"c2": {Name: "b", Id: "c2", T: ddl.Type{Name: ddl.String, Len: ddl.MaxLength}},
							"c3": {Name: "c", Id: "c3", T: ddl.Type{Name: ddl.Int64}},
						},
						PrimaryKeys: []ddl.IndexKey{{ColId: "c1"}},
					}},
				Audit: internal.Audit{
					MigrationType: migration.MigrationData_MIGRATION_TYPE_UNSPECIFIED.Enum(),
				},
			},
		},
		{
			name:    "Test rename success for interleaved table",
			tableId: "t1",
			payload: `
		{
		  "UpdateCols":{
			"c1": { "Rename": "aa" }
		}
		}`,
			statusCode: http.StatusOK,
			conv: &internal.Conv{
				SpSchema: map[string]ddl.CreateTable{
					"t1": {
						Name:   "t1",
						ColIds: []string{"c1", "c2", "c3"},
						ColDefs: map[string]ddl.ColumnDef{
							"c1": {Name: "a", Id: "c1", T: ddl.Type{Name: ddl.Int64}, NotNull: true},
							"c2": {Name: "b", Id: "c2", T: ddl.Type{Name: ddl.Int64}, NotNull: true},
							"c3": {Name: "c", Id: "c3", T: ddl.Type{Name: ddl.String, Len: ddl.MaxLength}, NotNull: true},
						},
						PrimaryKeys: []ddl.IndexKey{{ColId: "c1", Desc: false}, {ColId: "c2", Desc: false}},
						ParentId:    "t2",
					},
					"t2": {
						Name:   "t2",
						ColIds: []string{"c1", "2", "c3"},
						ColDefs: map[string]ddl.ColumnDef{
							"c1": {Name: "a", Id: "c1", T: ddl.Type{Name: ddl.Int64}, NotNull: true},
							"c2": {Name: "b", Id: "c2", T: ddl.Type{Name: ddl.Int64}, NotNull: true},
							"c3": {Name: "c", Id: "c3", T: ddl.Type{Name: ddl.String, Len: ddl.MaxLength}, NotNull: true},
						},
						PrimaryKeys: []ddl.IndexKey{{ColId: "c1", Desc: false}},
					},
				},
			},
			expectedConv: &internal.Conv{
				SpSchema: map[string]ddl.CreateTable{
					"t1": {
						Name:   "t1",
						ColIds: []string{"c1", "c2", "c3"},
						ColDefs: map[string]ddl.ColumnDef{
							"c1": {Name: "aa", Id: "c1", T: ddl.Type{Name: ddl.Int64}, NotNull: true},
							"c2": {Name: "b", Id: "c2", T: ddl.Type{Name: ddl.Int64}, NotNull: true},
							"c3": {Name: "c", Id: "c3", T: ddl.Type{Name: ddl.String, Len: ddl.MaxLength}, NotNull: true},
						},
						PrimaryKeys: []ddl.IndexKey{{ColId: "c1", Desc: false}, {ColId: "c2", Desc: false}},
						ParentId:    "t2",
					},
					"t2": {
						Name:   "t2",
						ColIds: []string{"c1", "c2", "c3"},
						ColDefs: map[string]ddl.ColumnDef{
							"c1": {Name: "aa", Id: "c1", T: ddl.Type{Name: ddl.Int64}, NotNull: true},
							"c2": {Name: "b", Id: "c2", T: ddl.Type{Name: ddl.Int64}, NotNull: true},
							"c3": {Name: "c", Id: "c3", T: ddl.Type{Name: ddl.String, Len: ddl.MaxLength}, NotNull: true},
						},
						PrimaryKeys: []ddl.IndexKey{{ColId: "c1", Desc: false}},
					},
				},
				SpDialect: constants.DIALECT_GOOGLESQL,
			},
		},
	}

	for _, tc := range tc {

		sessionState := session.GetSessionState()
		sessionState.Conv = tc.conv
		sessionState.Driver = constants.MYSQL

		payload := tc.payload

		req, err := http.NewRequest("POST", "/typemap/reviewTableSchema?table="+tc.tableId, strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}

		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()

		handler := http.HandlerFunc(ReviewTableSchema)

		handler.ServeHTTP(rr, req)

		res := ReviewTableSchemaResponse{}

		json.Unmarshal(rr.Body.Bytes(), &res)

		if status := rr.Code; int64(status) != tc.statusCode {
			t.Errorf("handler returned wrong status code: got %v want %v",
				status, tc.statusCode)
		}

		expectedddl := GetSpannerTableDDL(tc.expectedConv.SpSchema[tc.tableId], tc.expectedConv.SpDialect)

		if tc.statusCode == http.StatusOK {
			assert.Equal(t, expectedddl, res.DDL)
		}
	}
}
