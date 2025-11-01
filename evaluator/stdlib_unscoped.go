package evaluator

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zautumnz/keai/object"
	"github.com/zautumnz/keai/utils"
)

// These stdlib functions aren't scoped/namespaced

// panic
func panicFn(args ...OBJ) OBJ {
	switch e := args[0].(type) {
	case *object.Error:
		c := 1
		fmt.Println(e.Message)
		if e.Code != nil {
			c = int(*e.Code)
		}
		utils.ExitConditionally(c)
	default:
		return NewError("panic expected an error!")
	}
	return NULL
}

// error
func errorFn(args ...OBJ) OBJ {
	if len(args) != 1 {
		return NewError("wrong number of arguments. got=%d, want=1",
			len(args))
	}
	switch t := args[0].(type) {
	case *object.String:
		return &object.Error{Message: t.Value, BuiltinCall: true}
	case *object.Hash:
		msgStr := &object.String{Value: "message"}
		codeStr := &object.String{Value: "code"}
		dataStr := &object.String{Value: "data"}
		msg := t.Pairs[msgStr.HashKey()].Value
		code := t.Pairs[codeStr.HashKey()].Value
		data := t.Pairs[dataStr.HashKey()].Value
		e := &object.Error{BuiltinCall: true}
		if msg != nil {
			switch m := msg.(type) {
			case *object.String:
				e.Message = m.Value
			default:
				return NewError("error.message should be string!")
			}
		}
		if code != nil {
			switch c := code.(type) {
			case *object.Integer:
				cc := int(c.Value)
				e.Code = &cc
			default:
				return NewError("error.code should be integer!")
			}
		}
		if data != nil {
			e.Data = data.JSON(false)
		}
		return e
	default:
		return NewError("error() expected a string or hash!")
	}
}

// output a string to stdout
func printFn(args ...OBJ) OBJ {
	for _, arg := range args {
		var e error
		s := arg.Inspect()

		if arg.Type() == object.STRING_OBJ {
			if strings.Contains(s, "\\") {
				orig := s
				// double escape;
				// used for ansi escape codes and some other things
				s, e = strconv.Unquote(`"` + s + `"`)
				if e != nil {
					// this happens sometimes when working on things like
					// nested json, so we just use the original string instead
					fmt.Print(orig + " ")
				}
				fmt.Println(s + " ")
				return NULL
			}
		}

		fmt.Print(s + " ")
	}

	fmt.Println()
	return NULL
}

func init() {
	RegisterBuiltin("print",
		func(env *ENV, args ...OBJ) OBJ {
			return printFn(args...)
		})
	RegisterBuiltin("error",
		func(env *ENV, args ...OBJ) OBJ {
			return errorFn(args...)
		})
	RegisterBuiltin("panic",
		func(env *ENV, args ...OBJ) OBJ {
			return panicFn(args...)
		})
}
