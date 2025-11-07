package evaluator

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/zautumnz/keai/lexer"
	"github.com/zautumnz/keai/object"
	"github.com/zautumnz/keai/parser"
)

var searchPaths []string

func init() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("error getting cwd: %s", err)
	}

	if e := os.Getenv("KEAI_PATH"); e != "" {
		tokens := strings.Split(e, ":")
		for _, token := range tokens {
			addPath(token) // ignore errors
		}
	} else {
		searchPaths = append(searchPaths, cwd)
	}
}

func addPath(path string) error {
	path = os.ExpandEnv(filepath.Clean(path))
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	searchPaths = append(searchPaths, absPath)
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// FindModule finds a module based on name, used by the evaluator
func FindModule(name string) string {
	basename := fmt.Sprintf("%s.keai", name)
	for _, p := range searchPaths {
		filename := filepath.Join(p, basename)
		if exists(filename) {
			return filename
		}
	}
	return ""
}

// IsNumber checks to see if a value is a number
func IsNumber(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// Interpolate (str, env)
// return input string with $vars interpolated from environment
func Interpolate(str string, env *ENV) string {
	// Match all strings preceded by {{
	re := regexp.MustCompile(`(?s)(\\)?(\{\{)(.*?)(\}\})`)
	str = re.ReplaceAllStringFunc(str, func(m string) string {
		// If the string starts with a backslash, that's an escape, so we should
		// replace it with the remaining portion of the match. \{{VAR}} becomes
		// {{VAR}}
		if string(m[0]) == "\\" {
			return m[1:]
		}

		varName := ""

		// If you type a variable wrong, forgetting the closing bracket, we
		// simply return it to you: eg "my {{variable"

		if m[len(m)-1] != '}' || m[len(m)-2] != '}' {
			return m
		}

		varName = m[2 : len(m)-2]

		v, ok := env.Get(varName)

		// The variable might be an index expression
		if !ok {
			// Basically just spinning up a whole new instance of keai; very
			// inefficient, but it's the same thing we do on every module import
			l := lexer.New(string(varName))
			p := parser.New(l)
			program := p.ParseProgram()
			evaluated := Eval(program, env)
			if evaluated != nil {
				return evaluated.Inspect()
			}

			// Still no match found, so return an empty string
			return ""
		}

		return v.Inspect()
	})

	return str
}

// NewError prints and returns an error
func NewError(format string, a ...interface{}) *object.Error {
	message := fmt.Sprintf(format, a...)
	return &object.Error{Message: message}
}

// StringObjectMap is a map of string keys to keai objects
type StringObjectMap map[string]OBJ

// NewHash creates a new keai Hash
func NewHash(x StringObjectMap) *object.Hash {
	res := make(map[object.HashKey]object.HashPair)
	for k, v := range x {
		key := &object.String{Value: k}
		pair := object.HashPair{Key: key, Value: v}
		res[key.HashKey()] = pair
	}

	return &object.Hash{Pairs: res}
}
