package repl

import (
	"fmt"
	"io"
	"os"
	"os/user"
	"strconv"
	"strings"

	"github.com/chzyer/readline"
	"github.com/zautumnz/keai/evaluator"
	"github.com/zautumnz/keai/lexer"
	"github.com/zautumnz/keai/object"
	"github.com/zautumnz/keai/parser"
	"github.com/zautumnz/keai/utils"
)

func getHistorySize() int {
	val := os.Getenv("KEAI_HISTSIZE")
	l, e := strconv.Atoi(val)
	if e != nil || val == "" {
		return 1000
	}
	return l
}

func getUserHome() (string, error) {
	h := os.Getenv("HOME")
	if h == "" {
		usr, err := user.Current()
		if err != nil {
			return "", err
		}

		h = usr.HomeDir
	}

	return h, nil
}

func getHomeBasedFile(path string) string {
	userHome, err := getUserHome()
	if err != nil {
		fmt.Println("The current user has no home directory!")
		os.Exit(1)
	}
	return userHome + "/" + path
}

// init file idea, but not code, taken from github.com/abs-lang
func getInitFile() string {
	filePath := getHomeBasedFile(".keai_init")
	s, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	return string(s)
}

// Start runs the REPL
func Start(in io.Reader, out io.Writer, stdlib string) {
	// set so we don't os.Exit on errors
	utils.SetReplOrRun(true)
	env := object.NewEnvironment()

	// set up initial program with stdlib and optional init file
	initConfig := getInitFile()
	initLex := lexer.New(stdlib + "\n" + initConfig + "\n")
	initPars := parser.New(initLex)
	initProg := initPars.ParseProgram()
	// put the initial program in the env
	evaluator.Eval(initProg, env)

	l, err := readline.NewEx(&readline.Config{
		Prompt:            "> ",
		HistoryFile:       getHomeBasedFile(".keai_history"),
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistorySearchFold: true,
		HistoryLimit:      getHistorySize(),
	})

	if err != nil {
		panic(err)
	}
	defer l.Close()

	for {
		line, err := l.Readline()
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				break
			} else {
				continue
			}
		} else if err == io.EOF {
			break
		}

		line = strings.TrimSpace(line)
		lex := lexer.New(line)
		p := parser.New(lex)
		program := p.ParseProgram()
		if len(p.Errors()) != 0 {
			parser.PrintParserErrors(
				parser.ParserErrorsParams{Errors: p.Errors(), Out: out},
			)
			continue
		}
		evaluated := evaluator.Eval(program, env)
		if evaluated != nil {
			io.WriteString(out, evaluated.Inspect())
			io.WriteString(out, "\n")
		}
	}
}
