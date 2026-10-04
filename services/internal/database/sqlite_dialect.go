package database

import (
	"fmt"
	"strings"
	"unicode"
)

// sqlTokens keeps quoted values and comments opaque. Dialect lowering never
// rewrites substrings inside user data, JSON literals or quoted identifiers.
// This is the shared query vocabulary used by our repositories, not a general
// PostgreSQL emulator. Complex state changes belong in explicit transactions.
func sqlTokens(q string) ([]string, error) {
	var out []string
	for i := 0; i < len(q); {
		c := q[i]
		if unicode.IsSpace(rune(c)) {
			i++
			continue
		}
		if strings.HasPrefix(q[i:], "--") {
			for i < len(q) && q[i] != '\n' {
				i++
			}
			continue
		}
		if strings.HasPrefix(q[i:], "/*") {
			n := strings.Index(q[i+2:], "*/")
			if n < 0 {
				return nil, fmt.Errorf("unterminated SQL comment")
			}
			i += n + 4
			continue
		}
		start := i
		if c == '\'' || c == '"' {
			i++
			closed := false
			for i < len(q) {
				if q[i] == c {
					i++
					if i < len(q) && q[i] == c {
						i++
						continue
					}
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated SQL quote")
			}
			out = append(out, q[start:i])
			continue
		}
		matched := false
		for _, op := range []string{"->>", "::", "->", "||", "<=", ">=", "<>", "!="} {
			if strings.HasPrefix(q[i:], op) {
				out = append(out, op)
				i += len(op)
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		if (c >= '0' && c <= '9') || (c == '.' && i+1 < len(q) && q[i+1] >= '0' && q[i+1] <= '9') {
			i++
			for i < len(q) && q[i] >= '0' && q[i] <= '9' {
				i++
			}
			if i < len(q) && q[i] == '.' {
				i++
				for i < len(q) && q[i] >= '0' && q[i] <= '9' {
					i++
				}
			}
			if i < len(q) && (q[i] == 'e' || q[i] == 'E') {
				i++
				if i < len(q) && (q[i] == '+' || q[i] == '-') {
					i++
				}
				for i < len(q) && q[i] >= '0' && q[i] <= '9' {
					i++
				}
			}
			out = append(out, q[start:i])
			continue
		}
		if c == '$' || c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			i++
			for i < len(q) {
				c = q[i]
				if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
					break
				}
				i++
			}
			out = append(out, q[start:i])
			continue
		}
		out = append(out, q[i:i+1])
		i++
	}
	return out, nil
}
func matchingEnd(t []string, start int) int {
	depth := 0
	for i := start; i < len(t); i++ {
		if t[i] == "(" {
			depth++
		}
		if t[i] == ")" {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
func expressionStart(t []string, end int) int {
	start := end
	if t[end] == ")" {
		depth := 1
		start--
		for start >= 0 {
			if t[start] == ")" {
				depth++
			}
			if t[start] == "(" {
				depth--
				if depth == 0 {
					break
				}
			}
			start--
		}
		if start > 0 && isSQLWord(t[start-1]) {
			start--
		}
	}
	for start >= 2 && t[start-1] == "." {
		start -= 2
	}
	return start
}
func isSQLWord(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
func sqliteQuery(q string, args []any) (string, []any, error) {
	t, err := sqlTokens(q)
	if err != nil {
		return "", nil, err
	}
	// Resolve table-valued JSON aliases before lowering their member expressions.
	aliases := map[string]bool{}
	for i := 0; i+1 < len(t); i++ {
		if strings.EqualFold(t[i], "jsonb_array_elements") && t[i+1] == "(" {
			end := matchingEnd(t, i+1)
			if end < 0 || end+1 >= len(t) {
				return "", nil, fmt.Errorf("invalid JSON elements query")
			}
			alias := end + 1
			if strings.EqualFold(t[alias], "AS") {
				alias++
			}
			if alias >= len(t) {
				return "", nil, fmt.Errorf("missing JSON elements alias")
			}
			aliases[t[alias]] = true
		}
	}
	out := []string{}
	for i := 0; i < len(t); i++ {
		v := t[i]
		upper := strings.ToUpper(v)
		if (upper == "EFFECTIVE_DOCUMENT_ROLE" || upper == "EFFECTIVE_FOLDER_ROLE") && i+1 < len(t) && t[i+1] == "(" {
			end := matchingEnd(t, i+1)
			if end < 0 {
				return "", nil, fmt.Errorf("invalid permission expression")
			}
			comma := -1
			depth := 0
			for k := i + 2; k < end; k++ {
				if t[k] == "(" {
					depth++
				}
				if t[k] == ")" {
					depth--
				}
				if t[k] == "," && depth == 0 {
					comma = k
					break
				}
			}
			if comma < 0 {
				return "", nil, fmt.Errorf("invalid permission arguments")
			}
			target := strings.Join(t[i+2:comma], " ")
			principal := strings.Join(t[comma+1:end], " ")
			template := sqliteFolderRole
			if upper == "EFFECTIVE_DOCUMENT_ROLE" {
				template = sqliteDocumentRole
			}
			expanded, _, e := sqliteQuery(fmt.Sprintf(template, target, principal), nil)
			if e != nil {
				return "", nil, e
			}
			out = append(out, expanded)
			i = end
			continue
		}
		if v == "occccad" && i+1 < len(t) && t[i+1] == "." {
			i++
			continue
		}
		if upper == "LATERAL" {
			if i+1 >= len(t) || !strings.EqualFold(t[i+1], "jsonb_array_elements") {
				return "", nil, fmt.Errorf("unsupported SQLite lateral query")
			}
			continue
		}
		if upper == "FOR" && i+1 < len(t) && strings.EqualFold(t[i+1], "UPDATE") {
			i++
			if i+2 < len(t) && strings.EqualFold(t[i+1], "SKIP") && strings.EqualFold(t[i+2], "LOCKED") {
				i += 2
			}
			continue
		}
		// Our SQLite write transactions are BEGIN IMMEDIATE: row locks are replaced
		// by a database write reservation, never by an unlocked read-then-write.
		if strings.EqualFold(v, "now") && i+4 < len(t) && t[i+1] == "(" && t[i+2] == ")" && (t[i+3] == "+" || t[i+3] == "-") {
			sign := t[i+3]
			j := i + 4
			duration := ""
			if strings.EqualFold(t[j], "interval") && j+1 < len(t) {
				duration = t[j+1]
				j++
			} else if j+2 < len(t) && t[j+1] == "::" && strings.EqualFold(t[j+2], "interval") {
				duration = t[j]
				j += 2
			}
			if duration != "" {
				if strings.HasPrefix(duration, "$") {
					duration = "?" + duration[1:]
				}
				if sign == "-" {
					duration = "'-' || " + duration
				}
				out = append(out, "add_interval(now(),", duration, ")")
				i = j
				continue
			}
		}
		if v == "::" {
			if i+1 >= len(t) || len(out) == 0 {
				return "", nil, fmt.Errorf("invalid cast")
			}
			i++
			typ := strings.ToLower(t[i])
			switch typ {
			case "uuid", "text":
				start := expressionStart(out, len(out)-1)
				if start < 0 {
					return "", nil, fmt.Errorf("invalid text cast")
				}
				expr := strings.Join(out[start:], " ")
				out = append(out[:start], "CAST ( "+expr+" AS TEXT )")
			case "jsonb":
				start := expressionStart(out, len(out)-1)
				if start < 0 {
					return "", nil, fmt.Errorf("invalid JSON cast")
				}
				expr := strings.Join(out[start:], " ")
				out = append(out[:start], "json ( "+expr+" )")
			case "int", "integer", "bigint", "float8", "double":
				sqlType := "INTEGER"
				if typ == "float8" || typ == "double" {
					sqlType = "REAL"
					if i+1 < len(t) && strings.EqualFold(t[i+1], "precision") {
						i++
					}
				}
				start := expressionStart(out, len(out)-1)
				if start < 0 {
					return "", nil, fmt.Errorf("invalid cast expression")
				}
				expr := strings.Join(out[start:], " ")
				out = append(out[:start], "CAST ( "+expr+" AS "+sqlType+" )")
			default:
				return "", nil, fmt.Errorf("unsupported SQLite cast %q", typ)
			}
			continue
		}
		if upper == "ANY" && i+3 < len(t) && t[i+1] == "(" && strings.HasPrefix(t[i+2], "$") && t[i+3] == ")" {
			if len(out) == 0 || out[len(out)-1] != "=" {
				return "", nil, fmt.Errorf("unsupported ANY expression")
			}
			out[len(out)-1] = "IN"
			out = append(out, "(SELECT value FROM json_each(?"+t[i+2][1:]+"))")
			i += 3
			continue
		}
		if aliases[v] && i+1 < len(t) && (t[i+1] == "->" || t[i+1] == "->>") {
			v += ".value"
		}
		switch upper {
		case "JSONB_ARRAY_ELEMENTS":
			v = "json_each"
		case "JSONB_BUILD_OBJECT":
			v = "json_object"
		case "ARRAY_AGG":
			v = "json_group_array"
		case "JSONB_AGG":
			if i+3 < len(t) && t[i+1] == "(" && t[i+3] == ")" {
				out = append(out, "json_group_array(json("+t[i+2]+"))")
				i += 3
				continue
			}
			return "", nil, fmt.Errorf("unsupported JSON aggregation")
		case "GREATEST":
			v = "max"
		case "LEAST":
			v = "min"
		}
		if strings.HasPrefix(v, "$") {
			v = "?" + v[1:]
		}
		out = append(out, v)
	}
	bound, err := sqliteArgs(args)
	return strings.Join(out, " "), bound, err
}
