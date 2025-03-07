package sqlmanager_mssql

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/doug-martin/goqu/v9"
	mysql_queries "github.com/nucleuscloud/neosync/backend/gen/go/db/dbschemas/mysql"
	"github.com/nucleuscloud/neosync/backend/internal/nucleusdb"
	mssql_queries "github.com/nucleuscloud/neosync/backend/pkg/mssql-querier"
	sqlmanager_shared "github.com/nucleuscloud/neosync/backend/pkg/sqlmanager/shared"
)

type Manager struct {
	querier mssql_queries.Querier
	db      mysql_queries.DBTX
	close   func()
}

func NewManager(querier mssql_queries.Querier, db mysql_queries.DBTX, closer func()) *Manager {
	return &Manager{querier: querier, db: db, close: closer}
}

const defaultIdentity string = "IDENTITY(1,1)"

func (m *Manager) GetDatabaseSchema(ctx context.Context) ([]*sqlmanager_shared.DatabaseSchemaRow, error) {
	dbSchemas, err := m.querier.GetDatabaseSchema(ctx, m.db)
	if err != nil && !nucleusdb.IsNoRows(err) {
		return nil, err
	} else if err != nil && nucleusdb.IsNoRows(err) {
		return []*sqlmanager_shared.DatabaseSchemaRow{}, nil
	}

	output := []*sqlmanager_shared.DatabaseSchemaRow{}
	for _, row := range dbSchemas {
		charMaxLength := int32(-1)
		if row.CharacterMaximumLength.Valid {
			charMaxLength = row.CharacterMaximumLength.Int32
		}
		numericPrecision := int32(-1)
		if row.NumericPrecision.Valid {
			numericPrecision = int32(row.NumericPrecision.Int16)
		}
		numericScale := int32(-1)
		if row.NumericScale.Valid {
			numericScale = int32(row.NumericScale.Int16)
		}
		ordPosition := int16(-1)
		if row.OrdinalPosition >= math.MinInt16 && row.OrdinalPosition <= math.MaxInt16 {
			ordPosition = int16(row.OrdinalPosition) //nolint:gosec
		}
		var identityGeneration *string
		if row.IsIdentity {
			syntax := defaultIdentity
			identityGeneration = &syntax
		}
		var generatedType *string
		if row.GenerationExpression.Valid {
			generatedType = &row.GenerationExpression.String
		}

		output = append(output, &sqlmanager_shared.DatabaseSchemaRow{
			TableSchema:            row.TableSchema,
			TableName:              row.TableName,
			ColumnName:             row.ColumnName,
			DataType:               row.DataType,
			ColumnDefault:          row.ColumnDefault, // todo: make sure this is valid for the other funcs
			IsNullable:             row.IsNullable,
			GeneratedType:          generatedType,
			OrdinalPosition:        ordPosition,
			CharacterMaximumLength: charMaxLength,
			NumericPrecision:       numericPrecision,
			NumericScale:           numericScale,
			IdentityGeneration:     identityGeneration,
		})
	}

	return output, nil
}

func (m *Manager) GetSchemaColumnMap(ctx context.Context) (map[string]map[string]*sqlmanager_shared.ColumnInfo, error) {
	dbSchemas, err := m.GetDatabaseSchema(ctx)
	if err != nil {
		return nil, err
	}
	result := sqlmanager_shared.GetUniqueSchemaColMappings(dbSchemas)
	return result, nil
}

func (m *Manager) GetTableConstraintsBySchema(ctx context.Context, schemas []string) (*sqlmanager_shared.TableConstraints, error) {
	if len(schemas) == 0 {
		return &sqlmanager_shared.TableConstraints{}, nil
	}
	rows, err := m.querier.GetTableConstraintsBySchemas(ctx, m.db, schemas)
	if err != nil && !nucleusdb.IsNoRows(err) {
		return nil, err
	} else if err != nil && nucleusdb.IsNoRows(err) {
		return &sqlmanager_shared.TableConstraints{}, nil
	}

	foreignKeyMap := map[string][]*sqlmanager_shared.ForeignConstraint{}
	primaryKeyMap := map[string][]string{}
	uniqueConstraintsMap := map[string][][]string{}

	for _, row := range rows {
		tableName := sqlmanager_shared.BuildTable(row.SchemaName, row.TableName)
		constraintCols := splitAndStrip(row.ConstraintColumns, ", ")

		switch row.ConstraintType {
		case "FOREIGN KEY":
			if row.ReferencedColumns.Valid && row.ReferencedTable.Valid {
				fkCols := splitAndStrip(row.ReferencedColumns.String, ", ")

				ccNullability := splitAndStrip(row.ConstraintColumnsNullability, ", ")
				notNullable := []bool{}
				for _, nullability := range ccNullability {
					notNullable = append(notNullable, nullability == "NOT NULL")
				}
				if len(constraintCols) != len(fkCols) {
					return nil, fmt.Errorf("length of columns was not equal to length of foreign key cols: %d %d", len(constraintCols), len(fkCols))
				}
				if len(constraintCols) != len(notNullable) {
					return nil, fmt.Errorf("length of columns was not equal to length of not nullable cols: %d %d", len(constraintCols), len(notNullable))
				}

				foreignKeyMap[tableName] = append(foreignKeyMap[tableName], &sqlmanager_shared.ForeignConstraint{
					Columns:     constraintCols,
					NotNullable: notNullable,
					ForeignKey: &sqlmanager_shared.ForeignKey{
						Table:   row.ReferencedTable.String,
						Columns: fkCols,
					},
				})
			}

		case "PRIMARY KEY":
			if _, exists := primaryKeyMap[tableName]; !exists {
				primaryKeyMap[tableName] = []string{}
			}
			primaryKeyMap[tableName] = append(primaryKeyMap[tableName], sqlmanager_shared.DedupeSlice(constraintCols)...)
		case "UNIQUE":
			columns := sqlmanager_shared.DedupeSlice(constraintCols)
			uniqueConstraintsMap[tableName] = append(uniqueConstraintsMap[tableName], columns)
		}
	}

	return &sqlmanager_shared.TableConstraints{
		ForeignKeyConstraints: foreignKeyMap,
		PrimaryKeyConstraints: primaryKeyMap,
		UniqueConstraints:     uniqueConstraintsMap,
	}, nil
}

func (m *Manager) GetRolePermissionsMap(ctx context.Context) (map[string][]string, error) {
	rows, err := m.querier.GetRolePermissions(ctx, m.db)
	if err != nil && !nucleusdb.IsNoRows(err) {
		return nil, fmt.Errorf("unable to retrieve mssql role permissions: %w", err)
	} else if err != nil && nucleusdb.IsNoRows(err) {
		return map[string][]string{}, nil
	}

	schemaTablePrivsMap := map[string][]string{}
	for _, permission := range rows {
		key := sqlmanager_shared.BuildTable(permission.TableSchema, permission.TableName)
		schemaTablePrivsMap[key] = append(schemaTablePrivsMap[key], permission.PrivilegeType)
	}
	return schemaTablePrivsMap, err
}

func splitAndStrip(input, delim string) []string {
	output := []string{}

	for _, piece := range strings.Split(input, delim) {
		if strings.TrimSpace(piece) != "" {
			output = append(output, piece)
		}
	}

	return output
}

func (m *Manager) GetTableInitStatements(ctx context.Context, tables []*sqlmanager_shared.SchemaTable) ([]*sqlmanager_shared.TableInitStatement, error) {
	return []*sqlmanager_shared.TableInitStatement{}, nil
}

func (m *Manager) GetSchemaTableDataTypes(ctx context.Context, tables []*sqlmanager_shared.SchemaTable) (*sqlmanager_shared.SchemaTableDataTypeResponse, error) {
	return &sqlmanager_shared.SchemaTableDataTypeResponse{
		Sequences:  []*sqlmanager_shared.DataType{},
		Functions:  []*sqlmanager_shared.DataType{},
		Composites: []*sqlmanager_shared.DataType{},
		Enums:      []*sqlmanager_shared.DataType{},
		Domains:    []*sqlmanager_shared.DataType{},
	}, nil
}

func (m *Manager) GetSchemaTableTriggers(ctx context.Context, tables []*sqlmanager_shared.SchemaTable) ([]*sqlmanager_shared.TableTrigger, error) {
	return []*sqlmanager_shared.TableTrigger{}, nil
}

func (m *Manager) GetSchemaInitStatements(ctx context.Context, tables []*sqlmanager_shared.SchemaTable) ([]*sqlmanager_shared.InitSchemaStatements, error) {
	return []*sqlmanager_shared.InitSchemaStatements{}, nil
}

func (m *Manager) GetCreateTableStatement(ctx context.Context, schema, table string) (string, error) {
	return "", errors.ErrUnsupported
}

func (m *Manager) BatchExec(ctx context.Context, batchSize int, statements []string, opts *sqlmanager_shared.BatchExecOpts) error {
	// mssql does not support batching statements
	total := len(statements)
	for idx, stmt := range statements {
		err := m.Exec(ctx, stmt)
		if err != nil {
			return fmt.Errorf("failed to execute batch statement %d/%d: %w", idx+1, total, err)
		}
	}
	return nil
}

func (m *Manager) GetTableRowCount(
	ctx context.Context,
	schema, table string,
	whereClause *string,
) (int64, error) {
	tableName := sqlmanager_shared.BuildTable(schema, table)
	builder := goqu.Dialect(sqlmanager_shared.MssqlDriver)

	query := builder.From(goqu.I(tableName)).Select(goqu.COUNT("*"))
	if whereClause != nil && *whereClause != "" {
		query = query.Where(goqu.L(*whereClause))
	}
	sql, _, err := query.ToSQL()
	if err != nil {
		return 0, fmt.Errorf("unable to build table row count statement for mssql: %w", err)
	}
	var count int64
	err = m.db.QueryRowContext(ctx, sql).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("unable to query table row count for mssql: %w", err)
	}
	return count, err
}

func (m *Manager) Exec(ctx context.Context, statement string) error {
	_, err := m.db.ExecContext(ctx, statement)
	return err
}

func (m *Manager) Close() {
	if m.db != nil && m.close != nil {
		m.close()
	}
}
