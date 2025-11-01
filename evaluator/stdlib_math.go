package evaluator

import (
	"math"
	"math/rand"
	"time"

	"github.com/zautumnz/keai/object"
)

func mathAbs(args ...OBJ) OBJ {
	if len(args) != 1 {
		return NewError("wrong number of arguments. got=%d, want=1",
			len(args))
	}

	switch arg := args[0].(type) {
	case *object.Integer:
		v := arg.Value
		if v < 0 {
			v = v * -1
		}
		return &object.Integer{Value: v}
	case *object.Float:
		v := arg.Value
		if v < 0 {
			v = v * -1
		}
		return &object.Float{Value: v}
	default:
		return NewError("argument to `math.abs` not supported, got=%s",
			args[0].Type())
	}
}

// val = math.rand()
func mathRandom(args ...OBJ) OBJ {
	return &object.Float{Value: rand.Float64()}
}

// val = math.sqrt(int);
func mathSqrt(args ...OBJ) OBJ {
	if len(args) != 1 {
		return NewError("wrong number of arguments. got=%d, want=1",
			len(args))
	}

	switch arg := args[0].(type) {
	case *object.Integer:
		v := arg.Value
		return &object.Float{Value: math.Sqrt(float64(v))}
	case *object.Float:
		v := arg.Value
		return &object.Float{Value: math.Sqrt(v)}
	default:
		return NewError("argument to `math.sqrt` not supported, got=%s",
			args[0].Type())
	}
}

func init() {
	// Setup our random seed.
	rand.New(rand.NewSource(time.Now().UnixNano()))
	RegisterBuiltin("math.abs",
		func(env *ENV, args ...OBJ) OBJ {
			return mathAbs(args...)
		})
	RegisterBuiltin("math.rand",
		func(env *ENV, args ...OBJ) OBJ {
			return mathRandom(args...)
		})
	RegisterBuiltin("math.sqrt",
		func(env *ENV, args ...OBJ) OBJ {
			return mathSqrt(args...)
		})
}
