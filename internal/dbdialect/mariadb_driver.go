package dbdialect

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
)

const mariaDBDriverName = "euromni-mariadb"

func init() {
	sql.Register(mariaDBDriverName, mariaDBDriver{})
}

type mariaDBDriver struct{}

func (mariaDBDriver) Open(name string) (driver.Conn, error) {
	conn, err := (&mysql.MySQLDriver{}).Open(name)
	if err != nil {
		return nil, err
	}
	return &mariaDBConn{Conn: conn}, nil
}

type mariaDBConn struct {
	driver.Conn
}

func (c *mariaDBConn) Prepare(query string) (driver.Stmt, error) {
	rewritten, order, err := rewriteMariaDBQuery(query)
	if err != nil {
		return nil, err
	}
	stmt, err := c.Conn.Prepare(rewritten)
	if err != nil {
		return nil, err
	}
	return &mariaDBStmt{Stmt: stmt, order: order}, nil
}

func (c *mariaDBConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	rewritten, order, err := rewriteMariaDBQuery(query)
	if err != nil {
		return nil, err
	}
	var stmt driver.Stmt
	if preparer, ok := c.Conn.(driver.ConnPrepareContext); ok {
		stmt, err = preparer.PrepareContext(ctx, rewritten)
	} else {
		stmt, err = c.Conn.Prepare(rewritten)
	}
	if err != nil {
		return nil, err
	}
	return &mariaDBStmt{Stmt: stmt, order: order}, nil
}

func (c *mariaDBConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	rewritten, order, err := rewriteMariaDBQuery(query)
	if err != nil {
		return nil, err
	}
	ordered, err := reorderNamedValues(args, order)
	if err != nil {
		return nil, err
	}
	if execer, ok := c.Conn.(driver.ExecerContext); ok {
		return execer.ExecContext(ctx, rewritten, ordered)
	}
	stmt, err := c.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	return stmt.(driver.StmtExecContext).ExecContext(ctx, ordered)
}

func (c *mariaDBConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	rewritten, order, err := rewriteMariaDBQuery(query)
	if err != nil {
		return nil, err
	}
	ordered, err := reorderNamedValues(args, order)
	if err != nil {
		return nil, err
	}
	if querier, ok := c.Conn.(driver.QueryerContext); ok {
		return querier.QueryContext(ctx, rewritten, ordered)
	}
	stmt, err := c.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return stmt.(driver.StmtQueryContext).QueryContext(ctx, ordered)
}

func (c *mariaDBConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if beginner, ok := c.Conn.(driver.ConnBeginTx); ok {
		return beginner.BeginTx(ctx, opts)
	}
	if opts.Isolation != driver.IsolationLevel(sql.LevelDefault) || opts.ReadOnly {
		return nil, errors.New("MariaDB driver does not support requested transaction options")
	}
	return c.Conn.Begin()
}

func (c *mariaDBConn) Ping(ctx context.Context) error {
	if pinger, ok := c.Conn.(driver.Pinger); ok {
		return pinger.Ping(ctx)
	}
	return errors.New("MariaDB driver connection does not support ping")
}

type mariaDBStmt struct {
	driver.Stmt
	order []int
}

func (s *mariaDBStmt) NumInput() int {
	max := -1
	for _, index := range s.order {
		if index > max {
			max = index
		}
	}
	return max + 1
}

func (s *mariaDBStmt) Exec(args []driver.Value) (driver.Result, error) {
	ordered, err := reorderValues(args, s.order)
	if err != nil {
		return nil, err
	}
	return s.Stmt.Exec(ordered)
}

func (s *mariaDBStmt) Query(args []driver.Value) (driver.Rows, error) {
	ordered, err := reorderValues(args, s.order)
	if err != nil {
		return nil, err
	}
	return s.Stmt.Query(ordered)
}

func (s *mariaDBStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	ordered, err := reorderNamedValues(args, s.order)
	if err != nil {
		return nil, err
	}
	if execer, ok := s.Stmt.(driver.StmtExecContext); ok {
		return execer.ExecContext(ctx, ordered)
	}
	return nil, driver.ErrSkip
}

func (s *mariaDBStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	ordered, err := reorderNamedValues(args, s.order)
	if err != nil {
		return nil, err
	}
	if querier, ok := s.Stmt.(driver.StmtQueryContext); ok {
		return querier.QueryContext(ctx, ordered)
	}
	return nil, driver.ErrSkip
}

var (
	mariaDBCastPattern       = regexp.MustCompile(`(?i)::\s*(?:jsonb|text|uuid|integer|int|numeric\s*\(\s*19\s*,\s*4\s*\))`)
	mariaDBUpsertPattern     = regexp.MustCompile(`(?is)\bON\s+CONFLICT\s*(?:\([^)]*\))?\s+DO\s+UPDATE\s+SET\b`)
	mariaDBExcludedPattern   = regexp.MustCompile(`(?i)\bEXCLUDED\.([a-z_][a-z0-9_]*)`)
	mariaDBLockClausePattern = regexp.MustCompile(`(?i)\bFOR\s+UPDATE\s+OF\s+[a-z_][a-z0-9_]*(?:\s*,\s*[a-z_][a-z0-9_]*)*`)
	mariaDBReturningPattern  = regexp.MustCompile(`(?is)\s+RETURNING\b`)
	mariaDBJSONContains      = regexp.MustCompile(`(?i)([a-z_][a-z0-9_.]*)\s*@>\s*jsonb_build_array\(\s*(\$[0-9]+)\s*\)`)
	mariaDBArrayToJSON       = regexp.MustCompile(`(?i)array_to_json\(\s*([a-z_][a-z0-9_.]*)\s*\)`)
)

func rewriteMariaDBQuery(query string) (string, []int, error) {
	if mariaDBReturningPattern.MatchString(query) {
		return "", nil, errors.New("MariaDB does not support PostgreSQL RETURNING; use an application-generated identifier")
	}
	query = mariaDBCastPattern.ReplaceAllString(query, "")
	query = mariaDBJSONContains.ReplaceAllString(query, "JSON_CONTAINS($1, JSON_QUOTE($2))")
	query = mariaDBArrayToJSON.ReplaceAllString(query, "$1")
	query = mariaDBUpsertPattern.ReplaceAllString(query, "ON DUPLICATE KEY UPDATE")
	query = mariaDBExcludedPattern.ReplaceAllString(query, "VALUES($1)")
	query = mariaDBLockClausePattern.ReplaceAllString(query, "FOR UPDATE")

	var output strings.Builder
	order := make([]int, 0)
	state := byte(0)
	for i := 0; i < len(query); i++ {
		ch := query[i]
		switch state {
		case '\'', '"', '`':
			output.WriteByte(ch)
			if ch == state {
				if i+1 < len(query) && query[i+1] == state {
					output.WriteByte(query[i+1])
					i++
				} else {
					state = 0
				}
			} else if ch == '\\' && state != '`' && i+1 < len(query) {
				output.WriteByte(query[i+1])
				i++
			}
		case '-':
			output.WriteByte(ch)
			if ch == '\n' {
				state = 0
			}
		case '/':
			output.WriteByte(ch)
			if ch == '*' && i+1 < len(query) && query[i+1] == '/' {
				output.WriteByte(query[i+1])
				i++
				state = 0
			}
		default:
			if ch == '\'' || ch == '"' || ch == '`' {
				state = ch
				output.WriteByte(ch)
			} else if ch == '-' && i+1 < len(query) && query[i+1] == '-' {
				output.WriteByte(ch)
				output.WriteByte(query[i+1])
				i++
				state = '-'
			} else if ch == '/' && i+1 < len(query) && query[i+1] == '*' {
				output.WriteByte(ch)
				output.WriteByte(query[i+1])
				i++
				state = '/'
			} else if ch == '$' && i+1 < len(query) && query[i+1] >= '0' && query[i+1] <= '9' {
				end := i + 1
				for end < len(query) && query[end] >= '0' && query[end] <= '9' {
					end++
				}
				index, err := strconv.Atoi(query[i+1 : end])
				if err != nil || index < 1 {
					return "", nil, fmt.Errorf("invalid PostgreSQL placeholder %q", query[i:end])
				}
				order = append(order, index-1)
				output.WriteByte('?')
				i = end - 1
			} else {
				output.WriteByte(ch)
			}
		}
	}
	return output.String(), order, nil
}

func reorderNamedValues(args []driver.NamedValue, order []int) ([]driver.NamedValue, error) {
	ordered := make([]driver.NamedValue, len(order))
	for i, index := range order {
		if index >= len(args) {
			return nil, fmt.Errorf("SQL placeholder $%d has no argument", index+1)
		}
		ordered[i] = args[index]
		ordered[i].Ordinal = i + 1
	}
	if len(order) == 0 && len(args) != 0 {
		return nil, fmt.Errorf("SQL statement has %d arguments but no placeholders", len(args))
	}
	return ordered, nil
}

func reorderValues(args []driver.Value, order []int) ([]driver.Value, error) {
	ordered := make([]driver.Value, len(order))
	for i, index := range order {
		if index >= len(args) {
			return nil, fmt.Errorf("SQL placeholder $%d has no argument", index+1)
		}
		ordered[i] = args[index]
	}
	if len(order) == 0 && len(args) != 0 {
		return nil, fmt.Errorf("SQL statement has %d arguments but no placeholders", len(args))
	}
	return ordered, nil
}
