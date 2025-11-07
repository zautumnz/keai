# TODO

# Sources (Interesting stuff to check out)

* go-mustache
* prologic's version
* skx's version
* abs
* the book

# Remaining v1 Work

* Bugs (see bugs directory for details on some of them):
    * Strings need a double escape when they should need one (see last string
        example and colorize in stdlib)
    * Spread isn't quite right, see curry in stdlib
    * Vim config bugs:
        * Function group is matching `fn foo (x)` but we want `let foo = fn (x)`
        * Comments aren't indented when using `>>`/`<<` and `=`
    * Ctags config:
        * Identifiers can be unicode, and also can include dots
    * More than one level of dot access on stdlib and non-standard objects (like
        files) gets parsed as an IDENT which causes a failure
* Features:
    * http.client: add form support
* Chores:
    * Confirm that everything under ./examples works
    * Add argument validation to all internal functions and stdlib
    * Improve all Go error messages

## Possible Future Features

* LSP
* Treesitter
* Zed syntax
* Tests written in keai
    * If they can somehow count towards coverage from `go test` that would be
        cool
* Possible `break` keyword to get out of loops
* Allow listing empty root-level modules and non-object modules such as http and
    fs using just the root word (`http` or `fs`).
* Consider changing how module exports work to allow top-level (but still
    non-exported) mutable variables; maybe a new keyword (capital letters aren't
    an option because we allow unicode identifiers)
* Utility like Node's `__filename` (which can also be used to get dirname)
* Change import, http.server, and other paths to allow relative paths/from the
    keai file being executed
* Add basic module management: some kind of module manifest, vcs manager, and
    automatic KEAI_PATH modification
* Add option to compile a program (along with keai itself) to a binary
* 80%+ code coverage
* Nested interpolations
* Add tab-completion to the REPL
* Maybe combine float/integer to just one number type?
* Move as much of the stdlib into keai (out of Go) as possible
* Full-featured examples:
    * Twitter/Tumblr clone
    * Ranger clone
    * Text editor
* Date object or additions to core time module
* Markdown parser
* Simplify registerBuiltin calls so they can be looped over
* Cryptography builtins: GUID, hashes, AES, RSA, crypto/rand, etc.
* YAML support
* TOML support
* Websocket support
* Multiple-db ORM
