package modelcore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"text/scanner"
)

var (
	ErrParameterType = errors.New("PARAMETER_TYPE_MISMATCH")
	ErrUnitMismatch  = errors.New("UNIT_MISMATCH")
	ErrExpression    = errors.New("EXPRESSION_INVALID")
)

type Dimension struct {
	Length, Mass, Time, Current, Temperature, Amount, Luminous int8
	Semantic                                                   string
}

var (
	Dimensionless   = Dimension{}
	LengthDimension = Dimension{Length: 1}
	AngleDimension  = Dimension{Semantic: "ANGLE"}
)

func (dimension Dimension) Equal(other Dimension) bool { return dimension == other }
func (dimension Dimension) Add(other Dimension) Dimension {
	return Dimension{dimension.Length + other.Length, dimension.Mass + other.Mass,
		dimension.Time + other.Time, dimension.Current + other.Current,
		dimension.Temperature + other.Temperature, dimension.Amount + other.Amount,
		dimension.Luminous + other.Luminous, multiplySemantic(dimension.Semantic, other.Semantic)}
}
func (dimension Dimension) Sub(other Dimension) Dimension {
	return Dimension{dimension.Length - other.Length, dimension.Mass - other.Mass,
		dimension.Time - other.Time, dimension.Current - other.Current,
		dimension.Temperature - other.Temperature, dimension.Amount - other.Amount,
		dimension.Luminous - other.Luminous, divideSemantic(dimension.Semantic, other.Semantic)}
}

func multiplySemantic(left, right string) string {
	if left == "" {
		return right
	}
	if right == "" || left == right {
		return left
	}
	return left + "*" + right
}

func divideSemantic(left, right string) string {
	if right == "" {
		return left
	}
	if left == right {
		return ""
	}
	if left == "" {
		return "1/" + right
	}
	return left + "/" + right
}

type Quantity struct {
	SIValue   float64   `json:"siValue"`
	Dimension Dimension `json:"dimension"`
}

func NewQuantity(value float64, unit string) (Quantity, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return Quantity{}, fmt.Errorf("%w: quantity must be finite", ErrUnitMismatch)
	}
	definition, exists := units[strings.ToLower(strings.TrimSpace(unit))]
	if !exists {
		return Quantity{}, fmt.Errorf("%w: unsupported unit %q", ErrUnitMismatch, unit)
	}
	return Quantity{SIValue: value * definition.scale, Dimension: definition.dimension}, nil
}

var units = map[string]struct {
	scale     float64
	dimension Dimension
}{
	"": {1, Dimensionless}, "1": {1, Dimensionless},
	"m": {1, LengthDimension}, "mm": {0.001, LengthDimension}, "cm": {0.01, LengthDimension},
	"rad": {1, AngleDimension}, "deg": {math.Pi / 180, AngleDimension},
}

type ValueType string

const (
	ValueQuantity ValueType = "QUANTITY"
	ValueReal     ValueType = "REAL"
)

type ParameterDefinition struct {
	ParameterID string      `json:"parameterId"`
	Key         string      `json:"key"`
	Label       string      `json:"label"`
	ValueType   ValueType   `json:"valueType"`
	Dimension   Dimension   `json:"dimension"`
	Role        string      `json:"role"`
	Source      ValueSource `json:"source"`
}

type ValueSource struct {
	Literal    *Quantity        `json:"literal,omitempty"`
	Expression *TypedExpression `json:"expression,omitempty"`
}

type PropertySlotDescriptor struct {
	OwnerTypeURI   string    `json:"ownerTypeUri"`
	SlotID         string    `json:"slotId"`
	ValueType      ValueType `json:"valueType"`
	Dimension      Dimension `json:"dimension"`
	AllowedSources []string  `json:"allowedSources"`
	Affects        string    `json:"affects"`
	EvaluatorPhase uint16    `json:"evaluatorPhase"`
}

type ASTNode struct {
	Kind        string    `json:"kind"`
	Operator    string    `json:"operator,omitempty"`
	Quantity    *Quantity `json:"quantity,omitempty"`
	ParameterID string    `json:"parameterId,omitempty"`
	Left        *ASTNode  `json:"left,omitempty"`
	Right       *ASTNode  `json:"right,omitempty"`
	Dimension   Dimension `json:"dimension"`
}

type TypedExpression struct {
	SourceText            string          `json:"sourceText"`
	CheckedAST            ASTNode         `json:"checkedAst"`
	LanguageVersion       string          `json:"languageVersion"`
	ResultType            ValueType       `json:"resultType"`
	ResultDimension       Dimension       `json:"resultDimension"`
	Reads                 []DependencyKey `json:"reads"`
	FunctionCatalogDigest string          `json:"functionCatalogDigest"`
	Cost                  uint32          `json:"cost"`
}

type ParameterBinding struct {
	ParameterID string
	Dimension   Dimension
}

func CompileExpression(source string, names map[string]ParameterBinding, expected Dimension) (TypedExpression, error) {
	if len(source) == 0 || len(source) > 4096 {
		return TypedExpression{}, fmt.Errorf("%w: source length", ErrExpression)
	}
	parser := expressionParser{names: names, reads: map[DependencyKey]struct{}{}}
	parser.scanner.Init(strings.NewReader(source))
	parser.scanner.Mode = scanner.ScanIdents | scanner.ScanFloats | scanner.ScanInts | scanner.SkipComments
	parser.next()
	node, err := parser.parseExpression()
	if err != nil {
		return TypedExpression{}, err
	}
	if parser.token != scanner.EOF {
		return TypedExpression{}, fmt.Errorf("%w: unexpected token %q", ErrExpression, parser.text)
	}
	if parser.cost > 256 {
		return TypedExpression{}, fmt.Errorf("%w: expression cost exceeds 256 nodes", ErrExpression)
	}
	if !node.Dimension.Equal(expected) {
		return TypedExpression{}, fmt.Errorf("%w: expression dimension %+v, expected %+v", ErrUnitMismatch, node.Dimension, expected)
	}
	reads := make([]DependencyKey, 0, len(parser.reads))
	for read := range parser.reads {
		reads = append(reads, read)
	}
	sortDependencyKeys(reads)
	digest := sha256.Sum256([]byte("occccad-expression-functions-v1:none"))
	return TypedExpression{SourceText: source, CheckedAST: *node, LanguageVersion: "1",
		ResultType: ValueQuantity, ResultDimension: node.Dimension, Reads: reads,
		FunctionCatalogDigest: hex.EncodeToString(digest[:]), Cost: parser.cost}, nil
}

func EvaluateExpression(expression TypedExpression, values map[string]Quantity) (Quantity, error) {
	return evaluateAST(&expression.CheckedAST, values)
}

func evaluateAST(node *ASTNode, values map[string]Quantity) (Quantity, error) {
	if node == nil {
		return Quantity{}, fmt.Errorf("%w: missing AST node", ErrExpression)
	}
	switch node.Kind {
	case "LITERAL":
		return *node.Quantity, nil
	case "PARAMETER":
		value, exists := values[node.ParameterID]
		if !exists {
			return Quantity{}, fmt.Errorf("%w: parameter %s has no value", ErrExpression, node.ParameterID)
		}
		if !value.Dimension.Equal(node.Dimension) {
			return Quantity{}, fmt.Errorf("%w: parameter %s dimension changed", ErrUnitMismatch, node.ParameterID)
		}
		return value, nil
	case "BINARY":
		left, err := evaluateAST(node.Left, values)
		if err != nil {
			return Quantity{}, err
		}
		right, err := evaluateAST(node.Right, values)
		if err != nil {
			return Quantity{}, err
		}
		var value float64
		switch node.Operator {
		case "+":
			value = left.SIValue + right.SIValue
		case "-":
			value = left.SIValue - right.SIValue
		case "*":
			value = left.SIValue * right.SIValue
		case "/":
			if right.SIValue == 0 {
				return Quantity{}, fmt.Errorf("%w: division by zero", ErrExpression)
			}
			value = left.SIValue / right.SIValue
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return Quantity{}, fmt.Errorf("%w: non-finite result", ErrExpression)
		}
		return Quantity{SIValue: value, Dimension: node.Dimension}, nil
	default:
		return Quantity{}, fmt.Errorf("%w: unsupported AST node %s", ErrExpression, node.Kind)
	}
}

type expressionParser struct {
	scanner scanner.Scanner
	token   rune
	text    string
	names   map[string]ParameterBinding
	reads   map[DependencyKey]struct{}
	cost    uint32
}

func (parser *expressionParser) next() {
	parser.token = parser.scanner.Scan()
	parser.text = parser.scanner.TokenText()
}
func (parser *expressionParser) parseExpression() (*ASTNode, error) {
	left, err := parser.parseTerm()
	if err != nil {
		return nil, err
	}
	for parser.token == '+' || parser.token == '-' {
		op := parser.text
		parser.next()
		right, err := parser.parseTerm()
		if err != nil {
			return nil, err
		}
		if !left.Dimension.Equal(right.Dimension) {
			return nil, fmt.Errorf("%w: %s requires equal dimensions", ErrUnitMismatch, op)
		}
		parser.cost++
		left = &ASTNode{Kind: "BINARY", Operator: op, Left: left, Right: right, Dimension: left.Dimension}
	}
	return left, nil
}
func (parser *expressionParser) parseTerm() (*ASTNode, error) {
	left, err := parser.parsePrimary()
	if err != nil {
		return nil, err
	}
	for parser.token == '*' || parser.token == '/' {
		op := parser.text
		parser.next()
		right, err := parser.parsePrimary()
		if err != nil {
			return nil, err
		}
		dimension := left.Dimension.Add(right.Dimension)
		if op == "/" {
			dimension = left.Dimension.Sub(right.Dimension)
		}
		parser.cost++
		left = &ASTNode{Kind: "BINARY", Operator: op, Left: left, Right: right, Dimension: dimension}
	}
	return left, nil
}
func (parser *expressionParser) parsePrimary() (*ASTNode, error) {
	if parser.token == '(' {
		parser.next()
		node, err := parser.parseExpression()
		if err != nil {
			return nil, err
		}
		if parser.token != ')' {
			return nil, fmt.Errorf("%w: missing )", ErrExpression)
		}
		parser.next()
		return node, nil
	}
	if parser.token == scanner.Int || parser.token == scanner.Float {
		value, err := strconv.ParseFloat(parser.text, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid number", ErrExpression)
		}
		parser.next()
		unit := ""
		if parser.token == scanner.Ident {
			if _, exists := units[strings.ToLower(parser.text)]; exists {
				unit = parser.text
				parser.next()
			}
		}
		quantity, err := NewQuantity(value, unit)
		if err != nil {
			return nil, err
		}
		parser.cost++
		return &ASTNode{Kind: "LITERAL", Quantity: &quantity, Dimension: quantity.Dimension}, nil
	}
	if parser.token == scanner.Ident {
		name := parser.text
		binding, exists := parser.names[name]
		if !exists {
			return nil, fmt.Errorf("%w: unknown parameter %q", ErrExpression, name)
		}
		parser.next()
		key := DependencyKey("parameter:" + binding.ParameterID)
		parser.reads[key] = struct{}{}
		parser.cost++
		return &ASTNode{Kind: "PARAMETER", ParameterID: binding.ParameterID, Dimension: binding.Dimension}, nil
	}
	return nil, fmt.Errorf("%w: unexpected token %q", ErrExpression, parser.text)
}
