// Package evaluator contains the core of our interpreter, which walks
// the AST produced by the parser and evaluates the user-submitted program.
package evaluator

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/zautumnz/keai/ast"
	"github.com/zautumnz/keai/lexer"
	"github.com/zautumnz/keai/object"
	"github.com/zautumnz/keai/parser"
	"github.com/zautumnz/keai/utils"
)

// pre-defined objects
var (
	NULL  = &object.Null{}
	TRUE  = &object.Boolean{Value: true}
	FALSE = &object.Boolean{Value: false}
	CTX   = context.Background()
)

// OBJ is a type alias to save some typing
type OBJ = object.Object

// ENV is a type alias to save some typing
type ENV = object.Environment

// The built-in functions / standard-library methods are stored here.
var builtins = map[string]*object.Builtin{}

// Eval is our core function for evaluating nodes.
func Eval(node ast.Node, env *ENV) OBJ {
	return evalContext(context.Background(), node, env)
}

// evalContext is our core function for evaluating nodes.
// The context.Context provided can be used to cancel a running script instance.
func evalContext(ctx context.Context, node ast.Node, env *ENV) OBJ {
	// We test our context at every iteration of our main-loop.
	select {
	case <-ctx.Done():
		return &object.Error{Message: ctx.Err().Error()}
	default:
		// noop
	}

	switch node := node.(type) {
	//Statements
	case *ast.Program:
		return evalProgram(node, env)
	case *ast.ExpressionStatement:
		return Eval(node.Expression, env)

	//Expressions
	case *ast.IntegerLiteral:
		return &object.Integer{Value: node.Value}
	case *ast.FloatLiteral:
		return &object.Float{Value: node.Value}
	case *ast.Boolean:
		return nativeBoolToBooleanObject(node.Value)
	case *ast.NullLiteral:
		return NULL
	case *ast.PrefixExpression:
		right := Eval(node.Right, env)
		if isError(right) {
			return right
		}
		return evalPrefixExpression(node.Operator, right)
	case *ast.PostfixExpression:
		return evalPostfixExpression(env, node.Operator, node)
	case *ast.InfixExpression:
		left := Eval(node.Left, env)
		if isError(left) {
			return left
		}
		right := Eval(node.Right, env)
		if isError(right) {
			return right
		}
		res := evalInfixExpression(node.Operator, left, right, env)
		if isError(res) {
			fmt.Printf("Error: %s\n", res.Inspect())
			utils.ExitConditionally(1)
		}
		return res

	case *ast.BlockStatement:
		return evalBlockStatement(node, env)
	case *ast.IfExpression:
		return evalIfExpression(node, env)
	case *ast.ImportExpression:
		return evalImportExpression(node, env)
	case *ast.ForLoopExpression:
		return evalForLoopExpression(node, env)
	case *ast.ForeachStatement:
		return evalForeachExpression(node, env)
	case *ast.ReturnStatement:
		val := Eval(node.ReturnValue, env)
		return &object.ReturnValue{Value: val}
	case *ast.MutableStatement:
		val := Eval(node.Value, env)
		env.Set(node.Name.Value, val)
		return val
	case *ast.LetStatement:
		val := Eval(node.Value, env)
		env.SetLet(node.Name.Value, val)
		return val
	case *ast.Identifier:
		return evalIdentifier(node, env)
	case *ast.FunctionLiteral:
		params := node.Parameters
		body := node.Body
		defaults := node.Defaults
		docstring := node.DocString
		return &object.Function{
			Parameters: params,
			Env:        env,
			Body:       body,
			Defaults:   defaults,
			DocString:  docstring,
		}
	case *ast.CallExpression:
		function := Eval(node.Function, env)
		if isError(function) {
			return function
		}

		args := evalExpression(node.Arguments, env)

		// check for current args (...)
		if len(args) > 0 {
			firstArg, ok := args[0].(*object.Array)
			if ok && firstArg.IsCurrentArgs {
				newArgs := env.CurrentArgs
				args = append(newArgs, args[1:]...)
			}
		}

		res := ApplyFunction(env, function, args)

		switch t := res.(type) {
		case *object.Error:
			c := 1
			if t.Code != nil {
				c = int(*t.Code)
			}
			if !t.BuiltinCall {
				fmt.Fprintf(
					os.Stderr,
					"Error calling `%s` : %s\n",
					node.Function,
					res.Inspect(),
				)
				utils.ExitConditionally(c)
			}
		}

		return res

	case *ast.ArrayLiteral:
		elements := evalExpression(node.Elements, env)
		if len(elements) == 1 && isError(elements[0]) {
			return elements[0]
		}
		return &object.Array{Elements: elements}
	case *ast.StringLiteral:
		return &object.String{Value: Interpolate(node.Value, env)}
	case *ast.SpreadLiteral:
		return evalSpread(node, env)
	case *ast.CurrentArgsLiteral:
		return &object.Array{
			Token:         node.Token,
			Elements:      env.CurrentArgs,
			IsCurrentArgs: true,
		}
	case *ast.IndexExpression:
		left := Eval(node.Left, env)
		if isError(left) {
			return left
		}
		index := Eval(node.Index, env)
		if isError(index) {
			return index
		}
		return evalIndexExpression(left, index, env)
	case *ast.AssignStatement:
		return evalAssignStatement(node, env)
	case *ast.HashLiteral:
		return evalHashLiteral(node, env)
	}
	return nil
}

// eval block statement
func evalBlockStatement(block *ast.BlockStatement, env *ENV) OBJ {
	var result OBJ
	for _, statement := range block.Statements {
		result = Eval(statement, env)
		if result != nil {
			rt := result.Type()
			if rt == object.RETURN_VALUE_OBJ {
				return result
			}
		}
	}
	return result
}

// EvalModule evaluates the named module and returns a *object.Module object
// This creates a whole new keai instance (lexer, parser, env, and evaluator),
// which isn't ideal, but we also do this when working with string
// interpolation.
func EvalModule(name string) OBJ {
	filename := FindModule(name)
	if filename == "" {
		return NewError("ImportError: no module named '%s'", name)
	}

	b, err := os.ReadFile(filename)
	if err != nil {
		return NewError("IOError: error reading module '%s': %s", name, err)
	}

	l := lexer.New(string(b))
	p := parser.New(l)

	module := p.ParseProgram()
	if len(p.Errors()) != 0 {
		return NewError("ParseError: %s", p.Errors())
	}

	env := object.NewEnvironment()
	Eval(module, env)

	return env.ExportedHash()
}

var importCache map[string]OBJ

func init() {
	importCache = make(map[string]OBJ)
}

func evalImportExpression(ie *ast.ImportExpression, env *ENV) OBJ {
	// treat modules as singletons;
	// we don't allow modifying anythig exported by modules, but this
	// means we can skip re-evaling modules on subsequent imports
	ev, ok := importCache[ie.Name.String()]
	if ok {
		return ev
	}

	name := Eval(ie.Name, env)
	if isError(name) {
		return name
	}

	if s, ok := name.(*object.String); ok {
		attrs := EvalModule(s.Value)
		if isError(attrs) {
			return attrs
		}

		m := &object.Module{Name: s.Value, Attrs: attrs}
		importCache[ie.Name.String()] = m
		return m
	}

	return NewError("ImportError: invalid import path '%s'", name)
}

// for performance, using single instance of boolean
func nativeBoolToBooleanObject(input bool) *object.Boolean {
	if input {
		return TRUE
	}
	return FALSE
}

// eval prefix expression
func evalPrefixExpression(operator string, right OBJ) OBJ {
	switch operator {
	case "!":
		return evalBangOperatorExpression(right)
	case "-":
		return evalMinusPrefixOperatorExpression(right)
	case "~":
		return evalNotPrefixOperatorExpression(right)
	default:
		return NewError("unknown operator: %s%s", operator, right.Type())
	}
}

func evalPostfixExpression(
	env *ENV,
	operator string,
	node *ast.PostfixExpression,
) OBJ {
	switch operator {
	case "++":
		val, ok := env.Get(node.Token.Literal)
		if !ok {
			return NewError("%s is unknown", node.Token.Literal)
		}

		switch arg := val.(type) {
		case *object.Integer:
			v := arg.Value
			env.Set(node.Token.Literal, &object.Integer{Value: v + 1})
			return arg
		default:
			return NewError("%s is not an int", node.Token.Literal)
		}
	case "--":
		val, ok := env.Get(node.Token.Literal)
		if !ok {
			return NewError("%s is unknown", node.Token.Literal)
		}

		switch arg := val.(type) {
		case *object.Integer:
			v := arg.Value
			env.Set(node.Token.Literal, &object.Integer{Value: v - 1})
			return arg
		default:
			return NewError("%s is not an int", node.Token.Literal)
		}

	default:
		return NewError("unknown operator: %s", operator)
	}
}

func evalBangOperatorExpression(right OBJ) OBJ {
	switch right {
	case TRUE:
		return FALSE
	case FALSE:
		return TRUE
	case NULL:
		return TRUE
	default:
		return FALSE
	}
}

func evalMinusPrefixOperatorExpression(right OBJ) OBJ {
	switch obj := right.(type) {
	case *object.Integer:
		return &object.Integer{Value: -obj.Value}
	case *object.Float:
		return &object.Float{Value: -obj.Value}
	default:
		return NewError("unknown operator: -%s", right.Type())
	}
}

func evalNotPrefixOperatorExpression(right OBJ) OBJ {
	if right.Type() != object.INTEGER_OBJ {
		return NewError("expected integer, got %s", right.Type())
	}
	value := right.(*object.Integer).Value
	return &object.Integer{Value: ^value}
}

func evalInfixExpression(operator string, left, right OBJ, env *ENV) OBJ {
	switch {
	case left.Type() == object.INTEGER_OBJ && right.Type() == object.INTEGER_OBJ:
		return evalIntegerInfixExpression(operator, left, right)
	case left.Type() == object.FLOAT_OBJ && right.Type() == object.FLOAT_OBJ:
		return evalFloatInfixExpression(operator, left, right)
	case left.Type() == object.FLOAT_OBJ && right.Type() == object.INTEGER_OBJ:
		return evalFloatIntegerInfixExpression(operator, left, right)
	case left.Type() == object.INTEGER_OBJ && right.Type() == object.FLOAT_OBJ:
		return evalIntegerFloatInfixExpression(operator, left, right)
	case left.Type() == object.STRING_OBJ && right.Type() == object.STRING_OBJ:
		return evalStringInfixExpression(operator, left, right)
	case operator == "&&":
		return nativeBoolToBooleanObject(
			objectToNativeBoolean(left) && objectToNativeBoolean(right),
		)
	case operator == "||":
		return nativeBoolToBooleanObject(
			objectToNativeBoolean(left) || objectToNativeBoolean(right),
		)
	case operator == "==":
		return nativeBoolToBooleanObject(left == right)
	case operator == "!=":
		return nativeBoolToBooleanObject(left != right)
	case left.Type() == object.BOOLEAN_OBJ && right.Type() == object.BOOLEAN_OBJ:
		return evalBooleanInfixExpression(operator, left, right)
	case left.Type() != right.Type():
		return NewError("type mismatch: %s %s %s",
			left.Type(), operator, right.Type())
	default:
		return NewError("unknown operator: %s %s %s",
			left.Type(), operator, right.Type())
	}
}

// boolean operations
func evalBooleanInfixExpression(operator string, left, right OBJ) OBJ {
	// convert the bools to strings.
	l := &object.String{Value: string(left.Inspect())}
	r := &object.String{Value: string(right.Inspect())}

	switch operator {
	case "<":
		return evalStringInfixExpression(operator, l, r)
	case "<=":
		return evalStringInfixExpression(operator, l, r)
	case ">":
		return evalStringInfixExpression(operator, l, r)
	case ">=":
		return evalStringInfixExpression(operator, l, r)
	default:
		return NewError("unknown operator: %s %s %s",
			left.Type(), operator, right.Type())
	}
}

func evalIntegerInfixExpression(operator string, left, right OBJ) OBJ {
	leftVal := left.(*object.Integer).Value
	rightVal := right.(*object.Integer).Value
	switch operator {
	case "+":
		return &object.Integer{Value: leftVal + rightVal}
	case "+=":
		return &object.Integer{Value: leftVal + rightVal}
	case "%":
		return &object.Integer{Value: leftVal % rightVal}
	case "**":
		return &object.Integer{
			Value: int64(math.Pow(float64(leftVal), float64(rightVal))),
		}
	case "-":
		return &object.Integer{Value: leftVal - rightVal}
	case "-=":
		return &object.Integer{Value: leftVal - rightVal}
	case "*":
		return &object.Integer{Value: leftVal * rightVal}
	case "*=":
		return &object.Integer{Value: leftVal * rightVal}
	case "/":
		return &object.Integer{Value: leftVal / rightVal}
	case "/=":
		return &object.Integer{Value: leftVal / rightVal}
	case "<":
		return nativeBoolToBooleanObject(leftVal < rightVal)
	case "<=":
		return nativeBoolToBooleanObject(leftVal <= rightVal)
	case ">":
		return nativeBoolToBooleanObject(leftVal > rightVal)
	case ">=":
		return nativeBoolToBooleanObject(leftVal >= rightVal)
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	case "|":
		return &object.Integer{Value: leftVal | rightVal}
	case "^":
		return &object.Integer{Value: leftVal ^ rightVal}
	case "&":
		return &object.Integer{Value: leftVal & rightVal}
	case "<<":
		return &object.Integer{Value: leftVal << uint64(rightVal)}
	case ">>":
		return &object.Integer{Value: leftVal >> uint64(rightVal)}

	case "..":
		len := int(rightVal-leftVal) + 1
		array := make([]OBJ, len)
		i := 0
		for i < len {
			array[i] = &object.Integer{Value: leftVal}
			leftVal++
			i++
		}
		return &object.Array{Elements: array}
	default:
		return NewError("unknown operator: %s %s %s",
			left.Type(), operator, right.Type())
	}
}

func evalFloatInfixExpression(operator string, left, right OBJ) OBJ {
	leftVal := left.(*object.Float).Value
	rightVal := right.(*object.Float).Value
	switch operator {
	case "+":
		return &object.Float{Value: leftVal + rightVal}
	case "+=":
		return &object.Float{Value: leftVal + rightVal}
	case "-":
		return &object.Float{Value: leftVal - rightVal}
	case "-=":
		return &object.Float{Value: leftVal - rightVal}
	case "*":
		return &object.Float{Value: leftVal * rightVal}
	case "*=":
		return &object.Float{Value: leftVal * rightVal}
	case "**":
		return &object.Float{Value: math.Pow(leftVal, rightVal)}
	case "/":
		return &object.Float{Value: leftVal / rightVal}
	case "/=":
		return &object.Float{Value: leftVal / rightVal}
	case "<":
		return nativeBoolToBooleanObject(leftVal < rightVal)
	case "<=":
		return nativeBoolToBooleanObject(leftVal <= rightVal)
	case ">":
		return nativeBoolToBooleanObject(leftVal > rightVal)
	case ">=":
		return nativeBoolToBooleanObject(leftVal >= rightVal)
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return NewError("unknown operator: %s %s %s",
			left.Type(), operator, right.Type())
	}
}

func evalFloatIntegerInfixExpression(operator string, left, right OBJ) OBJ {
	leftVal := left.(*object.Float).Value
	rightVal := float64(right.(*object.Integer).Value)
	switch operator {
	case "+":
		return &object.Float{Value: leftVal + rightVal}
	case "+=":
		return &object.Float{Value: leftVal + rightVal}
	case "-":
		return &object.Float{Value: leftVal - rightVal}
	case "-=":
		return &object.Float{Value: leftVal - rightVal}
	case "*":
		return &object.Float{Value: leftVal * rightVal}
	case "*=":
		return &object.Float{Value: leftVal * rightVal}
	case "**":
		return &object.Float{Value: math.Pow(leftVal, rightVal)}
	case "/":
		return &object.Float{Value: leftVal / rightVal}
	case "/=":
		return &object.Float{Value: leftVal / rightVal}
	case "<":
		return nativeBoolToBooleanObject(leftVal < rightVal)
	case "<=":
		return nativeBoolToBooleanObject(leftVal <= rightVal)
	case ">":
		return nativeBoolToBooleanObject(leftVal > rightVal)
	case ">=":
		return nativeBoolToBooleanObject(leftVal >= rightVal)
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return NewError("unknown operator: %s %s %s",
			left.Type(), operator, right.Type())
	}
}

func evalIntegerFloatInfixExpression(operator string, left, right OBJ) OBJ {
	leftVal := float64(left.(*object.Integer).Value)
	rightVal := right.(*object.Float).Value
	switch operator {
	case "+":
		return &object.Float{Value: leftVal + rightVal}
	case "+=":
		return &object.Float{Value: leftVal + rightVal}
	case "-":
		return &object.Float{Value: leftVal - rightVal}
	case "-=":
		return &object.Float{Value: leftVal - rightVal}
	case "*":
		return &object.Float{Value: leftVal * rightVal}
	case "*=":
		return &object.Float{Value: leftVal * rightVal}
	case "**":
		return &object.Float{Value: math.Pow(leftVal, rightVal)}
	case "/":
		return &object.Float{Value: leftVal / rightVal}
	case "/=":
		return &object.Float{Value: leftVal / rightVal}
	case "<":
		return nativeBoolToBooleanObject(leftVal < rightVal)
	case "<=":
		return nativeBoolToBooleanObject(leftVal <= rightVal)
	case ">":
		return nativeBoolToBooleanObject(leftVal > rightVal)
	case ">=":
		return nativeBoolToBooleanObject(leftVal >= rightVal)
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return NewError("unknown operator: %s %s %s",
			left.Type(), operator, right.Type())
	}
}

func evalStringInfixExpression(operator string, left, right OBJ) OBJ {
	l := left.(*object.String)
	r := right.(*object.String)

	switch operator {
	case "==":
		return nativeBoolToBooleanObject(l.Value == r.Value)
	case "!=":
		return nativeBoolToBooleanObject(l.Value != r.Value)
	case ">=":
		return nativeBoolToBooleanObject(l.Value >= r.Value)
	case ">":
		return nativeBoolToBooleanObject(l.Value > r.Value)
	case "<=":
		return nativeBoolToBooleanObject(l.Value <= r.Value)
	case "<":
		return nativeBoolToBooleanObject(l.Value < r.Value)
	case "+":
		return &object.String{Value: l.Value + r.Value}
	case "+=":
		return &object.String{Value: l.Value + r.Value}
	}

	return NewError("unknown operator: %s %s %s",
		left.Type(), operator, right.Type())
}

// evalIfExpression handles an `if` expression, running the block
// if the condition matches, and running any optional else block
// otherwise.
func evalIfExpression(ie *ast.IfExpression, env *ENV) OBJ {
	condition := Eval(ie.Condition, env)
	if isError(condition) {
		return condition
	}
	if isTruthy(condition) {
		return Eval(ie.Consequence, env)
	}
	if ie.Alternative != nil {
		return Eval(ie.Alternative, env)
	}
	return NULL
}

func evalAssignStatement(a *ast.AssignStatement, env *ENV) (val OBJ) {
	evaluated := Eval(a.Value, env)
	if isError(evaluated) {
		return evaluated
	}

	// An assignment is generally:
	//    variable = value
	// But we cheat and reuse the implementation for:
	//    i += 4
	// In this case we record the "operator" as "+="
	switch a.Operator {
	case "+=":
		// Get the current value
		current, ok := env.Get(a.Name.String())
		if !ok {
			return NewError("%s is unknown", a.Name.String())
		}

		res := evalInfixExpression("+=", current, evaluated, env)
		if isError(res) {
			fmt.Printf("Error handling += %s\n", res.Inspect())
			return res
		}

		env.Set(a.Name.String(), res)
		return res

	case "-=":
		// Get the current value
		current, ok := env.Get(a.Name.String())
		if !ok {
			return NewError("%s is unknown", a.Name.String())
		}

		res := evalInfixExpression("-=", current, evaluated, env)
		if isError(res) {
			fmt.Printf("Error handling -= %s\n", res.Inspect())
			return res
		}

		env.Set(a.Name.String(), res)
		return res

	case "*=":
		// Get the current value
		current, ok := env.Get(a.Name.String())
		if !ok {
			return NewError("%s is unknown", a.Name.String())
		}

		res := evalInfixExpression("*=", current, evaluated, env)
		if isError(res) {
			fmt.Printf("Error handling *= %s\n", res.Inspect())
			return res
		}

		env.Set(a.Name.String(), res)
		return res

	case "/=":
		// Get the current value
		current, ok := env.Get(a.Name.String())
		if !ok {
			return NewError("%s is unknown", a.Name.String())
		}

		res := evalInfixExpression("/=", current, evaluated, env)
		if isError(res) {
			fmt.Printf("Error handling /= %s\n", res.Inspect())
			return res
		}

		env.Set(a.Name.String(), res)
		return res

	case "=":
		_, ok := env.Get(a.Name.String())
		if !ok {
			fmt.Printf("Setting unknown variable '%s' is an error!\n", a.Name.String())
			utils.ExitConditionally(1)
		}

		env.Set(a.Name.String(), evaluated)
	}

	return evaluated
}

func evalForLoopExpression(fle *ast.ForLoopExpression, env *ENV) OBJ {
	rt := TRUE
	for {
		condition := Eval(fle.Condition, env)
		if isError(condition) {
			return condition
		}
		if isTruthy(condition) {
			rt := Eval(fle.Consequence, env)
			if !isError(rt) &&
				(rt.Type() == object.RETURN_VALUE_OBJ || rt.Type() == object.ERROR_OBJ) {
				return rt
			}
		} else {
			break
		}
	}
	return rt
}

// handle "foreach x [,y] in .."
func evalForeachExpression(fle *ast.ForeachStatement, env *ENV) OBJ {
	// expression
	val := Eval(fle.Value, env)

	helper, ok := val.(object.Iterable)
	if !ok {
		return NewError(
			"%s object doesn't implement the Iterable interface",
			val.Type(),
		)
	}

	// The one/two values we're going to permit
	var permit []string
	permit = append(permit, fle.Ident)
	if fle.Index != "" {
		permit = append(permit, fle.Index)
	}

	// Create a new environment for the block
	// This will allow writing EVERYTHING to the parent scope,
	// except the two variables named in the permit-array
	child := object.NewTemporaryScope(env, permit)

	// Reset the state of any previous iteration.
	helper.Reset()

	// Get the initial values.
	ret, idx, ok := helper.Next()

	for ok {
		// Set the index + name
		child.Set(fle.Ident, ret)

		idxName := fle.Index
		if idxName != "" {
			child.Set(fle.Index, idx)
		}

		// Eval the block
		rt := Eval(fle.Body, child)

		// If we got an error/return then we handle it.
		if !isError(rt) &&
			(rt.Type() == object.RETURN_VALUE_OBJ ||
				rt.Type() == object.ERROR_OBJ) {
			return rt
		}

		// Loop again
		ret, idx, ok = helper.Next()
	}

	return NULL
}

func isTruthy(obj OBJ) bool {
	switch obj {
	case TRUE:
		return true
	case FALSE:
		return false
	case NULL:
		return false
	default:
		return true
	}
}

func evalProgram(program *ast.Program, env *ENV) OBJ {
	var result OBJ
	for _, statement := range program.Statements {
		result = Eval(statement, env)
		switch result := result.(type) {
		case *object.ReturnValue:
			return result.Value
		}
	}

	return result
}

func isError(obj OBJ) bool {
	if obj != nil {
		return obj.Type() == object.ERROR_OBJ
	}
	return false
}

func evalIdentifier(node *ast.Identifier, env *ENV) OBJ {
	if val, ok := env.Get(node.Value); ok {
		return val
	}
	if builtin, ok := builtins[node.Value]; ok {
		return builtin
	}
	fmt.Println("identifier not found: " + node.Value)
	utils.ExitConditionally(1)
	return NewError2("identifier not found: " + node.Value)
}

func evalExpression(exps []ast.Expression, env *ENV) []OBJ {
	var result []OBJ
	for _, e := range exps {
		evaluated := Eval(e, env)
		if isError(evaluated) {
			return []OBJ{evaluated}
		}
		result = append(result, evaluated)
	}

	return result
}

// Remove balanced characters around a string.
func trimQuotes(in string, c byte) string {
	if len(in) >= 2 {
		if in[0] == c && in[len(in)-1] == c {
			return in[1 : len(in)-1]
		}
	}
	return in
}

func evalIndexExpression(left, index OBJ, env *ENV) OBJ {
	switch {
	case left.Type() == object.ARRAY_OBJ:
		return evalArrayIndexExpression(left, index, env)
	case left.Type() == object.HASH_OBJ:
		return evalHashIndexExpression(left, index, env)
	case left.Type() == object.STRING_OBJ:
		return evalStringIndexExpression(left, index, env)
	case left.Type() == object.MODULE_OBJ:
		return evalModuleIndexExpression(left, index, env)
	default:
		if fn, ok := objectGetMethod(left, index, env); ok {
			return fn
		}
		return NewError("index operator not support:%s", left.Type())

	}
}

func evalModuleIndexExpression(module, index OBJ, env *ENV) OBJ {
	moduleObject := module.(*object.Module)
	return evalHashIndexExpression(moduleObject.Attrs, index, env)
}

func evalArrayIndexExpression(array, index OBJ, env *ENV) OBJ {
	arrayObject := array.(*object.Array)
	switch t := index.(type) {
	case *object.Integer:
		idx := t.Value
		max := int64(len(arrayObject.Elements) - 1)
		if idx < 0 || idx > max {
			return NULL
		}
		return arrayObject.Elements[idx]
	default:
		if fn, ok := objectGetMethod(array, index, env); ok {
			return fn
		}
		return NULL
	}
}

func evalHashIndexExpression(hash, index OBJ, env *ENV) OBJ {
	hashObject := hash.(*object.Hash)
	key, ok := index.(object.Hashable)
	if !ok {
		return NewError("unusable as hash key: %s", index.Type())
	}
	pair, ok := hashObject.Pairs[key.HashKey()]
	if !ok {
		var fn OBJ
		if fn, ok = objectGetMethod(hash, index, env); ok {
			return fn
		}
		return NULL
	}
	return pair.Value
}

func evalStringIndexExpression(input, index OBJ, env *ENV) OBJ {
	str := input.(*object.String).Value
	switch t := index.(type) {
	case *object.Integer:
		idx := t.Value
		max := int64(len(str))
		if idx < 0 || idx > max {
			return NULL
		}

		// Get the characters as an array of runes
		chars := []rune(str)

		// Now index
		ret := chars[idx]

		// And return as a string.
		return &object.String{Value: string(ret)}
	default:
		if fn, ok := objectGetMethod(input, index, env); ok {
			return fn
		}

		return NULL
	}
}

func evalHashLiteral(node *ast.HashLiteral, env *ENV) OBJ {
	pairs := make(map[object.HashKey]object.HashPair)
	for keyNode, valueNode := range node.Pairs {
		key := Eval(keyNode, env)
		if isError(key) {
			return key
		}
		hashKey, ok := key.(object.Hashable)
		if !ok {
			return NewError("unusable as hash key: %s", key.Type())
		}
		value := Eval(valueNode, env)
		if isError(value) {
			return value
		}
		hashed := hashKey.HashKey()
		pairs[hashed] = object.HashPair{Key: key, Value: value}

	}

	return &object.Hash{Pairs: pairs}
}

// ApplyFunction applies a function in an environment
func ApplyFunction(env *ENV, fn OBJ, args []OBJ) OBJ {
	switch fn := fn.(type) {
	case *object.Function:
		extendEnv := extendFunctionEnv(fn, args)
		evaluated := Eval(fn.Body, extendEnv)
		return upwrapReturnValue(evaluated)
	case *object.Builtin:
		return fn.Fn(env, args...)
	default:
		return NewError("not a function: %s", fn.Type())
	}
}

func extendFunctionEnv(fn *object.Function, args []OBJ) *ENV {
	env := object.NewEnclosedEnvironment(fn.Env, args)

	// Set the defaults
	for key, val := range fn.Defaults {
		env.Set(key, Eval(val, env))
	}
	for paramIdx, param := range fn.Parameters {
		if paramIdx < len(args) {
			env.Set(param.Value, args[paramIdx])
		}
	}
	return env
}

func upwrapReturnValue(obj OBJ) OBJ {
	if returnValue, ok := obj.(*object.ReturnValue); ok {
		return returnValue.Value
	}
	return obj
}

// RegisterBuiltin registers a built-in function. This is used to register
// our "standard library" functions.
func RegisterBuiltin(name string, fn object.BuiltinFunction) {
	builtins[name] = &object.Builtin{Fn: fn}
}

func objectGetMethod(o, key OBJ, env *ENV) (ret OBJ, ok bool) {
	switch k := key.(type) {
	case *object.String:
		var fn object.BuiltinFunction
		if fn = o.GetMethod(k.Value); fn != nil {
			return &object.Builtin{Fn: fn}, true
		}

		// If we reach this point then the invokation didn't
		// succeed, that probably means that the function wasn't
		// implemented in go.
		// So now we want to look for it in keai, and we have
		// enough details to find the appropriate function.
		//  * We have the object involved.
		//  * We have the type of that object.
		//  * We have the name of the function.
		//  * We have the arguments.
		//
		// We'll use the type + name to lookup the (global) function
		// to invoke. For example in this case we'll invoke
		// `string.foo()` - because the type of the object we're
		// invoking-against is string:
		//  "autumn".foo();
		// For this case we'll be looking for `array.foo()`.
		//   let a = [ 1, 2, 3 ];
		//   print(a.foo());
		// As a final fall-back we'll look for "object.foo()"
		// if "array.foo()" isn't defined.
		attempts := []string{}
		if _, ok = object.SystemTypesMap[o.Type()]; ok {
			attempts = append(attempts, strings.ToLower(string(o.Type())))
		} else {
			attempts = append(attempts, string(o.Type()))
		}
		attempts = append(attempts, "object")

		// Look for "$type.name", or "object.name"
		for _, prefix := range attempts {
			// What we're attempting to execute.
			name := prefix + "." + k.Value

			// Try to find that function in our environment.
			if val, ok := env.Get(name); ok {
				if fn, ok := val.(*object.Function); ok {
					copyFn := *fn
					emptyArgs := make([]OBJ, 0)
					copyFn.Env = object.NewEnclosedEnvironment(fn.Env, emptyArgs)
					copyFn.Env.Set("self", o)
					return &copyFn, true
				}
				return val, true
			}
		}
	}
	return nil, false
}

func objectToNativeBoolean(o OBJ) bool {
	if r, ok := o.(*object.ReturnValue); ok {
		o = r.Value
	}
	switch obj := o.(type) {
	case *object.Boolean:
		return obj.Value
	case *object.String:
		return obj.Value != ""
	case *object.Null:
		return false
	case *object.Integer:
		if obj.Value == 0 {
			return false
		}
		return true
	case *object.Float:
		if obj.Value == 0.0 {
			return false
		}
		return true
	case *object.Array:
		if len(obj.Elements) == 0 {
			return false
		}
		return true
	case *object.Hash:
		if len(obj.Pairs) == 0 {
			return false
		}
		return true
	default:
		return true
	}
}

func evalSpread(node ast.Node, env *ENV) OBJ {
	switch n := node.(type) {
	case *ast.SpreadLiteral:
		a := n.Right.TokenLiteral()
		val, ok := env.Get(a)
		if !ok {
			return NewError("%s is unknown", a)
		}

		switch ao := val.(type) {
		case *object.Array:
			return &object.Array{Elements: ao.Elements, IsCurrentArgs: true}
		default:
			return NewError("spread expected an array, got %s", ao.Type())
		}
	}

	return NULL
}
