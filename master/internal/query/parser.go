// Package query provides a hand-written lexer + recursive-descent parser
// for a SQL subset understood by the distributed database.
//
// Supported statements:
//
//	CREATE DATABASE <name>
//	DROP DATABASE <name>
//	CREATE TABLE <name> (<col> <type>, ...)
//	DROP TABLE <name>
//	INSERT INTO <table> (<cols>) VALUES (<vals>)
//	SELECT * FROM <table> [WHERE <col> <op> <val>] [LIMIT <n>]
//	UPDATE <table> SET <col>=<val> [, ...] [WHERE <col> <op> <val>]
//	DELETE FROM <table> [WHERE <col> <op> <val>]
package query

import (
	"fmt"
	"strconv"
	"strings"
)

// ── Statement types ───────────────────────────────────────────────────────────

type StmtKind string

const (
	KindCreateDB    StmtKind = "CREATE_DB"
	KindDropDB      StmtKind = "DROP_DB"
	KindCreateTable StmtKind = "CREATE_TABLE"
	KindDropTable   StmtKind = "DROP_TABLE"
	KindInsert      StmtKind = "INSERT"
	KindSelect      StmtKind = "SELECT"
	KindUpdate      StmtKind = "UPDATE"
	KindDelete      StmtKind = "DELETE"
)

// ColDef describes a column in CREATE TABLE.
type ColDef struct {
	Name string
	Type string // e.g. "VARCHAR(255)", "INT", "TEXT", "FLOAT"
}

// WhereClause is a single equality / comparison predicate.
type WhereClause struct {
	Col  string
	Op   string // "=", "!=", "<", "<=", ">", ">="
	Val  any    // string or float64
}

// Statement is the result of parsing one SQL statement.
type Statement struct {
	Kind StmtKind

	// CREATE / DROP DATABASE
	Database string

	// CREATE / DROP / INSERT / SELECT / UPDATE / DELETE
	Table string

	// CREATE TABLE
	Cols []ColDef

	// INSERT
	InsertCols []string
	InsertVals []any

	// SELECT
	Where *WhereClause
	Limit int // 0 = no limit

	// UPDATE
	SetCols map[string]any
	// (shares Where with SELECT)

	// DELETE
	// (shares Where)
}

// Parse parses a single SQL statement string and returns a Statement.
func Parse(input string) (*Statement, error) {
	p := &parser{lex: newLexer(input)}
	return p.parseStatement()
}

// ── Lexer ─────────────────────────────────────────────────────────────────────

type tokenKind int

const (
	tokWord    tokenKind = iota // keyword or identifier
	tokNumber                  // integer or float literal
	tokString                  // single-quoted string literal
	tokComma                   // ,
	tokLP                      // (
	tokRP                      // )
	tokEq                      // =
	tokNeq                     // !=
	tokLT                      // <
	tokLTE                     // <=
	tokGT                      // >
	tokGTE                     // >=
	tokStar                    // *
	tokSemi                    // ;
	tokEOF
)

type token struct {
	kind tokenKind
	val  string
}

type lexer struct {
	src  []rune
	pos  int
	toks []token
	cur  int
}

func newLexer(src string) *lexer {
	l := &lexer{src: []rune(src)}
	l.tokenise()
	return l
}

func (l *lexer) tokenise() {
	for l.pos < len(l.src) {
		ch := l.src[l.pos]

		// Whitespace
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			l.pos++
			continue
		}

		// Single-line comment
		if ch == '-' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '-' {
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.pos++
			}
			continue
		}

		switch ch {
		case ',':
			l.toks = append(l.toks, token{tokComma, ","})
			l.pos++
		case '(':
			l.toks = append(l.toks, token{tokLP, "("})
			l.pos++
		case ')':
			l.toks = append(l.toks, token{tokRP, ")"})
			l.pos++
		case '*':
			l.toks = append(l.toks, token{tokStar, "*"})
			l.pos++
		case ';':
			l.toks = append(l.toks, token{tokSemi, ";"})
			l.pos++
		case '=':
			l.toks = append(l.toks, token{tokEq, "="})
			l.pos++
		case '!':
			if l.pos+1 < len(l.src) && l.src[l.pos+1] == '=' {
				l.toks = append(l.toks, token{tokNeq, "!="})
				l.pos += 2
			} else {
				l.pos++
			}
		case '<':
			if l.pos+1 < len(l.src) && l.src[l.pos+1] == '=' {
				l.toks = append(l.toks, token{tokLTE, "<="})
				l.pos += 2
			} else {
				l.toks = append(l.toks, token{tokLT, "<"})
				l.pos++
			}
		case '>':
			if l.pos+1 < len(l.src) && l.src[l.pos+1] == '=' {
				l.toks = append(l.toks, token{tokGTE, ">="})
				l.pos += 2
			} else {
				l.toks = append(l.toks, token{tokGT, ">"})
				l.pos++
			}
		case '\'':
			// String literal
			l.pos++
			start := l.pos
			for l.pos < len(l.src) && l.src[l.pos] != '\'' {
				l.pos++
			}
			l.toks = append(l.toks, token{tokString, string(l.src[start:l.pos])})
			l.pos++ // consume closing quote
		default:
			// Number
			if ch >= '0' && ch <= '9' || ch == '-' {
				start := l.pos
				l.pos++
				for l.pos < len(l.src) && (l.src[l.pos] >= '0' && l.src[l.pos] <= '9' || l.src[l.pos] == '.') {
					l.pos++
				}
				l.toks = append(l.toks, token{tokNumber, string(l.src[start:l.pos])})
				continue
			}
			// Word / identifier (allow backtick-quoted)
			if ch == '`' {
				l.pos++
				start := l.pos
				for l.pos < len(l.src) && l.src[l.pos] != '`' {
					l.pos++
				}
				l.toks = append(l.toks, token{tokWord, string(l.src[start:l.pos])})
				l.pos++
				continue
			}
			if isWordStart(ch) {
				start := l.pos
				for l.pos < len(l.src) && isWordChar(l.src[l.pos]) {
					l.pos++
				}
				l.toks = append(l.toks, token{tokWord, string(l.src[start:l.pos])})
				continue
			}
			l.pos++ // skip unknown char
		}
	}
	l.toks = append(l.toks, token{tokEOF, ""})
}

func isWordStart(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_'
}
func isWordChar(r rune) bool {
	return isWordStart(r) || r >= '0' && r <= '9'
}

func (l *lexer) peek() token {
	if l.cur >= len(l.toks) {
		return token{tokEOF, ""}
	}
	return l.toks[l.cur]
}

func (l *lexer) next() token {
	t := l.peek()
	l.cur++
	return t
}

func (l *lexer) expectWord(kw string) error {
	t := l.next()
	if t.kind != tokWord || !strings.EqualFold(t.val, kw) {
		return fmt.Errorf("expected %q, got %q", kw, t.val)
	}
	return nil
}

func (l *lexer) expectKind(k tokenKind, desc string) (token, error) {
	t := l.next()
	if t.kind != k {
		return t, fmt.Errorf("expected %s, got %q", desc, t.val)
	}
	return t, nil
}

// ── Parser ────────────────────────────────────────────────────────────────────

type parser struct{ lex *lexer }

func (p *parser) parseStatement() (*Statement, error) {
	t := p.lex.peek()
	if t.kind != tokWord {
		return nil, fmt.Errorf("expected SQL keyword, got %q", t.val)
	}
	switch strings.ToUpper(t.val) {
	case "CREATE":
		return p.parseCreate()
	case "DROP":
		return p.parseDrop()
	case "INSERT":
		return p.parseInsert()
	case "SELECT":
		return p.parseSelect()
	case "UPDATE":
		return p.parseUpdate()
	case "DELETE":
		return p.parseDelete()
	default:
		return nil, fmt.Errorf("unsupported statement: %q", t.val)
	}
}

func (p *parser) parseCreate() (*Statement, error) {
	p.lex.next() // consume CREATE
	t := p.lex.next()
	if t.kind != tokWord {
		return nil, fmt.Errorf("expected DATABASE or TABLE after CREATE")
	}
	switch strings.ToUpper(t.val) {
	case "DATABASE":
		name, err := p.lex.expectKind(tokWord, "database name")
		if err != nil {
			return nil, err
		}
		return &Statement{Kind: KindCreateDB, Database: name.val}, nil
	case "TABLE":
		return p.parseCreateTable()
	default:
		return nil, fmt.Errorf("expected DATABASE or TABLE, got %q", t.val)
	}
}

func (p *parser) parseCreateTable() (*Statement, error) {
	name, err := p.lex.expectKind(tokWord, "table name")
	if err != nil {
		return nil, err
	}
	if _, err := p.lex.expectKind(tokLP, "("); err != nil {
		return nil, err
	}
	var cols []ColDef
	for {
		colName, err := p.lex.expectKind(tokWord, "column name")
		if err != nil {
			return nil, err
		}
		// Collect type tokens until comma or closing paren.
		var typeParts []string
		for {
			t := p.lex.peek()
			if t.kind == tokComma || t.kind == tokRP || t.kind == tokEOF {
				break
			}
			typeParts = append(typeParts, p.lex.next().val)
		}
		cols = append(cols, ColDef{Name: colName.val, Type: strings.Join(typeParts, " ")})
		t := p.lex.peek()
		if t.kind == tokRP {
			p.lex.next()
			break
		}
		if t.kind == tokComma {
			p.lex.next()
			continue
		}
		return nil, fmt.Errorf("unexpected token %q in column list", t.val)
	}
	return &Statement{Kind: KindCreateTable, Table: name.val, Cols: cols}, nil
}

func (p *parser) parseDrop() (*Statement, error) {
	p.lex.next() // consume DROP
	t := p.lex.next()
	if t.kind != tokWord {
		return nil, fmt.Errorf("expected DATABASE or TABLE after DROP")
	}
	switch strings.ToUpper(t.val) {
	case "DATABASE":
		name, err := p.lex.expectKind(tokWord, "database name")
		if err != nil {
			return nil, err
		}
		return &Statement{Kind: KindDropDB, Database: name.val}, nil
	case "TABLE":
		name, err := p.lex.expectKind(tokWord, "table name")
		if err != nil {
			return nil, err
		}
		return &Statement{Kind: KindDropTable, Table: name.val}, nil
	default:
		return nil, fmt.Errorf("expected DATABASE or TABLE, got %q", t.val)
	}
}

func (p *parser) parseInsert() (*Statement, error) {
	p.lex.next() // INSERT
	if err := p.lex.expectWord("INTO"); err != nil {
		return nil, err
	}
	table, err := p.lex.expectKind(tokWord, "table name")
	if err != nil {
		return nil, err
	}
	if _, err := p.lex.expectKind(tokLP, "("); err != nil {
		return nil, err
	}
	cols, err := p.parseWordList()
	if err != nil {
		return nil, err
	}
	if err := p.lex.expectWord("VALUES"); err != nil {
		return nil, err
	}
	if _, err := p.lex.expectKind(tokLP, "("); err != nil {
		return nil, err
	}
	vals, err := p.parseValueList()
	if err != nil {
		return nil, err
	}
	if len(cols) != len(vals) {
		return nil, fmt.Errorf("column count (%d) != value count (%d)", len(cols), len(vals))
	}
	return &Statement{Kind: KindInsert, Table: table.val, InsertCols: cols, InsertVals: vals}, nil
}

func (p *parser) parseSelect() (*Statement, error) {
	p.lex.next() // SELECT
	// Consume * or column list (we always do SELECT * internally)
	t := p.lex.peek()
	if t.kind == tokStar {
		p.lex.next()
	} else {
		// skip column list until FROM
		for t.kind != tokWord || !strings.EqualFold(t.val, "FROM") {
			if t.kind == tokEOF {
				return nil, fmt.Errorf("expected FROM")
			}
			p.lex.next()
			t = p.lex.peek()
		}
	}
	if err := p.lex.expectWord("FROM"); err != nil {
		return nil, err
	}
	table, err := p.lex.expectKind(tokWord, "table name")
	if err != nil {
		return nil, err
	}
	stmt := &Statement{Kind: KindSelect, Table: table.val}

	// Optional WHERE
	if t := p.lex.peek(); t.kind == tokWord && strings.EqualFold(t.val, "WHERE") {
		p.lex.next()
		wc, err := p.parseWhere()
		if err != nil {
			return nil, err
		}
		stmt.Where = wc
	}
	// Optional LIMIT
	if t := p.lex.peek(); t.kind == tokWord && strings.EqualFold(t.val, "LIMIT") {
		p.lex.next()
		n, err := p.lex.expectKind(tokNumber, "limit value")
		if err != nil {
			return nil, err
		}
		lim, _ := strconv.Atoi(n.val)
		stmt.Limit = lim
	}
	return stmt, nil
}

func (p *parser) parseUpdate() (*Statement, error) {
	p.lex.next() // UPDATE
	table, err := p.lex.expectKind(tokWord, "table name")
	if err != nil {
		return nil, err
	}
	if err := p.lex.expectWord("SET"); err != nil {
		return nil, err
	}
	setMap := make(map[string]any)
	for {
		col, err := p.lex.expectKind(tokWord, "column name")
		if err != nil {
			return nil, err
		}
		if _, err := p.lex.expectKind(tokEq, "="); err != nil {
			return nil, err
		}
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		setMap[col.val] = val
		if t := p.lex.peek(); t.kind == tokComma {
			p.lex.next()
		} else {
			break
		}
	}
	stmt := &Statement{Kind: KindUpdate, Table: table.val, SetCols: setMap}
	if t := p.lex.peek(); t.kind == tokWord && strings.EqualFold(t.val, "WHERE") {
		p.lex.next()
		wc, err := p.parseWhere()
		if err != nil {
			return nil, err
		}
		stmt.Where = wc
	}
	return stmt, nil
}

func (p *parser) parseDelete() (*Statement, error) {
	p.lex.next() // DELETE
	if err := p.lex.expectWord("FROM"); err != nil {
		return nil, err
	}
	table, err := p.lex.expectKind(tokWord, "table name")
	if err != nil {
		return nil, err
	}
	stmt := &Statement{Kind: KindDelete, Table: table.val}
	if t := p.lex.peek(); t.kind == tokWord && strings.EqualFold(t.val, "WHERE") {
		p.lex.next()
		wc, err := p.parseWhere()
		if err != nil {
			return nil, err
		}
		stmt.Where = wc
	}
	return stmt, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (p *parser) parseWordList() ([]string, error) {
	var out []string
	for {
		t, err := p.lex.expectKind(tokWord, "column name")
		if err != nil {
			return nil, err
		}
		out = append(out, t.val)
		if next := p.lex.peek(); next.kind == tokRP {
			p.lex.next()
			break
		}
		if _, err := p.lex.expectKind(tokComma, ","); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (p *parser) parseValueList() ([]any, error) {
	var out []any
	for {
		v, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		if t := p.lex.peek(); t.kind == tokRP {
			p.lex.next()
			break
		}
		if _, err := p.lex.expectKind(tokComma, ","); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (p *parser) parseValue() (any, error) {
	t := p.lex.next()
	switch t.kind {
	case tokString:
		return t.val, nil
	case tokNumber:
		f, err := strconv.ParseFloat(t.val, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q", t.val)
		}
		return f, nil
	case tokWord:
		upper := strings.ToUpper(t.val)
		if upper == "NULL" {
			return nil, nil
		}
		if upper == "TRUE" {
			return true, nil
		}
		if upper == "FALSE" {
			return false, nil
		}
		return t.val, nil
	default:
		return nil, fmt.Errorf("expected value, got %q", t.val)
	}
}

func (p *parser) parseWhere() (*WhereClause, error) {
	col, err := p.lex.expectKind(tokWord, "column name")
	if err != nil {
		return nil, err
	}
	// operator
	opTok := p.lex.next()
	var op string
	switch opTok.kind {
	case tokEq:
		op = "="
	case tokNeq:
		op = "!="
	case tokLT:
		op = "<"
	case tokLTE:
		op = "<="
	case tokGT:
		op = ">"
	case tokGTE:
		op = ">="
	default:
		return nil, fmt.Errorf("expected comparison operator, got %q", opTok.val)
	}
	val, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	return &WhereClause{Col: col.val, Op: op, Val: val}, nil
}