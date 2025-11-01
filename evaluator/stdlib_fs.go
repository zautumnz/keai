package evaluator

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/zautumnz/keai/object"
)

// array = fs.glob("/etc/*.conf")
func fsGlob(args ...OBJ) OBJ {
	if len(args) != 1 {
		return NewError("wrong number of arguments. got=%d, want=1",
			len(args))
	}
	pattern := args[0].(*object.String).Value

	entries, err := filepath.Glob(pattern)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	// Create an array to hold the results and populate it
	l := len(entries)
	result := make([]OBJ, l)
	for i, txt := range entries {
		result[i] = &object.String{Value: txt}
	}
	return &object.Array{Elements: result}
}

// Change a mode of a file - note the second argument is a string
// to emphasise octal.
func chmodFn(args ...OBJ) OBJ {
	if len(args) != 2 {
		return NewError("wrong number of arguments. got=%d, want=2",
			len(args))
	}

	path := args[0].Inspect()
	mode := ""

	switch args[1].(type) {
	case *object.String:
		mode = args[1].(*object.String).Value
	default:
		return NewError("Second argument must be string, got %v", args[1])
	}

	// convert from octal -> decimal
	result, err := strconv.ParseInt(mode, 8, 64)
	if err != nil {
		return FALSE
	}

	// Change the mode.
	err = os.Chmod(path, os.FileMode(result))
	if err != nil {
		return FALSE
	}
	return TRUE
}

// mkdir
func mkdirFn(args ...OBJ) OBJ {
	if len(args) != 1 {
		return NewError("wrong number of arguments. got=%d, want=1",
			len(args))
	}

	if args[0].Type() != object.STRING_OBJ {
		return NewError("argument to `mkdir` must be STRING, got %s",
			args[0].Type())
	}

	path := args[0].(*object.String).Value

	// Can't fail?
	mode, err := strconv.ParseInt("755", 8, 64)
	if err != nil {
		return FALSE
	}

	err = os.MkdirAll(path, os.FileMode(mode))
	if err != nil {
		return FALSE
	}
	return TRUE
}

// Open a file
func openFn(args ...OBJ) OBJ {
	path := ""
	mode := "r"

	// We need at least one arg
	if len(args) < 1 {
		return NewError("wrong number of arguments. got=%d, want=1+",
			len(args))
	}

	// Get the filename
	switch args[0].(type) {
	case *object.String:
		path = args[0].(*object.String).Value
	default:
		return NewError("argument to `file` not supported, got=%s",
			args[0].Type())

	}

	// Get the mode (optiona)
	if len(args) > 1 {
		switch args[1].(type) {
		case *object.String:
			mode = args[1].(*object.String).Value
		default:
			return NewError("argument to `file` not supported, got=%s",
				args[0].Type())

		}
	}

	// Create the object
	file := &object.File{Filename: path}
	file.Open(mode)
	return file
}

// Get file info.
func statFn(args ...OBJ) OBJ {
	if len(args) != 1 {
		return NewError("wrong number of arguments. got=%d, want=1",
			len(args))
	}
	path := args[0].Inspect()
	info, err := os.Stat(path)

	if err != nil {
		// Empty hash as we've not yet set anything
		return NewHash(StringObjectMap{})
	}

	// Populate a hash

	typeStr := "unknown"
	if info.Mode().IsDir() {
		typeStr = "directory"
	}
	if info.Mode().IsRegular() {
		typeStr = "file"
	}

	res := NewHash(StringObjectMap{
		"size":  &object.Integer{Value: info.Size()},
		"mtime": &object.Integer{Value: info.ModTime().Unix()},
		"perm":  &object.String{Value: info.Mode().String()},
		"mode":  &object.String{Value: fmt.Sprintf("%04o", info.Mode().Perm())},
		"type":  &object.String{Value: typeStr},
	})

	return res
}

// Remove a file/directory.
func rmFn(args ...OBJ) OBJ {
	if len(args) != 1 {
		return NewError("wrong number of arguments. got=%d, want=1",
			len(args))
	}

	path := args[0].Inspect()

	err := os.Remove(path)
	if err != nil {
		return FALSE
	}
	return TRUE
}

func mvFn(args ...OBJ) OBJ {
	var from string
	var to string
	switch a := args[0].(type) {
	case *object.String:
		from = a.Value
	default:
		return NewError("mv expected string arg!")
	}
	switch a := args[1].(type) {
	case *object.String:
		to = a.Value
	default:
		return NewError("mv expected string arg!")
	}

	e := os.Rename(from, to)
	if e != nil {
		return NewError("error moving file %s", e.Error())
	}

	return NULL
}

func cpFn(args ...OBJ) OBJ {
	var src string
	var dst string
	switch a := args[0].(type) {
	case *object.String:
		src = a.Value
	default:
		return NewError("mv expected string arg!")
	}
	switch a := args[1].(type) {
	case *object.String:
		dst = a.Value
	default:
		return NewError("mv expected string arg!")
	}

	sfi, err := os.Stat(src)
	if err != nil {
		return NewError("fs.cp source does not exist!")
	}
	if !sfi.Mode().IsRegular() {
		// cannot copy non-regular files (e.g., directories,
		// symlinks, devices, etc.)
		return NewError("fs.cp expected regular file!")
	}
	dfi, err := os.Stat(dst)
	if err != nil {
		if !os.IsNotExist(err) {
			return NewError("error copying file %s", err.Error())
		}
	} else {
		if !(dfi.Mode().IsRegular()) {
			return NewError("non-regular destination file")
		}
		if os.SameFile(sfi, dfi) {
			return NewError("copying to same file")
		}
	}

	in, err := os.Open(src)
	if err != nil {
		return NewError("error copying file %s", err.Error())
	}

	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return NewError("error copying file %s", err.Error())
	}

	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return NewError("error copying file %s", err.Error())
	}
	err = out.Sync()

	if err != nil {
		return NewError("error copying file %s", err.Error())
	}

	return NULL
}

func templateFn(env *ENV, args ...OBJ) OBJ {
	switch a := args[0].(type) {
	case *object.String:
		b, err := os.ReadFile(a.Value)
		if err != nil {
			return NewError("Error reading template file: %s", err)
		}
		s := string(b)
		res := Interpolate(s, env)
		return &object.String{Value: res}
	default:
		return NewError("fs.tmpl expected string arg!")
	}
}

func init() {
	RegisterBuiltin("fs.glob",
		func(env *ENV, args ...OBJ) OBJ {
			return fsGlob(args...)
		})
	RegisterBuiltin("fs.chmod",
		func(env *ENV, args ...OBJ) OBJ {
			return chmodFn(args...)
		})
	RegisterBuiltin("fs.mkdir",
		func(env *ENV, args ...OBJ) OBJ {
			return mkdirFn(args...)
		})
	RegisterBuiltin("fs.open",
		func(env *ENV, args ...OBJ) OBJ {
			return openFn(args...)
		})
	RegisterBuiltin("fs.stat",
		func(env *ENV, args ...OBJ) OBJ {
			return statFn(args...)
		})
	RegisterBuiltin("fs.rm",
		func(env *ENV, args ...OBJ) OBJ {
			return rmFn(args...)
		})
	RegisterBuiltin("fs.mv",
		func(env *ENV, args ...OBJ) OBJ {
			return mvFn(args...)
		})
	RegisterBuiltin("fs.cp",
		func(env *ENV, args ...OBJ) OBJ {
			return cpFn(args...)
		})
	RegisterBuiltin("fs.tmpl",
		func(env *ENV, args ...OBJ) OBJ {
			return templateFn(env, args...)
		})

}
